package llama

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/reasoning"
)

// warm holds the active modeld session handle across turns. It is bounded (idle
// TTL + resident cap, see modelrepo.WarmCache): switching models evicts and
// closes idle handles before opening another slot.
var warm = modelrepo.NewWarmCache[Session]()

// acquire returns the warm entry for this client's model+config, opening a modeld
// session on a miss. The caller must hold the entry's Turn for the whole turn.
func (c *client) acquire() (*modelrepo.WarmEntry[Session], error) {
	ref := c.ref()
	cfg := normalizeConfig(c.cfg)
	return warm.Acquire(sessionCacheKey(ref, cfg), func() (Session, error) {
		return newSession(ref, cfg)
	})
}

// sessionCacheKey identifies a resident session by the model's logical identity
// (name + type + content digest) and the runtime config — NOT the raw filesystem
// path, so two names resolving to the same bytes share warm KV and a path change
// alone never silently reuses a stale model.
func sessionCacheKey(ref modeldconn.ModelRef, cfg Config) string {
	cfg = normalizeConfig(cfg)
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s", ref.Type, ref.Name)
	fmt.Fprintf(&b, "\x00model=%s\x00ctx=%d\x00planner=%d\x00batch=%d\x00threads=%d\x00gpu=%d\x00flash=%t\x00kv=%s",
		ref.Digest, cfg.NumCtx, cfg.PlannerEffectiveContext, cfg.NumBatch, cfg.NumThreads, cfg.NumGpuLayers, cfg.FlashAttn, cfg.KVCacheType)
	b.WriteString("\x00split=")
	for i, v := range cfg.TensorSplit {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	fmt.Fprintf(&b, "\x00prompt=%s\x00template=%s\x00bos=%t\x00reasoning=%s",
		cfg.PromptFormat, cfg.PromptTemplateDigest, !cfg.DisableBOS, cfg.ReasoningFormat)
	// Adapter identity in list order (order is part of identity): this is what stops
	// base+A reusing base+B's warm KV. Empty adapters → the base model.
	b.WriteString("\x00adapters=")
	for i, a := range ref.Adapters {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, "%s@%s", a.Digest, strconv.FormatFloat(float64(a.Scale), 'g', -1, 32))
	}
	return b.String()
}

// closeCachedSessionsForTest releases all cached sessions (test cleanup).
func closeCachedSessionsForTest() { warm.Clear() }

type client struct {
	modelName         string
	modelPath         string
	profileID         string
	modelDigest       string
	backendVersion    string
	cfg               Config
	adapters          []AdapterSpec // LoRA adapters for this variant ("" = base model)
	maxOutputTokens   int
	toolProtocol      string // profile-declared tool-call protocol ("" = tools unsupported)
	reasoningProtocol string // profile-declared reasoning parser ("" = no reasoning parser)
	// deviceKind/freeBytes and the described* fields are the capacity facts
	// modeld reported at construction (empty/zero when modeld did not answer
	// Describe). They turn a context overflow into an actionable message — see
	// explainOverflow. Informational only: they must never be written back into
	// cfg, or a stale Describe answer becomes a hard ceiling on the real
	// session (see capacity.HardContextLimit).
	deviceKind                string
	freeBytes                 int64
	describedEffectiveContext int
	describedPlannerContext   int
	describedModelMaxContext  int
}

// explainOverflow enriches a context-overflow error with the capacity facts this
// client captured from modeld's Describe, so a chat surfaces an actionable
// message ("model X serves only N tokens on this GPU after weights …") instead of
// raw resident/additional token counts. It is a no-op for any other error and
// preserves errors.Is(err, ErrContextOverflow) plus the "context overflow"
// substring, so existing transport/error handling keeps recognizing it.
func (c *client) explainOverflow(err error) error {
	if err == nil || !errors.Is(err, ErrContextOverflow) {
		return err
	}
	where := "this device"
	if c.deviceKind != "" {
		where = "the " + c.deviceKind + " device"
	}
	free := ""
	if c.freeBytes > 0 {
		free = fmt.Sprintf(" with %s free", humanBytes(c.freeBytes))
	}
	// Prefer the live session's window carried inside the overflow error itself
	// (modeld's ContextOverflowError arrives over gRPC as text): the
	// construction-time Describe answer can be stale — e.g. computed while a
	// previous session was still resident — and quoting it produces nonsense
	// like "serves only 433 tokens" for a session actually serving 3854.
	served := overflowNumCtx(err)
	if served <= 0 && c.cfg.NumCtx > 0 {
		served = c.cfg.NumCtx
	}
	if served <= 0 {
		served = c.describedEffectiveContext
	}
	if served <= 0 {
		return fmt.Errorf("%w: model %q exceeded its context window on %s%s — free VRAM, use a smaller model, or enable q8_0 KV cache",
			err, c.modelName, where, free)
	}
	return fmt.Errorf("%w: model %q serves only %d context tokens on %s%s after model weights — free VRAM, use a smaller model, or enable q8_0 KV cache",
		err, c.modelName, served, where, free)
}

// overflowNumCtx extracts the live session window from a context-overflow
// error's text ("… num_ctx=2890 …"). modeld's typed ContextOverflowError does
// not survive the gRPC boundary — it arrives re-wrapped as a string — so text
// is the only cross-wire carrier of the session's actual window.
func overflowNumCtx(err error) int {
	if err == nil {
		return 0
	}
	msg := err.Error()
	i := strings.LastIndex(msg, "num_ctx=")
	if i < 0 {
		return 0
	}
	rest := msg[i+len("num_ctx="):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	n, convErr := strconv.Atoi(rest[:end])
	if convErr != nil {
		return 0
	}
	return n
}

// humanBytes formats a byte count with binary (KiB/MiB/…) units for diagnostics.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

// ref is the typed model handle this client opens sessions with: logical name +
// backend type + content digest for identity, plus the resolved on-disk path and
// any LoRA adapters that make this a distinct model variant.
func (c *client) ref() modeldconn.ModelRef {
	return modeldconn.ModelRef{Name: c.modelName, Type: "llama", Digest: c.modelDigest, Path: c.modelPath, Adapters: c.adapters}
}

func (c *client) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	cfg := applyChatArgs(args)

	var toolsJSON string
	parseToolCalls := false
	if len(cfg.Tools) > 0 {
		// Tools require a profile-declared tool protocol: the daemon renders tool
		// definitions and parses tool-call output via llama.cpp's model-native
		// common chat path. No protocol means no guessing.
		if c.toolProtocol == "" {
			return modelrepo.ChatResult{}, NewUnsupportedFeatureError("tool calls (model declares no tool_calls.protocol)")
		}
		if !toolCallProtocolKnown(c.toolProtocol) {
			return modelrepo.ChatResult{}, fmt.Errorf("%w: tool protocol %q", ErrUnsupportedFeature, c.toolProtocol)
		}
		parseToolCalls = true
		var err error
		if toolsJSON, err = serializeToolDefs(cfg.Tools); err != nil {
			return modelrepo.ChatResult{}, err
		}
	}

	dc, showThinking, enableThinking, err := c.decodeOptions(cfg)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	if parseToolCalls {
		dc.ParserProtocols = append(dc.ParserProtocols, toolParserProtocolCommonChat)
	}
	text, thinking, toolCalls, err := c.generate(ctx, messages, dc, toolsJSON, enableThinking, showThinking)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}

	msg := modelrepo.Message{Role: "assistant", Content: text}
	if showThinking {
		msg.Thinking = thinking
	}
	msg.ToolCalls = toolCalls
	return modelrepo.ChatResult{Message: msg, ToolCalls: toolCalls}, nil
}

func (c *client) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	var messages []modelrepo.Message
	if s := strings.TrimSpace(systemInstruction); s != "" {
		messages = append(messages, modelrepo.Message{Role: "system", Content: s})
	}
	messages = append(messages, modelrepo.Message{Role: "user", Content: prompt})
	temp := float64(temperature)
	dc, _, enableThinking, err := c.decodeOptions(&modelrepo.ChatConfig{Temperature: &temp})
	if err != nil {
		return "", err
	}
	text, _, _, err := c.generate(ctx, messages, dc, "", enableThinking, false)
	return text, err
}

func (c *client) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := applyChatArgs(args)
	if len(cfg.Tools) > 0 {
		return nil, NewUnsupportedFeatureError("tool calls")
	}
	dc, showThinking, enableThinking, err := c.decodeOptions(cfg)
	if err != nil {
		return nil, err
	}
	cs, err := c.acquire()
	if err != nil {
		return nil, err
	}

	cs.Turn.Lock()
	if err := c.prime(ctx, cs, messages, "", enableThinking); err != nil {
		cs.Turn.Unlock()
		if fatalSessionError(err) {
			warm.Drop(cs)
		}
		return nil, err
	}
	chunks, err := cs.Sess.Decode(ctx, dc)
	if err != nil {
		cs.Turn.Unlock()
		if fatalSessionError(err) {
			warm.Drop(cs)
		}
		return nil, c.explainOverflow(err)
	}

	out := make(chan *modelrepo.StreamParcel, 16)
	go func() {
		defer close(out)
		defer cs.Turn.Unlock()
		for chunk := range chunks {
			if chunk.Error != nil {
				out <- &modelrepo.StreamParcel{Error: c.explainOverflow(chunk.Error)}
				if fatalSessionError(chunk.Error) {
					warm.Drop(cs)
				}
				return
			}
			if chunk.Text != "" {
				out <- &modelrepo.StreamParcel{Data: chunk.Text}
			}
			if showThinking && chunk.Thinking != "" {
				out <- &modelrepo.StreamParcel{Thinking: chunk.Thinking}
			}
		}
	}()
	return out, nil
}

func (c *client) generate(ctx context.Context, messages []modelrepo.Message, dc DecodeConfig, toolsJSON string, enableThinking *bool, showThinking bool) (string, string, []modelrepo.ToolCall, error) {
	cs, err := c.acquire()
	if err != nil {
		return "", "", nil, err
	}
	cs.Turn.Lock()
	defer cs.Turn.Unlock()

	if err := c.prime(ctx, cs, messages, toolsJSON, enableThinking); err != nil {
		if fatalSessionError(err) {
			warm.Drop(cs)
		}
		return "", "", nil, err
	}
	chunks, err := cs.Sess.Decode(ctx, dc)
	if err != nil {
		return "", "", nil, c.explainOverflow(err)
	}
	var b strings.Builder
	var thinking strings.Builder
	var toolCalls []modelrepo.ToolCall
	for chunk := range chunks {
		if chunk.Error != nil {
			if fatalSessionError(chunk.Error) {
				warm.Drop(cs)
			}
			return "", "", nil, c.explainOverflow(chunk.Error)
		}
		b.WriteString(chunk.Text)
		if showThinking {
			thinking.WriteString(chunk.Thinking)
		}
		toolCalls = appendToolCalls(toolCalls, chunk.ToolCalls)
	}
	return strings.TrimSpace(b.String()), thinking.String(), toolCalls, nil
}

func appendToolCalls(dst []modelrepo.ToolCall, src []ToolCall) []modelrepo.ToolCall {
	for _, in := range src {
		var out modelrepo.ToolCall
		out.ID = in.ID
		out.Type = in.Type
		out.Function.Name = in.Function.Name
		out.Function.Arguments = in.Function.Arguments
		dst = append(dst, out)
	}
	return dst
}

// prime ensures the warm stable prefix and prefills the volatile suffix. Caller
// holds cs.Turn.
func (c *client) prime(ctx context.Context, cs *modelrepo.WarmEntry[Session], messages []modelrepo.Message, toolsJSON string, enableThinking *bool) error {
	plan, err := buildPromptPlan(messages, c.cfg, promptIdentity{
		ProfileID:      c.profileID,
		ModelDigest:    c.modelDigest,
		BackendVersion: c.backendVersion,
		Adapters:       c.adapters,
	}, toolsJSON)
	if err != nil {
		return err
	}
	if _, err := cs.Sess.EnsurePrefix(ctx, plan.Stable); err != nil {
		return c.explainOverflow(err)
	}
	plan.Volatile.EnableThinking = enableThinking
	_, err = cs.Sess.PrefillSuffix(ctx, plan.Volatile)
	return c.explainOverflow(err)
}

func applyChatArgs(args []modelrepo.ChatArgument) *modelrepo.ChatConfig {
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}
	return cfg
}

func decodeConfig(cfg *modelrepo.ChatConfig, maxOutputTokens int) DecodeConfig {
	dc := DecodeConfig{MaxTokens: 256}
	if cfg != nil && cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
		dc.MaxTokens = *cfg.MaxTokens
	}
	dc.MaxTokens, _ = modelrepo.ClampMaxOutputTokens(dc.MaxTokens, maxOutputTokens)
	if cfg != nil && cfg.Temperature != nil {
		v := *cfg.Temperature
		dc.Temperature = &v
	}
	if cfg != nil && cfg.TopP != nil {
		v := *cfg.TopP
		dc.TopP = &v
	}
	if cfg != nil && cfg.Seed != nil {
		v := *cfg.Seed
		dc.Seed = &v
	}
	return dc
}

func (c *client) decodeOptions(cfg *modelrepo.ChatConfig) (DecodeConfig, bool, *bool, error) {
	dc := decodeConfig(cfg, c.maxOutputTokens)
	if err := validateReasoningProtocol(c.reasoningProtocol); err != nil {
		return DecodeConfig{}, false, nil, err
	}
	if c.reasoningProtocol == "" {
		return dc, false, nil, nil
	}
	if c.cfg.ReasoningFormat == "" {
		return DecodeConfig{}, false, nil, fmt.Errorf("%w: reasoning format is required when reasoning protocol %q is set", ErrUnsupportedFeature, c.reasoningProtocol)
	}
	dc.ParserProtocols = append(dc.ParserProtocols, c.reasoningProtocol)
	dc.ReasoningFormat = c.cfg.ReasoningFormat
	showThinking := false
	var enableThinking *bool
	if cfg != nil && cfg.Think != nil {
		level, ok, err := reasoning.NormalizeOptional(*cfg.Think)
		if err != nil {
			return DecodeConfig{}, false, nil, err
		}
		if ok && level != reasoning.Auto {
			v := level != reasoning.Off
			enableThinking = &v
		}
		showThinking = ok && level != reasoning.Off && level != reasoning.Auto
	}
	return dc, showThinking, enableThinking, nil
}

func fatalSessionError(err error) bool {
	return errors.Is(err, ErrSessionClosed) || errors.Is(err, ErrSessionFatal)
}

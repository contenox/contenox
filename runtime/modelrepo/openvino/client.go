package openvino

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/reasoning"
	"github.com/contenox/runtime/runtime/transport"
)

// warm holds the active modeld session handle across turns. It is bounded (idle
// TTL + resident cap, see modelrepo.WarmCache): switching models evicts and
// closes idle handles before opening another slot. Eviction captures the
// session's Snapshot to a durable on-disk store keyed by the same identity as
// sessionCacheKey, so swapping a model back in later restores its warm KV
// instead of cold-prefilling. Snapshot survival is off (falls back to
// always-cold reopen, the pre-existing behavior) when modeldconn.SnapshotDir
// returns "".
var warm = modelrepo.NewWarmCacheWithSnapshots[Session](
	modelrepo.NewDiskSnapshotStore(func() string { return modeldconn.SnapshotDir("openvino") }, modelrepo.SnapshotMaxBytes, modelrepo.SnapshotTTL),
	func(ctx context.Context, s Session) ([]byte, error) {
		// Short-circuit the Snapshot round-trip when snapshot survival is disabled
		// (SnapshotDir == ""), so the kill switch costs nothing per eviction/exit.
		if modeldconn.SnapshotDir("openvino") == "" {
			return nil, nil
		}
		snap, err := s.Snapshot(ctx)
		if err != nil {
			return nil, err
		}
		return json.Marshal(snap)
	},
	func(ctx context.Context, s Session, blob []byte) error {
		var snap transport.SessionSnapshot
		if err := json.Unmarshal(blob, &snap); err != nil {
			return err
		}
		return s.Restore(ctx, snap)
	},
)

// init flushes resident warm sessions to the durable snapshot store on graceful
// process exit (modelrepo.Shutdown, invoked by the CLI after a command returns),
// so warm KV survives a runtime restart even when no model swap ever evicted the
// session during the process's life.
func init() {
	modelrepo.RegisterShutdownHook(func() error { warm.CaptureResident(); return nil })
}

// acquire returns the warm entry for this client's model+config, opening a modeld
// session on a miss. The caller must hold the entry's Turn for the whole turn.
func (c *client) acquire() (*modelrepo.WarmEntry[Session], error) {
	ref := c.ref()
	cfg := normalizeConfig(c.cfg)
	key := sessionCacheKey(ref, cfg)
	if c.target.BackendID != "" {
		key = c.target.BackendID + ":" + key
	}
	return warm.Acquire(key, func() (Session, error) {
		return newSession(ref, cfg, c.target)
	})
}

// sessionCacheKey identifies a resident session by the model's logical identity
// (name + type + content digest) and the runtime config — not the raw IR path,
// so a path change alone never silently reuses a stale model.
func sessionCacheKey(ref modeldconn.ModelRef, cfg Config) string {
	cfg = normalizeConfig(cfg)
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s", ref.Type, ref.Name)
	fmt.Fprintf(&b, "\x00model=%s\x00ctx=%d\x00planner=%d\x00prompt=%s\x00template=%s",
		ref.Digest, cfg.NumCtx, cfg.PlannerEffectiveContext, cfg.PromptFormat, cfg.PromptTemplateDigest)
	b.WriteString("\x00adapters=")
	appendAdapterIdentity(&b, ref.Adapters)
	return b.String()
}

type client struct {
	modelName        string
	modelPath        string
	profileID        string
	modelDigest      string
	backendVersion   string
	cfg              Config
	adapters         []transport.AdapterSpec
	maxOutputTokens  int
	toolProtocol     string // profile-declared OpenVINO tool protocol ("" = tools unsupported)
	reasoningParser  string // profile-declared complete reasoning parser ("" = no complete parser)
	reasoningStream  string // profile-declared incremental reasoning parser ("" = no stream parser)
	supportsThinking bool   // profile/catalog-declared template thinking controls
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

	target modeldconn.ModeldTarget // if set, use targeted modeld conn (remote or explicit node)
}

// explainOverflow enriches a context-overflow error with the capacity facts this
// client captured from modeld's Describe, so a chat surfaces an actionable
// message instead of raw resident/additional token counts. It is a no-op for any
// other error and preserves errors.Is(err, transport.ErrContextOverflow) plus the
// "context overflow"/"context window" substring so existing handling still works.
func (c *client) explainOverflow(err error) error {
	if err == nil || !errors.Is(err, transport.ErrContextOverflow) {
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
	// Prefer the live session's window carried as structured transport detail;
	// Describe-time numbers can be stale and misleading.
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

func overflowNumCtx(err error) int {
	if detail, ok := transport.ContextOverflowDetailFromError(err); ok {
		return detail.NumCtx
	}
	return 0
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
// backend type + content digest for identity, plus the resolved IR directory.
func (c *client) ref() modeldconn.ModelRef {
	return modeldconn.ModelRef{Name: c.modelName, Type: "openvino", Digest: c.modelDigest, Path: c.modelPath, Adapters: c.adapters}
}

func (c *client) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	cfg := applyChatArgs(args)

	toolPlan, err := c.prepareTools(cfg)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}

	dc, showThinking, enableThinking, reasoningEffort, err := c.decodeConfig(cfg, toolPlan.completeDecode())
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	if toolPlan.ParserProtocol != "" {
		dc.ParserProtocols = append(dc.ParserProtocols, toolPlan.ParserProtocol)
	}
	dc.StructuredOutput = toolPlan.StructuredOutput
	text, thinking, toolCalls, err := c.generate(ctx, messages, dc, toolPlan.ToolsJSON, enableThinking, reasoningEffort, showThinking)
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
	cfg := &modelrepo.ChatConfig{Temperature: &temp}
	dc, _, enableThinking, reasoningEffort, err := c.decodeConfig(cfg, false)
	if err != nil {
		return "", err
	}
	text, _, _, err := c.generate(ctx, messages, dc, "", enableThinking, reasoningEffort, false)
	return text, err
}

func (c *client) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := applyChatArgs(args)
	toolPlan, err := c.prepareTools(cfg)
	if err != nil {
		return nil, err
	}
	dc, showThinking, enableThinking, reasoningEffort, err := c.decodeConfig(cfg, toolPlan.completeDecode())
	if err != nil {
		return nil, err
	}
	if toolPlan.ParserProtocol != "" {
		dc.ParserProtocols = append(dc.ParserProtocols, toolPlan.ParserProtocol)
	}
	dc.StructuredOutput = toolPlan.StructuredOutput
	cs, err := c.acquire()
	if err != nil {
		return nil, err
	}

	cs.Turn.Lock()
	if err := c.prime(ctx, cs, messages, toolPlan.ToolsJSON, enableThinking, reasoningEffort); err != nil {
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
			toolCalls := modelToolCalls(chunk.ToolCalls)
			if chunk.Text != "" || len(toolCalls) > 0 {
				out <- &modelrepo.StreamParcel{Data: chunk.Text, ToolCalls: toolCalls}
			}
			if showThinking && chunk.Thinking != "" {
				out <- &modelrepo.StreamParcel{Thinking: chunk.Thinking}
			}
		}
	}()
	return out, nil
}

func (c *client) decodeConfig(cfg *modelrepo.ChatConfig, completeParsers bool) (DecodeConfig, bool, *bool, string, error) {
	dc := decodeConfig(cfg, c.maxOutputTokens)
	reasoningProtocol := c.reasoningStream
	if completeParsers {
		reasoningProtocol = c.reasoningParser
	}
	if reasoningProtocol != "" {
		dc.ParserProtocols = append(dc.ParserProtocols, reasoningProtocol)
	}
	showThinking := false
	var enableThinking *bool
	var reasoningEffort string
	if (c.supportsThinking || reasoningProtocol != "") && cfg != nil && cfg.Think != nil {
		level, ok, err := reasoning.NormalizeOptional(*cfg.Think)
		if err != nil {
			return DecodeConfig{}, false, nil, "", err
		}
		if ok && level != reasoning.Auto {
			v := level != reasoning.Off
			enableThinking = &v
		}
		if reasoningProtocol != "" {
			showThinking = ok && level != reasoning.Off && level != reasoning.Auto
		}
		reasoningEffort = reasoningEffortForTemplate(level)
	}
	return dc, showThinking, enableThinking, reasoningEffort, nil
}

type toolPlan struct {
	ToolsJSON        string
	ParserProtocol   string
	StructuredOutput transport.StructuredOutputConfig
}

func (p toolPlan) completeDecode() bool {
	return p.ParserProtocol != "" || p.StructuredOutput.Protocol != ""
}

func (c *client) prepareTools(cfg *modelrepo.ChatConfig) (toolPlan, error) {
	if cfg == nil || len(cfg.Tools) == 0 {
		return toolPlan{}, nil
	}
	// Tools require a profile-declared protocol: modeld renders the tool
	// definitions via the model's own chat template (model-native), and the
	// declared OpenVINO parser protocol parses model output inside modeld. No
	// protocol means no guessing.
	if c.toolProtocol == "" {
		return toolPlan{}, NewUnsupportedFeatureError("tool calls (model declares no tool_calls.protocol)")
	}
	if !toolCallProtocolKnown(c.toolProtocol) {
		return toolPlan{}, fmt.Errorf("%w: tool protocol %q", ErrUnsupportedFeature, c.toolProtocol)
	}
	toolsJSON, err := serializeToolDefs(cfg.Tools)
	if err != nil {
		return toolPlan{}, err
	}
	if toolCallProtocolUsesParser(c.toolProtocol) {
		return toolPlan{ToolsJSON: toolsJSON, ParserProtocol: c.toolProtocol}, nil
	}
	switch c.toolProtocol {
	case toolProtocolJSONSchemaToolCalls:
		payload, err := toolCallsJSONSchema(cfg.Tools)
		if err != nil {
			return toolPlan{}, err
		}
		return toolPlan{
			ToolsJSON: toolsJSON,
			StructuredOutput: transport.StructuredOutputConfig{
				Protocol: toolProtocolJSONSchemaToolCalls,
				Payload:  payload,
			},
		}, nil
	}
	return toolPlan{}, fmt.Errorf("%w: tool protocol %q", ErrUnsupportedFeature, c.toolProtocol)
}

func (c *client) generate(ctx context.Context, messages []modelrepo.Message, dc DecodeConfig, toolsJSON string, enableThinking *bool, reasoningEffort string, showThinking bool) (string, string, []modelrepo.ToolCall, error) {
	cs, err := c.acquire()
	if err != nil {
		return "", "", nil, err
	}
	cs.Turn.Lock()
	defer cs.Turn.Unlock()

	if err := c.prime(ctx, cs, messages, toolsJSON, enableThinking, reasoningEffort); err != nil {
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
		toolCalls = append(toolCalls, modelToolCalls(chunk.ToolCalls)...)
	}
	return strings.TrimSpace(b.String()), thinking.String(), toolCalls, nil
}

func serializeToolDefs(tools []modelrepo.Tool) (string, error) {
	if len(tools) == 0 {
		return "", nil
	}
	b, err := json.Marshal(tools)
	if err != nil {
		return "", fmt.Errorf("openvino: serialize tool definitions: %w", err)
	}
	return string(b), nil
}

func modelToolCalls(in []ToolCall) []modelrepo.ToolCall {
	if len(in) == 0 {
		return nil
	}
	out := make([]modelrepo.ToolCall, 0, len(in))
	for _, tc := range in {
		call := modelrepo.ToolCall{ID: tc.ID, Type: tc.Type}
		call.Function.Name = tc.Function.Name
		call.Function.Arguments = tc.Function.Arguments
		out = append(out, call)
	}
	return out
}

// prime ensures the warm stable prefix and prefills the volatile suffix. Caller
// holds cs.Turn. toolsJSON (model-native tool definitions) rides on the stable
// prefix so modeld renders it via the model's own chat template.
func (c *client) prime(ctx context.Context, cs *modelrepo.WarmEntry[Session], messages []modelrepo.Message, toolsJSON string, enableThinking *bool, reasoningEffort string) error {
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
	plan.Volatile.ReasoningEffort = reasoningEffort
	_, err = cs.Sess.PrefillSuffix(ctx, plan.Volatile)
	return c.explainOverflow(err)
}

func reasoningEffortForTemplate(level string) string {
	switch level {
	case reasoning.Low, reasoning.Medium, reasoning.High:
		return level
	case reasoning.Minimal:
		return reasoning.Low
	case reasoning.XHigh:
		return reasoning.High
	default:
		return ""
	}
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

var (
	_ modelrepo.LLMChatClient       = (*client)(nil)
	_ modelrepo.LLMStreamClient     = (*client)(nil)
	_ modelrepo.LLMPromptExecClient = (*client)(nil)
)

type embedClient struct {
	modelName   string
	modelPath   string
	modelDigest string
	target      modeldconn.ModeldTarget
}

func (c *embedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	ref := modeldconn.ModelRef{
		Name:   c.modelName,
		Type:   "openvino",
		Digest: c.modelDigest,
		Path:   c.modelPath,
	}
	var res transport.EmbedResult
	var err error
	if c.target.Endpoint != "" {
		res, err = modeldconn.EmbedTarget(ctx, c.target, ref, transport.Config{}, prompt)
	} else {
		res, err = modeldconn.Embed(ctx, ref, transport.Config{}, prompt)
	}
	if err != nil {
		return nil, err
	}
	out := make([]float64, len(res.Vector))
	for i, v := range res.Vector {
		out[i] = float64(v)
	}
	return out, nil
}

var _ modelrepo.LLMEmbedClient = (*embedClient)(nil)

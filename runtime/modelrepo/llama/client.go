package llama

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
)

// warm holds resident sessions across turns. It is bounded (idle TTL + resident
// cap, see modelrepo.WarmCache): switching models evicts and closes the LRU
// session so modeld releases its model instead of stacking them.
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
	fmt.Fprintf(&b, "\x00model=%s\x00ctx=%d\x00batch=%d\x00threads=%d\x00gpu=%d\x00flash=%t\x00kv=%s",
		ref.Digest, cfg.NumCtx, cfg.NumBatch, cfg.NumThreads, cfg.NumGpuLayers, cfg.FlashAttn, cfg.KVCacheType)
	b.WriteString("\x00split=")
	for i, v := range cfg.TensorSplit {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	fmt.Fprintf(&b, "\x00prompt=%s\x00template=%s\x00bos=%t",
		cfg.PromptFormat, cfg.PromptTemplateDigest, !cfg.DisableBOS)
	return b.String()
}

// closeCachedSessionsForTest releases all cached sessions (test cleanup).
func closeCachedSessionsForTest() { warm.Clear() }

type client struct {
	modelName       string
	modelPath       string
	profileID       string
	modelDigest     string
	backendVersion  string
	cfg             Config
	maxOutputTokens int
	toolProtocol    string // profile-declared tool-call protocol ("" = tools unsupported)
}

// ref is the typed model handle this client opens sessions with: logical name +
// backend type + content digest for identity, plus the resolved on-disk path.
func (c *client) ref() modeldconn.ModelRef {
	return modeldconn.ModelRef{Name: c.modelName, Type: "llama", Digest: c.modelDigest, Path: c.modelPath}
}

func (c *client) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	cfg := applyChatArgs(args)

	var toolsJSON string
	var parser toolCallParser
	if len(cfg.Tools) > 0 {
		// Tools require a profile-declared parser protocol: the daemon renders the
		// tool definitions via the model's own GGUF chat template (model-native),
		// and the declared protocol parses the model's tool-call output. No protocol
		// means no guessing — tool calls are unsupported for this model.
		p, err := toolCallParserFor(c.toolProtocol)
		if err != nil {
			return modelrepo.ChatResult{}, err
		}
		if p == nil {
			return modelrepo.ChatResult{}, NewUnsupportedFeatureError("tool calls (model declares no tool_calls.protocol)")
		}
		parser = p
		if toolsJSON, err = serializeToolDefs(cfg.Tools); err != nil {
			return modelrepo.ChatResult{}, err
		}
	}

	text, err := c.generate(ctx, messages, decodeConfig(cfg, c.maxOutputTokens), toolsJSON)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}

	msg := modelrepo.Message{Role: "assistant", Content: text}
	if parser != nil {
		calls, content, perr := parser(text)
		if perr != nil {
			return modelrepo.ChatResult{}, perr
		}
		msg.Content = content
		msg.ToolCalls = calls
	}
	return modelrepo.ChatResult{Message: msg}, nil
}

func (c *client) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	var messages []modelrepo.Message
	if s := strings.TrimSpace(systemInstruction); s != "" {
		messages = append(messages, modelrepo.Message{Role: "system", Content: s})
	}
	messages = append(messages, modelrepo.Message{Role: "user", Content: prompt})
	temp := float64(temperature)
	return c.generate(ctx, messages, decodeConfig(&modelrepo.ChatConfig{Temperature: &temp}, c.maxOutputTokens), "")
}

func (c *client) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := applyChatArgs(args)
	if len(cfg.Tools) > 0 {
		return nil, NewUnsupportedFeatureError("tool calls")
	}
	cs, err := c.acquire()
	if err != nil {
		return nil, err
	}

	cs.Turn.Lock()
	if err := c.prime(ctx, cs, messages, ""); err != nil {
		cs.Turn.Unlock()
		if fatalSessionError(err) {
			warm.Drop(cs)
		}
		return nil, err
	}
	chunks, err := cs.Sess.Decode(ctx, decodeConfig(cfg, c.maxOutputTokens))
	if err != nil {
		cs.Turn.Unlock()
		if fatalSessionError(err) {
			warm.Drop(cs)
		}
		return nil, err
	}

	out := make(chan *modelrepo.StreamParcel, 16)
	go func() {
		defer close(out)
		defer cs.Turn.Unlock()
		for chunk := range chunks {
			if chunk.Error != nil {
				out <- &modelrepo.StreamParcel{Error: chunk.Error}
				if fatalSessionError(chunk.Error) {
					warm.Drop(cs)
				}
				return
			}
			if chunk.Text != "" {
				out <- &modelrepo.StreamParcel{Data: chunk.Text}
			}
		}
	}()
	return out, nil
}

func (c *client) generate(ctx context.Context, messages []modelrepo.Message, dc DecodeConfig, toolsJSON string) (string, error) {
	cs, err := c.acquire()
	if err != nil {
		return "", err
	}
	cs.Turn.Lock()
	defer cs.Turn.Unlock()

	if err := c.prime(ctx, cs, messages, toolsJSON); err != nil {
		if fatalSessionError(err) {
			warm.Drop(cs)
		}
		return "", err
	}
	chunks, err := cs.Sess.Decode(ctx, dc)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range chunks {
		if chunk.Error != nil {
			if fatalSessionError(chunk.Error) {
				warm.Drop(cs)
			}
			return "", chunk.Error
		}
		b.WriteString(chunk.Text)
	}
	return strings.TrimSpace(b.String()), nil
}

// prime ensures the warm stable prefix and prefills the volatile suffix. Caller
// holds cs.Turn.
func (c *client) prime(ctx context.Context, cs *modelrepo.WarmEntry[Session], messages []modelrepo.Message, toolsJSON string) error {
	plan, err := buildPromptPlan(messages, c.cfg, promptIdentity{
		ProfileID:      c.profileID,
		ModelDigest:    c.modelDigest,
		BackendVersion: c.backendVersion,
	}, toolsJSON)
	if err != nil {
		return err
	}
	if _, err := cs.Sess.EnsurePrefix(ctx, plan.Stable); err != nil {
		return err
	}
	_, err = cs.Sess.PrefillSuffix(ctx, plan.Volatile)
	return err
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

func fatalSessionError(err error) bool {
	return errors.Is(err, ErrSessionClosed) || errors.Is(err, ErrSessionFatal)
}

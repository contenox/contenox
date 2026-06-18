package openvino

import (
	"context"
	"fmt"
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
// (name + type + content digest) and the runtime config — not the raw IR path,
// so a path change alone never silently reuses a stale model.
func sessionCacheKey(ref modeldconn.ModelRef, cfg Config) string {
	cfg = normalizeConfig(cfg)
	var b strings.Builder
	fmt.Fprintf(&b, "%s/%s", ref.Type, ref.Name)
	fmt.Fprintf(&b, "\x00model=%s\x00ctx=%d\x00prompt=%s\x00template=%s",
		ref.Digest, cfg.NumCtx, cfg.PromptFormat, cfg.PromptTemplateDigest)
	return b.String()
}

type client struct {
	modelName       string
	modelPath       string
	profileID       string
	modelDigest     string
	cfg             Config
	maxOutputTokens int
}

// ref is the typed model handle this client opens sessions with: logical name +
// backend type + content digest for identity, plus the resolved IR directory.
func (c *client) ref() modeldconn.ModelRef {
	return modeldconn.ModelRef{Name: c.modelName, Type: "openvino", Digest: c.modelDigest, Path: c.modelPath}
}

func (c *client) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	cfg := applyChatArgs(args)
	if len(cfg.Tools) > 0 {
		return modelrepo.ChatResult{}, NewUnsupportedFeatureError("tool calls")
	}
	text, err := c.generate(ctx, messages, decodeConfig(cfg, c.maxOutputTokens))
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	return modelrepo.ChatResult{Message: modelrepo.Message{Role: "assistant", Content: text}}, nil
}

func (c *client) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	var messages []modelrepo.Message
	if s := strings.TrimSpace(systemInstruction); s != "" {
		messages = append(messages, modelrepo.Message{Role: "system", Content: s})
	}
	messages = append(messages, modelrepo.Message{Role: "user", Content: prompt})
	temp := float64(temperature)
	return c.generate(ctx, messages, decodeConfig(&modelrepo.ChatConfig{Temperature: &temp}, c.maxOutputTokens))
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
	if err := c.prime(ctx, cs, messages); err != nil {
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

func (c *client) generate(ctx context.Context, messages []modelrepo.Message, dc DecodeConfig) (string, error) {
	cs, err := c.acquire()
	if err != nil {
		return "", err
	}
	cs.Turn.Lock()
	defer cs.Turn.Unlock()

	if err := c.prime(ctx, cs, messages); err != nil {
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
func (c *client) prime(ctx context.Context, cs *modelrepo.WarmEntry[Session], messages []modelrepo.Message) error {
	plan, err := buildPromptPlan(messages, c.cfg, promptIdentity{
		ProfileID:   c.profileID,
		ModelDigest: c.modelDigest,
	})
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

var (
	_ modelrepo.LLMChatClient       = (*client)(nil)
	_ modelrepo.LLMStreamClient     = (*client)(nil)
	_ modelrepo.LLMPromptExecClient = (*client)(nil)
)

type embedClient struct {
	modelPath string
	device    string
}

func (c *embedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	// The native OpenVINO embeddings backend is in modeld. For now, the client
	// returns unsupported. Once we transport embeddings, this will call modeldconn.
	return nil, NewUnsupportedFeatureError("embed client (not implemented over transport)")
}

var _ modelrepo.LLMEmbedClient = (*embedClient)(nil)

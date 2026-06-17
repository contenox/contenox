package openvino

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/contenox/runtime/runtime/modelrepo"
)

// cachedSession is a persistent session kept warm across turns. The turn mutex
// serializes a whole EnsurePrefix -> PrefillSuffix -> Decode sequence so
// concurrent requests on the same model do not corrupt the resident KV.
type cachedSession struct {
	key  string
	sess Session
	turn sync.Mutex
}

var sessionCache = struct {
	sync.Mutex
	m map[string]*cachedSession
}{m: map[string]*cachedSession{}}

func acquireCachedSession(modelPath, modelDigest string, cfg Config) (*cachedSession, error) {
	cfg = normalizeConfig(cfg)
	key := sessionCacheKey(modelPath, modelDigest, cfg)
	sessionCache.Lock()
	if cs, ok := sessionCache.m[key]; ok {
		sessionCache.Unlock()
		return cs, nil
	}
	sessionCache.Unlock()

	s, err := newSession(modelPath, cfg)
	if err != nil {
		return nil, err
	}
	sessionCache.Lock()
	defer sessionCache.Unlock()
	if cs, ok := sessionCache.m[key]; ok {
		_ = s.Close()
		return cs, nil
	}
	cs := &cachedSession{key: key, sess: s}
	sessionCache.m[key] = cs
	return cs, nil
}

func sessionCacheKey(modelPath, modelDigest string, cfg Config) string {
	cfg = normalizeConfig(cfg)
	var b strings.Builder
	b.WriteString(modelPath)
	fmt.Fprintf(&b, "\x00model=%s\x00ctx=%d\x00prompt=%s\x00template=%s",
		modelDigest, cfg.NumCtx, cfg.PromptFormat, cfg.PromptTemplateDigest)
	return b.String()
}

func dropCachedSession(cs *cachedSession) {
	if cs == nil {
		return
	}
	sessionCache.Lock()
	if sessionCache.m[cs.key] == cs {
		delete(sessionCache.m, cs.key)
	}
	sessionCache.Unlock()
	_ = cs.sess.Close()
}

type client struct {
	modelPath       string
	profileID       string
	modelDigest     string
	cfg             Config
	maxOutputTokens int
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
	cs, err := acquireCachedSession(c.modelPath, c.modelDigest, c.cfg)
	if err != nil {
		return nil, err
	}

	cs.turn.Lock()
	if err := c.prime(ctx, cs, messages); err != nil {
		cs.turn.Unlock()
		if fatalSessionError(err) {
			dropCachedSession(cs)
		}
		return nil, err
	}
	chunks, err := cs.sess.Decode(ctx, decodeConfig(cfg, c.maxOutputTokens))
	if err != nil {
		cs.turn.Unlock()
		if fatalSessionError(err) {
			dropCachedSession(cs)
		}
		return nil, err
	}

	out := make(chan *modelrepo.StreamParcel, 16)
	go func() {
		defer close(out)
		defer cs.turn.Unlock()
		for chunk := range chunks {
			if chunk.Error != nil {
				out <- &modelrepo.StreamParcel{Error: chunk.Error}
				if fatalSessionError(chunk.Error) {
					dropCachedSession(cs)
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
	cs, err := acquireCachedSession(c.modelPath, c.modelDigest, c.cfg)
	if err != nil {
		return "", err
	}
	cs.turn.Lock()
	defer cs.turn.Unlock()

	if err := c.prime(ctx, cs, messages); err != nil {
		if fatalSessionError(err) {
			dropCachedSession(cs)
		}
		return "", err
	}
	chunks, err := cs.sess.Decode(ctx, dc)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for chunk := range chunks {
		if chunk.Error != nil {
			if fatalSessionError(chunk.Error) {
				dropCachedSession(cs)
			}
			return "", chunk.Error
		}
		b.WriteString(chunk.Text)
	}
	return strings.TrimSpace(b.String()), nil
}

// prime ensures the warm stable prefix and prefills the volatile suffix. Caller
// holds cs.turn.
func (c *client) prime(ctx context.Context, cs *cachedSession, messages []modelrepo.Message) error {
	plan, err := buildPromptPlan(messages, c.cfg, promptIdentity{
		ProfileID:   c.profileID,
		ModelDigest: c.modelDigest,
	})
	if err != nil {
		return err
	}
	if _, err := cs.sess.EnsurePrefix(ctx, plan.Stable); err != nil {
		return err
	}
	_, err = cs.sess.PrefillSuffix(ctx, plan.Volatile)
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

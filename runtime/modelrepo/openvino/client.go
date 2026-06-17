package openvino

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

const defaultMaxNewTokens = 256

type genAIClient struct {
	session           *genAISessionRef
	modelName         string
	modelDigest       string
	runtimeDigest     string
	maxOutputTokens   int
	toolProtocol      string
	reasoningProtocol string
	tracker           libtracker.ActivityTracker
	manifestMu        sync.Mutex
	lastManifest      contextasm.ContextManifest
}

func (c *genAIClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}
	toolsJSON, err := toolsToJSON(cfg.Tools)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	if len(cfg.Tools) > 0 && strings.TrimSpace(c.toolProtocol) == "" {
		return modelrepo.ChatResult{}, errors.New("openvino model profile does not declare a tool_calls.protocol")
	}

	opts := generateOptions(cfg, c.maxOutputTokens)
	opts.ParserProtocols = c.parserProtocols(len(cfg.Tools) > 0)

	plan := classifyChatContextWithIdentity(messages, toolsJSON, c.manifestIdentity())
	prompt := c.renderPrompt(plan.Messages, toolsJSON)
	result, err := c.generate(ctx, "chat", prompt, plan.Manifest, opts)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}

	visible := strings.TrimSpace(result.Text)
	var thinking string
	var toolCalls []modelrepo.ToolCall
	parsed, err := decodeParsedGeneration(result.ParsedJSON)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	if parsed.content != "" {
		visible = strings.TrimSpace(parsed.content)
	}
	thinking = parsed.thinking
	toolCalls = parsed.calls
	return modelrepo.ChatResult{
		Message:   modelrepo.Message{Role: "assistant", Content: visible, Thinking: thinking, ToolCalls: toolCalls},
		ToolCalls: toolCalls,
	}, nil
}

func (c *genAIClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	messages := []modelrepo.Message{{Role: "user", Content: prompt}}
	if s := strings.TrimSpace(systemInstruction); s != "" {
		messages = append([]modelrepo.Message{{Role: "system", Content: s}}, messages...)
	}

	temp := float64(temperature)
	opts := ovsession.GenerateOptions{
		MaxNewTokens: defaultPromptMaxTokens(c.maxOutputTokens),
		Temperature:  &temp,
	}
	plan := classifyChatContextWithIdentity(messages, "", c.manifestIdentity())
	promptText := c.renderPrompt(plan.Messages, "")
	result, err := c.generate(ctx, "prompt", promptText, plan.Manifest, opts)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(result.Text), nil
}

func (c *genAIClient) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}
	if len(cfg.Tools) > 0 {
		return nil, errors.New("openvino GenAI stream does not support tool calls yet")
	}

	plan := classifyChatContextWithIdentity(messages, "", c.manifestIdentity())
	chunks, err := c.session.Stream(ctx, c.renderPrompt(plan.Messages, ""), generateOptions(cfg, c.maxOutputTokens))
	if err != nil {
		return nil, err
	}

	out := make(chan *modelrepo.StreamParcel, 16)
	go func() {
		defer close(out)
		for chunk := range chunks {
			if chunk.Error != nil {
				out <- &modelrepo.StreamParcel{Error: chunk.Error}
				return
			}
			if chunk.Text == "" {
				continue
			}
			out <- &modelrepo.StreamParcel{Data: chunk.Text}
		}
	}()
	return out, nil
}

func (c *genAIClient) Close() error {
	if c == nil || c.session == nil {
		return nil
	}
	return c.session.Close()
}

func newGenAIClient(ctx context.Context, modelName, modelPath, modelDigest string, cfg ovsession.GenAIConfig, maxOutputTokens int, toolProtocol, reasoningProtocol string, tracker libtracker.ActivityTracker) (*genAIClient, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(modelDigest) == "" {
		var err error
		modelDigest, err = modelDirDigest(modelPath)
		if err != nil {
			return nil, err
		}
	}
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	session, err := acquireGenAISession(ctx, modelPath, modelDigest, cfg)
	if err != nil {
		return nil, fmt.Errorf("openvino GenAI client: %w", err)
	}
	return &genAIClient{
		session:           session,
		modelName:         modelName,
		modelDigest:       modelDigest,
		runtimeDigest:     openvinoRuntimeDigest(cfg),
		maxOutputTokens:   maxOutputTokens,
		toolProtocol:      strings.TrimSpace(toolProtocol),
		reasoningProtocol: strings.TrimSpace(reasoningProtocol),
		tracker:           tracker,
	}, nil
}

func (c *genAIClient) manifestIdentity() contextasm.ManifestIdentity {
	return contextasm.ManifestIdentity{
		ProfileID:            c.modelName,
		Backend:              "openvino",
		ModelDigest:          c.modelDigest,
		PromptFormat:         "openvino_chat_template",
		PromptTemplateDigest: c.modelDigest,
		RuntimeDigest:        c.runtimeDigest,
		AddBOS:               false,
	}
}

func (c *genAIClient) generate(ctx context.Context, operation, prompt string, manifest contextasm.ContextManifest, opts ovsession.GenerateOptions) (ovsession.GenAIResult, error) {
	reuseCompatible, reuseBlockedBy := c.manifestReuseCompatibility(manifest)
	reportErr, reportChange, end := c.tracker.Start(ctx, operation, "openvino",
		"model", c.modelName,
		"model_digest", c.modelDigest,
		"manifest_digest", manifest.Digest(),
		"stable_prefix_hash", manifest.StableByteHash,
		"reuse_compatible", reuseCompatible,
		"reuse_blocked_by", reuseBlockedBy,
		"prompt_bytes", len(prompt),
		"max_new_tokens", opts.MaxNewTokens,
	)
	defer end()

	result, err := c.session.Generate(ctx, prompt, opts)
	if err != nil {
		reportErr(err)
		return ovsession.GenAIResult{}, err
	}
	c.rememberManifest(manifest)
	promptTokens, promptTokenHash := c.promptTokenTelemetry(ctx, prompt)
	reportChange("cache_usage", genAICacheTelemetry(manifest, result.Metrics, promptTokens, promptTokenHash))
	return result, nil
}

func (c *genAIClient) manifestReuseCompatibility(manifest contextasm.ContextManifest) (bool, string) {
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	return c.lastManifest.CompatibleRuntime(manifest)
}

func (c *genAIClient) rememberManifest(manifest contextasm.ContextManifest) {
	c.manifestMu.Lock()
	defer c.manifestMu.Unlock()
	c.lastManifest = manifest
}

func (c *genAIClient) promptTokenTelemetry(ctx context.Context, prompt string) (int, string) {
	tokens, err := c.session.Tokenize(ctx, prompt, false)
	if err != nil || len(tokens) == 0 {
		return 0, ""
	}
	return len(tokens), contextasm.HashTokenIDs(tokens)
}

func genAICacheTelemetry(manifest contextasm.ContextManifest, m ovsession.PipelineMetrics, promptTokens int, promptTokenHash string) map[string]any {
	out := map[string]any{
		"manifest_digest":    manifest.Digest(),
		"stable_prefix_hash": manifest.StableByteHash,
		"requests":           m.Requests,
		"scheduled_requests": m.ScheduledRequests,
		"cache_usage":        m.CacheUsage,
		"max_cache_usage":    m.MaxCacheUsage,
		"avg_cache_usage":    m.AvgCacheUsage,
		"cache_size_bytes":   m.CacheSizeInBytes,
		"inference_duration": m.InferenceDuration,
	}
	if promptTokens > 0 {
		out["prompt_tokens"] = promptTokens
		out["prompt_token_hash"] = promptTokenHash
	}
	return out
}

func (c *genAIClient) parserProtocols(includeTools bool) []string {
	var protocols []string
	if includeTools && c.toolProtocol != "" {
		protocols = append(protocols, c.toolProtocol)
	}
	if c.reasoningProtocol != "" {
		protocols = append(protocols, c.reasoningProtocol)
	}
	return protocols
}

func generateOptions(cfg *modelrepo.ChatConfig, maxOutputTokens int) ovsession.GenerateOptions {
	maxTokens := defaultPromptMaxTokens(maxOutputTokens)
	if cfg != nil && cfg.MaxTokens != nil && *cfg.MaxTokens > 0 {
		maxTokens = *cfg.MaxTokens
	}
	maxTokens, _ = modelrepo.ClampMaxOutputTokens(maxTokens, maxOutputTokens)

	var temp *float64
	if cfg != nil && cfg.Temperature != nil {
		v := *cfg.Temperature
		temp = &v
	}

	var topP *float64
	if cfg != nil && cfg.TopP != nil {
		v := *cfg.TopP
		topP = &v
	}

	return ovsession.GenerateOptions{
		MaxNewTokens: maxTokens,
		Temperature:  temp,
		TopP:         topP,
	}
}

func defaultPromptMaxTokens(maxOutputTokens int) int {
	maxTokens, _ := modelrepo.ClampMaxOutputTokens(defaultMaxNewTokens, maxOutputTokens)
	return maxTokens
}

func toChatMessages(messages []modelrepo.Message) []ovsession.ChatMessage {
	out := make([]ovsession.ChatMessage, 0, len(messages))
	for _, m := range messages {
		out = append(out, ovsession.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

// renderPrompt formats messages with the model's own chat template via OpenVINO,
// falling back to a minimal ChatML layout for models without a usable template.
func (c *genAIClient) renderPrompt(messages []modelrepo.Message, toolsJSON string) string {
	if templated, err := c.session.ApplyChatTemplate(toChatMessages(messages), toolsJSON); err == nil && strings.TrimSpace(templated) != "" {
		return templated
	}
	return buildPrompt(messages)
}

func buildPrompt(messages []modelrepo.Message) string {
	var b strings.Builder
	for _, m := range messages {
		switch m.Role {
		case "system":
			fmt.Fprintf(&b, "<|system|>\n%s\n", m.Content)
		case "user":
			fmt.Fprintf(&b, "<|user|>\n%s\n", m.Content)
		case "assistant":
			fmt.Fprintf(&b, "<|assistant|>\n%s\n", m.Content)
		case "tool":
			fmt.Fprintf(&b, "<|tool|>\n%s\n", m.Content)
		default:
			fmt.Fprintf(&b, "%s\n", m.Content)
		}
	}
	b.WriteString("<|assistant|>\n")
	return b.String()
}

var (
	_ modelrepo.LLMChatClient       = (*genAIClient)(nil)
	_ modelrepo.LLMPromptExecClient = (*genAIClient)(nil)
	_ modelrepo.LLMStreamClient     = (*genAIClient)(nil)
)

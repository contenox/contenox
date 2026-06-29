package openvino

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

// openvinoProvider implements modelrepo.Provider. A model lives at
// <modelDir>/<name>/ as an OpenVINO IR. Inference runs in modeld: the provider
// builds the prompt plan and drives the session over the transport.
type openvinoProvider struct {
	name     string
	modelDir string
	caps     modelrepo.CapabilityConfig
}

func newProvider(name, modelDir string, caps modelrepo.CapabilityConfig) modelrepo.Provider {
	return &openvinoProvider{name: name, modelDir: modelDir, caps: caps}
}

func (p *openvinoProvider) GetBackendIDs() []string { return []string{"openvino"} }
func (p *openvinoProvider) ModelName() string       { return p.name }
func (p *openvinoProvider) GetID() string           { return "openvino:" + p.name }
func (p *openvinoProvider) GetType() string         { return "openvino" }
func (p *openvinoProvider) GetContextLength() int   { return p.caps.ContextLength }
func (p *openvinoProvider) GetMaxOutputTokens() int { return p.caps.MaxOutputTokens }
func (p *openvinoProvider) CanChat() bool           { return p.caps.CanChat && SessionAvailable() }
func (p *openvinoProvider) CanEmbed() bool          { return p.caps.CanEmbed && SessionAvailable() }
func (p *openvinoProvider) CanStream() bool         { return p.caps.CanStream && SessionAvailable() }
func (p *openvinoProvider) CanPrompt() bool         { return p.caps.CanPrompt && SessionAvailable() }
func (p *openvinoProvider) CanThink() bool          { return p.caps.CanThink }

func (p *openvinoProvider) GetChatConnection(ctx context.Context, _ string) (modelrepo.LLMChatClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("chat")
	}
	if !p.caps.CanChat {
		return nil, NewUnsupportedFeatureError("chat")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetStreamConnection(ctx context.Context, _ string) (modelrepo.LLMStreamClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("stream")
	}
	if !p.caps.CanStream {
		return nil, NewUnsupportedFeatureError("stream")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetPromptConnection(ctx context.Context, _ string) (modelrepo.LLMPromptExecClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("prompt")
	}
	if !p.caps.CanPrompt {
		return nil, NewUnsupportedFeatureError("prompt")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetEmbedConnection(_ context.Context, _ string) (modelrepo.LLMEmbedClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("embed")
	}
	if !p.caps.CanEmbed {
		return nil, NewUnsupportedFeatureError("embed")
	}
	return p.newEmbedClient()
}

func (p *openvinoProvider) notWired(kind string) error {
	return fmt.Errorf("%w: %s client for model %q requires a running modeld serving the openvino backend", ErrSessionUnavailable, kind, p.name)
}

func (p *openvinoProvider) newClient(ctx context.Context) (*client, error) {
	dir := filepath.Join(p.modelDir, p.name)
	profile, err := loadModelProfile(dir)
	if err != nil {
		return nil, err
	}
	if profile.ToolCalls.Protocol == "" {
		profile.ToolCalls.Protocol = curatedToolProtocol(ctx, p.name, "openvino")
	}
	reasoningParser, reasoningStream := profile.Reasoning.protocols()
	caps := profile.capabilityConfig()
	numCtx := p.caps.ContextLength
	if numCtx == 0 {
		numCtx = caps.ContextLength
	}
	maxOut := p.caps.MaxOutputTokens
	if maxOut == 0 {
		maxOut = caps.MaxOutputTokens
	}
	// Identity comes from the model's own files (no hardcoded format): the digest
	// content-addresses the model, and the template digest tracks its Jinja chat
	// template, which modeld applies via the IR tokenizer.
	modelDigest, templateDigest := modelIdentity(dir)
	adapters, err := resolveProfileAdapters(dir, profile.Adapters)
	if err != nil {
		return nil, err
	}
	profileID := p.name
	cfg := Config{
		NumCtx:               numCtx,
		PromptFormat:         "openvino-chat-template",
		PromptTemplateDigest: templateDigest,
	}
	ref := modeldconn.ModelRef{Name: p.name, Type: "openvino", Digest: modelDigest, Path: dir, Adapters: adapters}
	backendID := backendVersion()
	// Capacity facts from modeld's Describe, kept so a context overflow can be
	// explained with the device's real limits instead of raw token counts.
	var deviceKind string
	var freeBytes int64
	if info, derr := modeldconn.Describe(ctx, ref, transport.Config(cfg)); derr == nil {
		if info.EffectiveContext > 0 {
			cfg = clampContextForModeld(cfg, info.EffectiveContext)
		} else {
			cfg = normalizeConfig(cfg)
		}
		cfg.PlannerEffectiveContext = transport.ResolvePlannerEffectiveContext(cfg.PlannerEffectiveContext, cfg.NumCtx, info)
		deviceKind = info.DeviceKind
		freeBytes = info.FreeBytes
		if v := backendVersionFromModelInfo(info); v != "" {
			backendID = v
		}
	} else {
		cfg = normalizeConfig(cfg)
	}
	return &client{
		modelName:       p.name,
		modelPath:       dir,
		profileID:       profileID,
		modelDigest:     modelDigest,
		backendVersion:  backendID,
		toolProtocol:    profile.ToolCalls.Protocol,
		reasoningParser: reasoningParser,
		reasoningStream: reasoningStream,
		cfg:             cfg,
		adapters:        adapters,
		maxOutputTokens: maxOut,
		deviceKind:      deviceKind,
		freeBytes:       freeBytes,
	}, nil
}

func curatedToolProtocol(ctx context.Context, modelName, backendType string) string {
	d, err := modelregistry.New(nil).Resolve(ctx, modelName)
	if err != nil || d.BackendType() != backendType {
		return ""
	}
	if d.ToolProtocol == "" || !toolCallProtocolKnown(d.ToolProtocol) {
		return ""
	}
	return d.ToolProtocol
}

func (p *openvinoProvider) newEmbedClient() (*embedClient, error) {
	dir := filepath.Join(p.modelDir, p.name)
	if _, err := loadModelProfile(dir); err != nil {
		return nil, err
	}
	modelDigest, _ := modelIdentity(dir)
	return &embedClient{
		modelName:   p.name,
		modelPath:   dir,
		modelDigest: modelDigest,
	}, nil
}

func backendVersion() string {
	return "OpenVINO GenAI"
}

func backendVersionFromModelInfo(info transport.ModelInfo) string {
	switch {
	case info.RuntimeName != "" && info.RuntimeDigest != "":
		return info.RuntimeName + "@" + info.RuntimeDigest
	case info.RuntimeDigest != "":
		return info.RuntimeDigest
	case info.RuntimeName != "":
		return info.RuntimeName
	default:
		return ""
	}
}

func clampContext(cfg Config, cap int) Config {
	if cfg.PromptFormat == "" {
		cfg.PromptFormat = "openvino-chat-template"
	}
	if cap > 0 && (cfg.NumCtx <= 0 || cfg.NumCtx > cap) {
		cfg.NumCtx = cap
	}
	return normalizeConfig(cfg)
}

func clampContextForModeld(cfg Config, cap int) Config {
	cfg = clampContext(cfg, cap)
	if cap > modeldCapacitySafetyTokens && cfg.NumCtx > cap-modeldCapacitySafetyTokens {
		cfg.NumCtx = cap - modeldCapacitySafetyTokens
	}
	return cfg
}

const modeldCapacitySafetyTokens = 64

var _ modelrepo.Provider = (*openvinoProvider)(nil)

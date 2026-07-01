package llama

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
)

// provider implements modelrepo.Provider for the llama.cpp GGUF compatibility node.
// A model lives at <modelDir>/<name>/model.gguf with an optional
// contenox-llama.json runtime profile beside it.
type provider struct {
	name     string
	modelDir string
	caps     modelrepo.CapabilityConfig
}

func newProvider(name, modelDir string, caps modelrepo.CapabilityConfig) modelrepo.Provider {
	return &provider{name: name, modelDir: modelDir, caps: caps}
}

func (p *provider) GetBackendIDs() []string { return []string{"llama"} }
func (p *provider) ModelName() string       { return p.name }
func (p *provider) GetID() string           { return "llama:" + p.name }
func (p *provider) GetType() string         { return "llama" }
func (p *provider) GetContextLength() int   { return p.caps.ContextLength }
func (p *provider) GetMaxOutputTokens() int { return p.caps.MaxOutputTokens }
func (p *provider) CanChat() bool           { return SessionAvailable() }
func (p *provider) CanEmbed() bool          { return EmbedAvailable() }
func (p *provider) CanStream() bool         { return SessionAvailable() }
func (p *provider) CanPrompt() bool         { return SessionAvailable() }
func (p *provider) CanThink() bool          { return p.caps.CanThink }

func (p *provider) GetChatConnection(ctx context.Context, _ string) (modelrepo.LLMChatClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("chat")
	}
	return p.newClient(ctx)
}

func (p *provider) GetStreamConnection(ctx context.Context, _ string) (modelrepo.LLMStreamClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("stream")
	}
	return p.newClient(ctx)
}

func (p *provider) GetPromptConnection(ctx context.Context, _ string) (modelrepo.LLMPromptExecClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("prompt")
	}
	return p.newClient(ctx)
}

func (p *provider) GetEmbedConnection(_ context.Context, _ string) (modelrepo.LLMEmbedClient, error) {
	if !EmbedAvailable() {
		return nil, p.notWired("embed")
	}
	dir := filepath.Join(p.modelDir, p.name)
	profile, err := loadModelProfile(dir)
	if err != nil {
		return nil, err
	}
	modelPath := filepath.Join(dir, "model.gguf")
	modelDigest := profile.ModelDigest
	if modelDigest == "" {
		modelDigest, err = modelFileDigest(modelPath)
		if err != nil {
			return nil, err
		}
	}
	return &embedClient{
		modelName:   p.name,
		modelPath:   modelPath,
		modelDigest: modelDigest,
		cfg:         profile.config(),
	}, nil
}

func (p *provider) newClient(ctx context.Context) (*client, error) {
	dir := filepath.Join(p.modelDir, p.name)
	profile, err := loadModelProfile(dir)
	if err != nil {
		return nil, err
	}
	if profile.ToolCalls.Protocol == "" {
		profile.ToolCalls.Protocol = curatedToolProtocol(ctx, p.name, "llama")
	}
	if profile.Reasoning.Protocol == "" {
		profile.Reasoning.Protocol, profile.Reasoning.Format = curatedReasoning(ctx, p.name, "llama")
	} else if profile.Reasoning.Format == "" {
		_, profile.Reasoning.Format = curatedReasoning(ctx, p.name, "llama")
	}
	modelPath := filepath.Join(dir, "model.gguf")
	modelDigest := profile.ModelDigest
	if modelDigest == "" {
		modelDigest, err = modelFileDigest(modelPath)
		if err != nil {
			return nil, err
		}
	}
	adapters, err := resolveProfileAdapters(dir, profile.Adapters)
	if err != nil {
		return nil, err
	}
	profileID := profile.ProfileID
	if profileID == "" {
		profileID = p.name
	}
	baseCfg := profile.config()
	cfg := baseCfg
	if !profile.explicitRuntimeContext() {
		cfg.NumCtx = 0
	}
	if p.caps.ContextLength > 0 {
		cfg = clampContext(cfg, p.caps.ContextLength)
	} else if profile.explicitRuntimeContext() {
		cfg = normalizeConfig(cfg)
	}
	ref := modeldconn.ModelRef{Name: p.name, Type: "llama", Digest: modelDigest, Path: modelPath, Adapters: adapters}
	backendID := backendVersion()
	// Capacity facts from modeld's Describe, kept so a context overflow can be
	// explained with the device's real limits instead of raw token counts.
	var deviceKind string
	var freeBytes int64
	if sessionFactory == nil {
		describeCfg := profile.describeConfig()
		if p.caps.ContextLength > 0 && describeCfg.NumCtx > p.caps.ContextLength {
			describeCfg.NumCtx = p.caps.ContextLength
		}
		if info, derr := modeldconn.Describe(ctx, ref, transport.Config(describeCfg)); derr == nil {
			cfg = applyModeldInfoToConfig(cfg, info)
			deviceKind = info.DeviceKind
			freeBytes = info.FreeBytes
			if v := backendVersionFromModelInfo(info); v != "" {
				backendID = v
			}
		} else {
			cfg = normalizeConfig(baseCfg)
		}
	} else {
		cfg = normalizeConfig(cfg)
	}
	return &client{
		modelName:         p.name,
		modelPath:         modelPath,
		profileID:         profileID,
		modelDigest:       modelDigest,
		backendVersion:    backendID,
		cfg:               cfg,
		adapters:          adapters,
		maxOutputTokens:   p.caps.MaxOutputTokens,
		toolProtocol:      profile.ToolCalls.Protocol,
		reasoningProtocol: profile.Reasoning.Protocol,
		deviceKind:        deviceKind,
		freeBytes:         freeBytes,
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

func curatedReasoningProtocol(ctx context.Context, modelName, backendType string) string {
	protocol, _ := curatedReasoning(ctx, modelName, backendType)
	return protocol
}

func curatedReasoningFormat(ctx context.Context, modelName, backendType string) string {
	_, format := curatedReasoning(ctx, modelName, backendType)
	return format
}

func curatedReasoning(ctx context.Context, modelName, backendType string) (protocol, format string) {
	d, err := modelregistry.New(nil).Resolve(ctx, modelName)
	if err != nil || d.BackendType() != backendType {
		return "", ""
	}
	if d.ReasoningProtocol == "" || d.ReasoningFormat == "" || !reasoningProtocolKnown(d.ReasoningProtocol) {
		return "", ""
	}
	return d.ReasoningProtocol, d.ReasoningFormat
}

func (p *provider) notWired(kind string) error {
	return fmt.Errorf("%w: %s client for model %q requires a running modeld serving the llama backend", ErrSessionUnavailable, kind, p.name)
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

func applyModeldInfoToConfig(cfg Config, info transport.ModelInfo) Config {
	if info.EffectiveContext > 0 {
		cfg = clampContextForModeld(cfg, info.EffectiveContext)
	}
	cfg.PlannerEffectiveContext = transport.ResolvePlannerEffectiveContext(cfg.PlannerEffectiveContext, cfg.NumCtx, info)
	if info.RequestedGpuLayers > 0 || info.ResolvedGpuLayers > 0 {
		cfg.NumGpuLayers = info.ResolvedGpuLayers
	}
	return cfg
}

func clampContext(cfg Config, cap int) Config {
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

var (
	_ modelrepo.Provider            = (*provider)(nil)
	_ modelrepo.LLMChatClient       = (*client)(nil)
	_ modelrepo.LLMStreamClient     = (*client)(nil)
	_ modelrepo.LLMPromptExecClient = (*client)(nil)
	_ modelrepo.LLMEmbedClient      = (*embedClient)(nil)
)

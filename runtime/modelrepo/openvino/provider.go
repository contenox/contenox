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
//
// When target.Endpoint != "", sessions and Describe are routed to that specific
// modeld node (supports remote specialist GPU nodes registered as "modeld" backends).
type openvinoProvider struct {
	name     string
	modelDir string
	caps     modelrepo.CapabilityConfig
	target   modeldconn.ModeldTarget
}

func newProvider(name, modelDir string, caps modelrepo.CapabilityConfig) modelrepo.Provider {
	return &openvinoProvider{name: name, modelDir: modelDir, caps: caps}
}

// NewProviderForTarget used for providers backed by a specific (possibly remote) modeld node.
// modelDir may be "" (the node resolves by name/digest).
func NewProviderForTarget(name, modelDir string, caps modelrepo.CapabilityConfig, target modeldconn.ModeldTarget) modelrepo.Provider {
	return &openvinoProvider{name: name, modelDir: modelDir, caps: caps, target: target}
}

func (p *openvinoProvider) GetBackendIDs() []string {
	if p.target.BackendID != "" {
		return []string{p.target.BackendID}
	}
	return []string{"openvino"}
}
func (p *openvinoProvider) ModelName() string       { return p.name }
func (p *openvinoProvider) GetID() string           { return "openvino:" + p.name }
func (p *openvinoProvider) GetType() string         { return "openvino" }
func (p *openvinoProvider) GetContextLength() int   { return p.caps.ContextLength }
func (p *openvinoProvider) GetMaxOutputTokens() int { return p.caps.MaxOutputTokens }

// isTargeted reports whether this provider is bound to a specific (possibly
// remote) modeld node rather than the implicit local lease. SessionAvailable
// only reflects THIS machine's own modeld lease state (modeldconn.ServeableBackend())
// — meaningless for a remote or otherwise explicit target, whose actual
// availability was already proven at reconcile time (see
// runtime/runtimestate/adapter.go, which only constructs a targeted provider
// once ListModels succeeded against that live node) and is carried here via p.caps.
func (p *openvinoProvider) isTargeted() bool {
	return p.target.BackendID != "" || p.target.Endpoint != ""
}

func (p *openvinoProvider) CanChat() bool {
	if p.isTargeted() {
		return p.caps.CanChat
	}
	return p.caps.CanChat && SessionAvailable()
}

func (p *openvinoProvider) CanEmbed() bool {
	if p.isTargeted() {
		return p.caps.CanEmbed
	}
	return p.caps.CanEmbed && SessionAvailable()
}

func (p *openvinoProvider) CanStream() bool {
	if p.isTargeted() {
		return p.caps.CanStream
	}
	return p.caps.CanStream && SessionAvailable()
}

func (p *openvinoProvider) CanPrompt() bool {
	if p.isTargeted() {
		return p.caps.CanPrompt
	}
	return p.caps.CanPrompt && SessionAvailable()
}

func (p *openvinoProvider) CanThink() bool { return p.caps.CanThink }

func (p *openvinoProvider) CanVision() bool { return p.caps.CanVision }

func (p *openvinoProvider) GetChatConnection(ctx context.Context, _ string) (modelrepo.LLMChatClient, error) {
	if !p.isTargeted() && !SessionAvailable() {
		return nil, p.notWired("chat")
	}
	if !p.caps.CanChat {
		return nil, NewUnsupportedFeatureError("chat")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetStreamConnection(ctx context.Context, _ string) (modelrepo.LLMStreamClient, error) {
	if !p.isTargeted() && !SessionAvailable() {
		return nil, p.notWired("stream")
	}
	if !p.caps.CanStream {
		return nil, NewUnsupportedFeatureError("stream")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetPromptConnection(ctx context.Context, _ string) (modelrepo.LLMPromptExecClient, error) {
	if !p.isTargeted() && !SessionAvailable() {
		return nil, p.notWired("prompt")
	}
	if !p.caps.CanPrompt {
		return nil, NewUnsupportedFeatureError("prompt")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetEmbedConnection(_ context.Context, _ string) (modelrepo.LLMEmbedClient, error) {
	if !p.isTargeted() && !SessionAvailable() {
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
	var dir string
	if p.modelDir != "" {
		dir = filepath.Join(p.modelDir, p.name)
	} else if p.target.Endpoint == "" {
		return nil, fmt.Errorf("openvino provider for %q has no modelDir and no target endpoint", p.name)
	}
	profile, err := loadModelProfile(dir)
	if err != nil {
		return nil, err
	}
	if profile.ToolCalls.Protocol == "" {
		profile.ToolCalls.Protocol = curatedToolProtocol(ctx, p.name, "openvino")
	}
	reasoningParser, reasoningStream := profile.Reasoning.protocols()
	caps := profile.capabilityConfig()
	if dir != "" && visionEncoderPresent(dir) {
		caps.CanVision = true
	}
	maxOut := p.caps.MaxOutputTokens
	if maxOut == 0 {
		maxOut = caps.MaxOutputTokens
	}
	// Identity comes from the model's own files (no hardcoded format): the digest
	// content-addresses the model, and the template digest tracks its Jinja chat
	// template, which modeld applies via the IR tokenizer.
	var modelDigest, templateDigest string
	var adapters []transport.AdapterSpec
	if dir != "" {
		modelDigest, templateDigest = modelIdentity(dir)
		var err error
		adapters, err = resolveProfileAdapters(dir, profile.Adapters)
		if err != nil {
			return nil, err
		}
	} else if p.target.Endpoint != "" {
		// Remote: rely on wire identity + node; adapters carried in ModelRef when known.
	}
	profileID := p.name
	// NumCtx stays 0 (auto) end-to-end: modeld's authoritative, post-eviction
	// resolveConfigFromInfo computes the window fresh at OpenSession. The
	// catalog's declared context length is the trained ceiling, which modeld
	// derives itself from the model files (ModelMaxCtx) — pre-baking it (or a
	// Describe answer taken while another session was still resident) here
	// would freeze the session at a stale ceiling. See capacity.HardContextLimit.
	cfg := normalizeConfig(Config{
		PromptFormat:         "openvino-chat-template",
		PromptTemplateDigest: templateDigest,
	})
	if requestedCtx := modelrepo.RequestedContextLengthFromContext(ctx); requestedCtx > 0 {
		cfg.NumCtx = requestedCtx
		cfg = normalizeConfig(cfg)
	}
	ref := modeldconn.ModelRef{Name: p.name, Type: "openvino", Digest: modelDigest, Path: dir, Adapters: adapters}
	backendID := backendVersion()
	// Capacity facts from modeld's Describe, kept so a context overflow can be
	// explained with the device's real limits instead of raw token counts.
	// Informational only: never written back into cfg.
	var deviceKind string
	var freeBytes int64
	var describedEffectiveContext, describedPlannerContext, describedModelMaxContext int
	var derr error
	var info transport.ModelInfo
	if p.target.Endpoint != "" {
		info, derr = modeldconn.DescribeTarget(ctx, p.target, ref, transport.Config(cfg))
	} else {
		info, derr = modeldconn.Describe(ctx, ref, transport.Config(cfg))
	}
	// Pre-Describe best effort: catalog caps or the vision encoder shipped in
	// the snapshot. A successful Describe below overrides it — the live
	// daemon's answer is authoritative (see applyModeldTemplateCapabilities).
	supportsVision := p.caps.CanVision || caps.CanVision
	if derr == nil {
		deviceKind = info.DeviceKind
		freeBytes = info.FreeBytes
		describedEffectiveContext = info.EffectiveContext
		describedPlannerContext = info.PlannerEffectiveContext
		describedModelMaxContext = info.ModelMaxContext
		applyModeldTemplateCapabilities(&caps, info)
		supportsVision = info.SupportsVision
		if v := backendVersionFromModelInfo(info); v != "" {
			backendID = v
		}
	}
	return &client{
		modelName:                 p.name,
		modelPath:                 dir,
		profileID:                 profileID,
		modelDigest:               modelDigest,
		backendVersion:            backendID,
		toolProtocol:              profile.ToolCalls.Protocol,
		reasoningParser:           reasoningParser,
		reasoningStream:           reasoningStream,
		cfg:                       cfg,
		adapters:                  adapters,
		maxOutputTokens:           maxOut,
		deviceKind:                deviceKind,
		freeBytes:                 freeBytes,
		supportsThinking:          p.caps.CanThink || caps.CanThink,
		supportsVision:            supportsVision,
		describedEffectiveContext: describedEffectiveContext,
		describedPlannerContext:   describedPlannerContext,
		describedModelMaxContext:  describedModelMaxContext,
		target:                    p.target,
	}, nil
}

func applyModeldTemplateCapabilities(caps *modelrepo.CapabilityConfig, info transport.ModelInfo) {
	if info.ChatTemplateSupportsThinking || info.ChatTemplateSupportsReasoningEffort || info.ChatTemplateReasoningFormat != "" {
		caps.CanThink = true
	}
	// Vision is assigned, not OR'd: a live daemon's Describe answer is the
	// authoritative truth and must be able to downgrade the offline
	// file-presence signal — an older daemon without VLM support reports false
	// (or omits the field) and would silently drop image parts if the resolver
	// still routed images to it.
	caps.CanVision = info.SupportsVision
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
	var dir string
	if p.modelDir != "" {
		dir = filepath.Join(p.modelDir, p.name)
		if _, err := loadModelProfile(dir); err != nil {
			return nil, err
		}
	}
	var modelDigest string
	if dir != "" {
		modelDigest, _ = modelIdentity(dir)
	}
	return &embedClient{
		modelName:   p.name,
		modelPath:   dir,
		modelDigest: modelDigest,
		target:      p.target,
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

var _ modelrepo.Provider = (*openvinoProvider)(nil)

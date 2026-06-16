package openvino

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/openvino/ovsession"
)

// openvinoProvider implements modelrepo.Provider. It mirrors the local provider:
// a model lives at <modelDir>/<name>/ as an OpenVINO IR (openvino_model.xml).
type openvinoProvider struct {
	name     string
	modelDir string
	caps     modelrepo.CapabilityConfig
	tracker  libtracker.ActivityTracker
}

func newProvider(name, modelDir string, caps modelrepo.CapabilityConfig, tracker libtracker.ActivityTracker) modelrepo.Provider {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	return &openvinoProvider{name: name, modelDir: modelDir, caps: caps, tracker: tracker}
}

func (p *openvinoProvider) GetBackendIDs() []string { return []string{"openvino"} }
func (p *openvinoProvider) ModelName() string       { return p.name }
func (p *openvinoProvider) GetID() string           { return "openvino:" + p.name }
func (p *openvinoProvider) GetType() string         { return "openvino" }
func (p *openvinoProvider) GetContextLength() int   { return p.caps.ContextLength }
func (p *openvinoProvider) GetMaxOutputTokens() int { return p.caps.MaxOutputTokens }
func (p *openvinoProvider) CanChat() bool           { return ovsession.GenAIAvailable }
func (p *openvinoProvider) CanEmbed() bool          { return false }
func (p *openvinoProvider) CanStream() bool         { return ovsession.GenAIAvailable }
func (p *openvinoProvider) CanPrompt() bool         { return ovsession.GenAIAvailable }
func (p *openvinoProvider) CanThink() bool          { return p.caps.CanThink }

func (p *openvinoProvider) GetChatConnection(ctx context.Context, _ string) (modelrepo.LLMChatClient, error) {
	if !p.CanChat() {
		return nil, p.notWired("chat")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetStreamConnection(ctx context.Context, _ string) (modelrepo.LLMStreamClient, error) {
	if !p.CanStream() {
		return nil, p.notWired("stream")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetPromptConnection(ctx context.Context, _ string) (modelrepo.LLMPromptExecClient, error) {
	if !p.CanPrompt() {
		return nil, p.notWired("prompt")
	}
	return p.newClient(ctx)
}

func (p *openvinoProvider) GetEmbedConnection(_ context.Context, _ string) (modelrepo.LLMEmbedClient, error) {
	return nil, p.notWired("embed")
}

func (p *openvinoProvider) notWired(kind string) error {
	return fmt.Errorf("openvino %s client for model %q is not wired in this build; compile with openvino and openvino_genai tags for prompt/chat", kind, p.name)
}

func (p *openvinoProvider) newClient(ctx context.Context) (*genAIClient, error) {
	modelPath := filepath.Join(p.modelDir, p.name)
	profile, err := loadModelProfile(modelPath)
	if err != nil {
		return nil, err
	}
	caps := mergeCapabilities(p.caps, profile.capabilityConfig())
	device := openvinoDevice(profile.Device)
	digest, err := modelDirDigest(modelPath)
	if err != nil {
		return nil, err
	}
	return newGenAIClient(ctx, p.name, modelPath, digest, profile.sessionConfig(device), caps.MaxOutputTokens, profile.ToolCalls.Protocol, profile.Reasoning.Protocol, p.tracker)
}

func openvinoDevice(profileDevice string) string {
	if device := os.Getenv("CONTENOX_OPENVINO_DEVICE"); device != "" {
		return device
	}
	if profileDevice != "" {
		return profileDevice
	}
	if device := os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE"); device != "" {
		return device
	}
	return "CPU"
}

var _ modelrepo.Provider = (*openvinoProvider)(nil)

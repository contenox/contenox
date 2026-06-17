package openvino

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/contenox/runtime/runtime/modelrepo"
)

// openvinoProvider implements modelrepo.Provider. A model lives at
// <modelDir>/<name>/ as an OpenVINO IR (openvino_model.xml). Inference runs in
// modeld: the provider builds the prompt plan and drives the session over the
// transport.
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
func (p *openvinoProvider) CanChat() bool           { return SessionAvailable() }
func (p *openvinoProvider) CanEmbed() bool          { return false }
func (p *openvinoProvider) CanStream() bool         { return SessionAvailable() }
func (p *openvinoProvider) CanPrompt() bool         { return SessionAvailable() }
func (p *openvinoProvider) CanThink() bool          { return p.caps.CanThink }

func (p *openvinoProvider) GetChatConnection(_ context.Context, _ string) (modelrepo.LLMChatClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("chat")
	}
	return p.newClient()
}

func (p *openvinoProvider) GetStreamConnection(_ context.Context, _ string) (modelrepo.LLMStreamClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("stream")
	}
	return p.newClient()
}

func (p *openvinoProvider) GetPromptConnection(_ context.Context, _ string) (modelrepo.LLMPromptExecClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("prompt")
	}
	return p.newClient()
}

func (p *openvinoProvider) GetEmbedConnection(_ context.Context, _ string) (modelrepo.LLMEmbedClient, error) {
	return nil, p.notWired("embed")
}

func (p *openvinoProvider) notWired(kind string) error {
	return fmt.Errorf("%w: %s client for model %q (modeld not available)", ErrSessionUnavailable, kind, p.name)
}

func (p *openvinoProvider) newClient() (*client, error) {
	dir := filepath.Join(p.modelDir, p.name)
	profile, err := loadModelProfile(dir)
	if err != nil {
		return nil, err
	}
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
	return &client{
		modelPath:       dir,
		profileID:       p.name,
		modelDigest:     modelDigest,
		cfg: Config{
			NumCtx:               numCtx,
			PromptFormat:         "openvino-chat-template",
			PromptTemplateDigest: templateDigest,
		},
		maxOutputTokens: maxOut,
	}, nil
}

var _ modelrepo.Provider = (*openvinoProvider)(nil)

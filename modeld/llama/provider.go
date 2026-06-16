package llama

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/contenox/runtime/modeld"
)

// provider implements modeld.Provider for the graduated llama.cpp local node.
// A model lives at <modelDir>/<name>/model.gguf with an optional
// contenox-llama.json runtime profile beside it.
type provider struct {
	name     string
	modelDir string
	caps     modeld.CapabilityConfig
}

func newProvider(name, modelDir string, caps modeld.CapabilityConfig) modeld.Provider {
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

func (p *provider) GetChatConnection(_ context.Context, _ string) (modeld.LLMChatClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("chat")
	}
	return p.newClient()
}

func (p *provider) GetStreamConnection(_ context.Context, _ string) (modeld.LLMStreamClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("stream")
	}
	return p.newClient()
}

func (p *provider) GetPromptConnection(_ context.Context, _ string) (modeld.LLMPromptExecClient, error) {
	if !SessionAvailable() {
		return nil, p.notWired("prompt")
	}
	return p.newClient()
}

func (p *provider) GetEmbedConnection(_ context.Context, _ string) (modeld.LLMEmbedClient, error) {
	if !EmbedAvailable() {
		return nil, p.notWired("embed")
	}
	dir := filepath.Join(p.modelDir, p.name)
	profile, err := loadModelProfile(dir)
	if err != nil {
		return nil, err
	}
	return &embedClient{modelPath: filepath.Join(dir, "model.gguf"), cfg: profile.config()}, nil
}

func (p *provider) newClient() (*client, error) {
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
	profileID := profile.ProfileID
	if profileID == "" {
		profileID = p.name
	}
	return &client{
		modelPath:       modelPath,
		profileID:       profileID,
		modelDigest:     modelDigest,
		backendVersion:  backendVersion(),
		cfg:             profile.config(),
		maxOutputTokens: p.caps.MaxOutputTokens,
	}, nil
}

func (p *provider) notWired(kind string) error {
	return fmt.Errorf("%w: %s client for model %q is not wired in this build; compile with -tags llamanode", ErrSessionUnavailable, kind, p.name)
}

var (
	_ modeld.Provider            = (*provider)(nil)
	_ modeld.LLMChatClient       = (*client)(nil)
	_ modeld.LLMStreamClient     = (*client)(nil)
	_ modeld.LLMPromptExecClient = (*client)(nil)
	_ modeld.LLMEmbedClient      = (*embedClient)(nil)
)

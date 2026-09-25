package modeld

import (
	"context"
	"fmt"

	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/libtracker"
)

// ModeldProvider exposes a model served by native daemon backends.
type ModeldProvider struct {
	Name            string
	ID              string
	ContextLength   int
	MaxOutputTokens int
	SupportsChat    bool
	SupportsEmbed   bool
	SupportsStream  bool
	SupportsPrompt  bool
	SupportsThink   bool
	SupportsVision  bool
	Backends        []string
	tracker         libtracker.ActivityTracker
}

// NewModeldProvider constructs a provider from daemon-discovered capabilities.
func NewModeldProvider(name string, backends []string, caps modelrepo.CapabilityConfig, tracker libtracker.ActivityTracker) modelrepo.Provider {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	return &ModeldProvider{
		Name:            name,
		ID:              "modeld:" + name,
		ContextLength:   caps.ContextLength,
		MaxOutputTokens: caps.MaxOutputTokens,
		SupportsChat:    caps.CanChat,
		SupportsEmbed:   caps.CanEmbed,
		SupportsStream:  caps.CanStream,
		SupportsPrompt:  caps.CanPrompt,
		SupportsThink:   caps.CanThink,
		SupportsVision:  caps.CanVision,
		Backends:        backends,
		tracker:         tracker,
	}
}

func (p *ModeldProvider) GetBackendIDs() []string {
	return p.Backends
}

func (p *ModeldProvider) ModelName() string {
	return p.Name
}

func (p *ModeldProvider) GetID() string {
	return p.ID
}

func (p *ModeldProvider) GetType() string {
	return "modeld"
}

func (p *ModeldProvider) GetContextLength() int   { return p.ContextLength }
func (p *ModeldProvider) GetMaxOutputTokens() int { return p.MaxOutputTokens }

func (p *ModeldProvider) CanChat() bool {
	return p.SupportsChat
}

func (p *ModeldProvider) CanEmbed() bool {
	return p.SupportsEmbed
}

func (p *ModeldProvider) CanStream() bool {
	return p.SupportsStream
}

func (p *ModeldProvider) CanPrompt() bool {
	return p.SupportsPrompt
}

func (p *ModeldProvider) CanThink() bool {
	return p.SupportsThink
}

func (p *ModeldProvider) CanVision() bool {
	return p.SupportsVision
}

func (p *ModeldProvider) CanAudio() bool {
	return false
}

func (p *ModeldProvider) GetChatConnection(ctx context.Context, backendID string) (modelrepo.LLMChatClient, error) {
	if !p.CanChat() {
		return nil, fmt.Errorf("provider %s (model %s) does not support chat", p.GetID(), p.ModelName())
	}
	return &ModeldChatClient{
		modelName:       p.ModelName(),
		backendID:       backendID,
		maxOutputTokens: p.MaxOutputTokens,
		supportsThink:   p.SupportsThink,
		tracker:         p.tracker,
	}, nil
}

func (p *ModeldProvider) GetEmbedConnection(ctx context.Context, backendID string) (modelrepo.LLMEmbedClient, error) {
	if !p.CanEmbed() {
		return nil, fmt.Errorf("provider %s (model %s) does not support embeddings", p.GetID(), p.ModelName())
	}
	return &ModeldEmbedClient{
		modelName: p.ModelName(),
		backendID: backendID,
		tracker:   p.tracker,
	}, nil
}

func (p *ModeldProvider) GetPromptConnection(ctx context.Context, backendID string) (modelrepo.LLMPromptExecClient, error) {
	if !p.CanPrompt() {
		return nil, fmt.Errorf("provider %s (model %s) does not support prompting", p.GetID(), p.ModelName())
	}
	return &ModeldPromptClient{
		modelName:       p.ModelName(),
		backendID:       backendID,
		maxOutputTokens: p.MaxOutputTokens,
		supportsThink:   p.SupportsThink,
		tracker:         p.tracker,
	}, nil
}

func (p *ModeldProvider) GetStreamConnection(ctx context.Context, backendID string) (modelrepo.LLMStreamClient, error) {
	if !p.CanStream() {
		return nil, fmt.Errorf("provider %s (model %s) does not support streaming", p.GetID(), p.ModelName())
	}
	return &ModeldStreamClient{
		modelName:       p.ModelName(),
		backendID:       backendID,
		maxOutputTokens: p.MaxOutputTokens,
		supportsThink:   p.SupportsThink,
		tracker:         p.tracker,
	}, nil
}

var _ modelrepo.Provider = (*ModeldProvider)(nil)

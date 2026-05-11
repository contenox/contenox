package llmresolver

import (
	"github.com/contenox/contenox/libtracker"
)

type Request struct {
	ProviderTypes []string
	ModelNames    []string
	ContextLength int
	Tracker       libtracker.ActivityTracker
}

type EmbedRequest struct {
	ModelName    string
	ProviderType string
	Tracker      libtracker.ActivityTracker
}

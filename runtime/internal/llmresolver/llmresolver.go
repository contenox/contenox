package llmresolver

import (
	"github.com/contenox/runtime/libtracker"
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

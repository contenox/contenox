package chatservice

import (
	"context"
	"fmt"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/taskengine"
)

type activityTrackerDecorator struct {
	service Service
	tracker libtracker.ActivityTracker
}

// OpenAIChatCompletions implements Service.
func (d *activityTrackerDecorator) OpenAIChatCompletions(ctx context.Context, chainID string, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error) {
	// Start tracking with relevant context
	reportErr, _, endFn := d.tracker.Start(
		ctx,
		"openai_chat_completions",
		"chat",
		"chain_id", chainID,
		"model", req.Model,
		"message_count", len(req.Messages),
		"max_tokens", req.MaxTokens,
	)
	defer endFn()

	// Execute the operation
	resp, traces, err := d.service.OpenAIChatCompletions(ctx, chainID, req)
	if err != nil {
		// Report error with additional context
		reportErr(fmt.Errorf("chat completions failed: %w", err))
		return nil, traces, err
	}

	return resp, traces, nil
}

// WithActivityTracker creates a new decorated service that tracks activity
func WithActivityTracker(service Service, tracker libtracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}

// Ensure the decorator implements the Service interface
var _ Service = (*activityTrackerDecorator)(nil)

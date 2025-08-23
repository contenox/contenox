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
func (d *activityTrackerDecorator) OpenAIChatCompletions(ctx context.Context, req taskengine.OpenAIChatRequest) (*taskengine.OpenAIChatResponse, []taskengine.CapturedStateUnit, error) {
	// Start tracking with relevant context
	reportErr, _, endFn := d.tracker.Start(
		ctx,
		"openai_chat_completions",
		"chat",
		"model", req.Model,
		"message_count", len(req.Messages),
		"max_tokens", req.MaxTokens,
	)
	defer endFn()

	// Execute the operation
	resp, traces, err := d.service.OpenAIChatCompletions(ctx, req)
	if err != nil {
		// Report error with additional context
		reportErr(fmt.Errorf("chat completions failed: %w", err))
		return nil, traces, err
	}

	return resp, traces, nil
}

// SetTaskChainID implements Service.
func (d *activityTrackerDecorator) SetTaskChainID(ctx context.Context, taskChainID string) error {
	// Start tracking with relevant context
	reportErr, _, endFn := d.tracker.Start(
		ctx,
		"set_task_chain_id",
		"chat",
		"chain_id", taskChainID,
	)
	defer endFn()

	// Execute the operation
	err := d.service.SetTaskChainID(ctx, taskChainID)
	if err != nil {
		// Report error with additional context
		reportErr(fmt.Errorf("failed to set task chain ID: %w", err))
		return err
	}

	return nil
}

// GetTaskChainID implements Service.
func (d *activityTrackerDecorator) GetTaskChainID(ctx context.Context) (string, error) {
	// Start tracking with relevant context
	reportErr, _, endFn := d.tracker.Start(
		ctx,
		"get_task_chain_id",
		"chat",
	)
	defer endFn()

	// Execute the operation
	chainID, err := d.service.GetTaskChainID(ctx)
	if err != nil {
		// Report error with additional context
		reportErr(fmt.Errorf("failed to get task chain ID: %w", err))
		return "", err
	}

	return chainID, nil
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

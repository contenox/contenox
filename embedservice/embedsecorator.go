package embedservice

import (
	"context"
	"fmt"

	"github.com/contenox/activitytracker"
)

type activityTrackerDecorator struct {
	service Service
	tracker activitytracker.ActivityTracker
}

func (d *activityTrackerDecorator) Embed(ctx context.Context, text string) ([]float64, error) {
	// Start tracking with relevant context
	reportErr, _, endFn := d.tracker.Start(
		ctx,
		"embed",
		"embedding",
		"text_length", len(text),
	)
	defer endFn()

	// Execute the embedding operation
	vector, err := d.service.Embed(ctx, text)
	if err != nil {
		// Report error with additional context
		reportErr(fmt.Errorf("embedding failed: %w", err))
		return nil, err
	}

	// Report successful operation metrics
	reportChange := func(id string, data any) {
		// Using "operation" as the ID since there's no natural entity ID
		d.tracker.Start(
			ctx,
			"embed_result",
			"embedding",
			"vector_length", len(vector),
			"text_length", len(text),
		)
	}
	reportChange("operation", map[string]interface{}{
		"vector_length": len(vector),
		"text_length":   len(text),
	})

	return vector, nil
}

func WithActivityTracker(service Service, tracker activitytracker.ActivityTracker) Service {
	return &activityTrackerDecorator{
		service: service,
		tracker: tracker,
	}
}

var _ Service = (*activityTrackerDecorator)(nil)

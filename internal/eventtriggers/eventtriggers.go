package eventtriggers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/contenox/runtime/eventsourceservice"
	"github.com/contenox/runtime/eventstore"
	"github.com/contenox/runtime/functionservice"
)

type Manager struct {
	functionService functionservice.Service
	eventService    eventsourceservice.Service
}

func New(
	functionService functionservice.Service,
	eventService eventsourceservice.Service,
) *Manager {
	return &Manager{
		functionService: functionService,
		eventService:    eventService,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	ch := make(chan []byte, 1024)
	sub, err := m.eventService.SubscribeToEvents(ctx, "*", ch)
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()

	// Start consuming events from the channel
	for {
		select {
		case <-ctx.Done():
			return ctx.Err() // Respect context cancellation
		case data := <-ch:
			var event eventstore.Event
			if err := json.Unmarshal(data, &event); err != nil {
				// Log error and continue — don't block the entire stream
				fmt.Printf("Failed to unmarshal event: %v\n", err)
				continue
			}

			// Look up triggers for this event type
			triggers, err := m.functionService.ListEventTriggersByEventType(ctx, event.EventType)
			if err != nil {
				fmt.Printf("Failed to fetch triggers for event type %s: %v\n", event.EventType, err)
				continue
			}

			// For each trigger, place a job
			for _, trigger := range triggers {
				if err := m.PlaceJob(ctx, &event); err != nil {
					fmt.Printf("Failed to place job for trigger %s on event %s: %v\n", trigger.Name, event.EventType, err)
					// Decide: continue or break? Since one failing job shouldn't block others, we continue.
					continue
				}
			}
		}
	}
}

func (m *Manager) PlaceJob(ctx context.Context, event *eventstore.Event) error {
	return nil
}

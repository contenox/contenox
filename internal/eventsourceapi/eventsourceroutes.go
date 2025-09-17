package eventsourceapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/contenox/runtime/eventsourceservice"
	"github.com/contenox/runtime/eventstore"
	serverops "github.com/contenox/runtime/internal/apiframework"
)

// AddEventSourceRoutes registers HTTP routes for event source operations.
func AddEventSourceRoutes(mux *http.ServeMux, service eventsourceservice.Service) {
	e := &eventSourceManager{service: service}

	// Write operations
	mux.HandleFunc("POST /events", e.appendEvent)

	// Read operations
	mux.HandleFunc("GET /events/aggregate", e.getEventsByAggregate)
	mux.HandleFunc("GET /events/type", e.getEventsByType)
	mux.HandleFunc("GET /events/source", e.getEventsBySource) // NEW
	mux.HandleFunc("GET /events/types", e.getEventTypesInRange)

	// Delete operations
	mux.HandleFunc("DELETE /events/type", e.deleteEventsByTypeInRange)
}

type eventSourceManager struct {
	service eventsourceservice.Service
}

// Appends a new event to the event store.
//
// The event ID and CreatedAt will be auto-generated if not provided.
// Events must be within ±10 minutes of current server time.
func (e *eventSourceManager) appendEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	event, err := serverops.Decode[eventstore.Event](r) // @request eventstore.Event
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	if err := e.service.AppendEvent(ctx, &event); err != nil {
		_ = serverops.Error(w, r, err, serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, event) // @response eventstore.Event
}

// Retrieves events for a specific aggregate within a time range.
//
// Useful for rebuilding aggregate state or auditing changes.
func (e *eventSourceManager) getEventsByAggregate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	eventType := serverops.GetQueryParam(r, "event_type", "", "The type of event to filter by.")
	aggregateType := serverops.GetQueryParam(r, "aggregate_type", "", "The aggregate type (e.g., 'user', 'order').")
	aggregateID := serverops.GetQueryParam(r, "aggregate_id", "", "The unique ID of the aggregate.")
	fromStr := serverops.GetQueryParam(r, "from", time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "Start time in RFC3339 format.")
	toStr := serverops.GetQueryParam(r, "to", time.Now().UTC().Format(time.RFC3339), "End time in RFC3339 format.")
	limitStr := serverops.GetQueryParam(r, "limit", "100", "Maximum number of events to return.")

	if eventType == "" {
		_ = serverops.Error(w, r, fmt.Errorf("event_type is required %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}
	if aggregateType == "" {
		_ = serverops.Error(w, r, fmt.Errorf("aggregate_type is required %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}
	if aggregateID == "" {
		_ = serverops.Error(w, r, fmt.Errorf("aggregate_id is required %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'from' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'to' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		_ = serverops.Error(w, r, fmt.Errorf("invalid limit, must be positive integer %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	events, err := e.service.GetEventsByAggregate(ctx, eventType, from, to, aggregateType, aggregateID, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, events) // @response []eventstore.Event
}

// Retrieves events of a specific type within a time range.
//
// Useful for cross-aggregate analysis or system-wide event monitoring.
func (e *eventSourceManager) getEventsByType(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	eventType := serverops.GetQueryParam(r, "event_type", "", "The type of event to filter by.")
	fromStr := serverops.GetQueryParam(r, "from", time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "Start time in RFC3339 format.")
	toStr := serverops.GetQueryParam(r, "to", time.Now().UTC().Format(time.RFC3339), "End time in RFC3339 format.")
	limitStr := serverops.GetQueryParam(r, "limit", "100", "Maximum number of events to return.")

	if eventType == "" {
		_ = serverops.Error(w, r, fmt.Errorf("event_type is required %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'from' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'to' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		_ = serverops.Error(w, r, fmt.Errorf("invalid limit, must be positive integer %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	events, err := e.service.GetEventsByType(ctx, eventType, from, to, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, events) // @response []eventstore.Event
}

// Retrieves events from a specific source within a time range.
//
// Useful for auditing or monitoring events from specific subsystems.
func (e *eventSourceManager) getEventsBySource(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	eventType := serverops.GetQueryParam(r, "event_type", "", "The type of event to filter by.")
	eventSource := serverops.GetQueryParam(r, "event_source", "", "The source system that generated the event.")
	fromStr := serverops.GetQueryParam(r, "from", time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "Start time in RFC3339 format.")
	toStr := serverops.GetQueryParam(r, "to", time.Now().UTC().Format(time.RFC3339), "End time in RFC3339 format.")
	limitStr := serverops.GetQueryParam(r, "limit", "100", "Maximum number of events to return.")

	if eventType == "" {
		_ = serverops.Error(w, r, fmt.Errorf("event_type is required %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}
	if eventSource == "" {
		_ = serverops.Error(w, r, fmt.Errorf("event_source is required %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'from' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'to' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		_ = serverops.Error(w, r, fmt.Errorf("invalid limit, must be positive integer %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	events, err := e.service.GetEventsBySource(ctx, eventType, from, to, eventSource, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, events) // @response []eventstore.Event
}

// Lists distinct event types that occurred within a time range.
//
// Useful for discovery or building event type filters in UIs.
func (e *eventSourceManager) getEventTypesInRange(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fromStr := serverops.GetQueryParam(r, "from", time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "Start time in RFC3339 format.")
	toStr := serverops.GetQueryParam(r, "to", time.Now().UTC().Format(time.RFC3339), "End time in RFC3339 format.")
	limitStr := serverops.GetQueryParam(r, "limit", "100", "Maximum number of event types to return.")

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'from' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'to' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		_ = serverops.Error(w, r, fmt.Errorf("invalid limit, must be positive integer %w", serverops.ErrUnprocessableEntity), serverops.ListOperation)
		return
	}

	eventTypes, err := e.service.GetEventTypesInRange(ctx, from, to, limit)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, eventTypes) // @response []string
}

// Deletes all events of a specific type within a time range.
//
// USE WITH CAUTION — this is a destructive operation.
// Typically used for GDPR compliance or cleaning up test data.
func (e *eventSourceManager) deleteEventsByTypeInRange(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	eventType := serverops.GetQueryParam(r, "event_type", "", "The type of event to delete.")
	fromStr := serverops.GetQueryParam(r, "from", "", "Start time in RFC3339 format.")
	toStr := serverops.GetQueryParam(r, "to", "", "End time in RFC3339 format.")

	if eventType == "" {
		_ = serverops.Error(w, r, fmt.Errorf("event_type is required %w", serverops.ErrUnprocessableEntity), serverops.DeleteOperation)
		return
	}
	if fromStr == "" {
		_ = serverops.Error(w, r, fmt.Errorf("'from' is required %w", serverops.ErrUnprocessableEntity), serverops.DeleteOperation)
		return
	}
	if toStr == "" {
		_ = serverops.Error(w, r, fmt.Errorf("'to' is required %w", serverops.ErrUnprocessableEntity), serverops.DeleteOperation)
		return
	}

	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'from' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.DeleteOperation)
		return
	}

	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid 'to' time format, expected RFC3339 %w", serverops.ErrUnprocessableEntity), serverops.DeleteOperation)
		return
	}

	if err := e.service.DeleteEventsByTypeInRange(ctx, eventType, from, to); err != nil {
		_ = serverops.Error(w, r, err, serverops.DeleteOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "events deleted") // @response string
}

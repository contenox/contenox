package eventbridgeapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/PaesslerAG/jsonpath"
	"github.com/contenox/runtime/eventbridgeservice"
	"github.com/contenox/runtime/eventstore"

	serverops "github.com/contenox/runtime/apiframework"
)

// AddEventBridgeRoutes registers HTTP routes for event bridge operations
func AddEventBridgeRoutes(mux *http.ServeMux, bridgeService eventbridgeservice.Service) {
	h := &eventBridgeHandler{service: bridgeService}

	// Main ingestion endpoint - applies mapping configuration to incoming events
	mux.HandleFunc("POST /ingest", h.ingestEvent)

	// Sync endpoint to refresh mapping cache
	mux.HandleFunc("POST /sync", h.syncMappings)
}

type eventBridgeHandler struct {
	service eventbridgeservice.Service
}

// IngestEvent processes incoming events by applying mapping configuration
//
// This endpoint transforms raw payloads into structured events using the mapping
// configuration specified by the path query parameter. The mapping defines how to extract
// event properties like aggregate_id, event_type, etc. from the incoming data.
//
// The path query parameter corresponds to a pre-configured mapping that specifies:
// - How to extract the event type from the payload
// - How to extract the aggregate ID and type
// - How to handle metadata mapping
// - Field extraction rules for event properties
func (h *eventBridgeHandler) ingestEvent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	path := serverops.GetQueryParam(r, "path", "", "The mapping configuration path to apply")
	if path == "" {
		_ = serverops.Error(w, r, fmt.Errorf("path query parameter is required: %w", serverops.ErrBadPathValue), serverops.CreateOperation)
		return
	}

	// Get the mapping configuration
	mapping, err := h.service.GetMapping(ctx, path)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("failed to get mapping for path %s: %w", path, err), serverops.CreateOperation)
		return
	}

	// Decode the incoming payload
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("invalid JSON payload: %w", err), serverops.CreateOperation)
		return
	}

	// Apply mapping to transform payload into structured event
	event, err := h.applyMapping(mapping, payload, r.Header)
	if err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("failed to apply mapping: %w", err), serverops.CreateOperation)
		return
	}

	// Ingest the transformed event
	if err := h.service.Ingest(ctx, *event); err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("failed to ingest event: %w", err), serverops.CreateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusCreated, event) // @response eventstore.Event
}

// applyMapping transforms a raw payload into a structured event using the mapping configuration
func (h *eventBridgeHandler) applyMapping(mapping *eventstore.MappingConfig, payload map[string]interface{}, headers http.Header) (*eventstore.Event, error) {
	event := &eventstore.Event{
		ID:        generateEventID(),
		CreatedAt: time.Now().UTC(),
		Version:   mapping.Version,
	}

	// Extract event type - either from field or use fixed value
	event.EventType = mapping.EventType
	if mapping.EventTypeField != "" {
		if eventType, ok := getFieldValue(payload, mapping.EventTypeField); ok {
			event.EventType = fmt.Sprintf("%v", eventType)
		}
	}

	// Extract event source - either from field or use fixed value
	event.EventSource = mapping.EventSource
	if mapping.EventSourceField != "" {
		if eventSource, ok := getFieldValue(payload, mapping.EventSourceField); ok {
			event.EventSource = fmt.Sprintf("%v", eventSource)
		}
	}

	// Extract aggregate ID - required field
	if mapping.AggregateIDField != "" {
		if aggregateID, ok := getFieldValue(payload, mapping.AggregateIDField); ok {
			event.AggregateID = fmt.Sprintf("%v", aggregateID)
		} else {
			return nil, fmt.Errorf("aggregate ID field '%s' not found in payload", mapping.AggregateIDField)
		}
	} else {
		return nil, fmt.Errorf("aggregate ID field mapping is required")
	}

	// Extract or use fixed aggregate type
	if mapping.AggregateTypeField != "" {
		if aggregateType, ok := getFieldValue(payload, mapping.AggregateTypeField); ok {
			event.AggregateType = fmt.Sprintf("%v", aggregateType)
		} else {
			return nil, fmt.Errorf("aggregate type field '%s' not found in payload", mapping.AggregateTypeField)
		}
	} else if mapping.AggregateType != "" {
		event.AggregateType = mapping.AggregateType
	} else {
		return nil, fmt.Errorf("aggregate type field mapping is required")
	}

	// Set the payload data
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event data: %w", err)
	}
	event.Data = data

	// Extract metadata if mapping specified
	if len(mapping.MetadataMapping) > 0 {
		metadata := make(map[string]interface{})
		for metaKey, fieldPath := range mapping.MetadataMapping {
			if value, ok := getFieldValue(payload, fieldPath); ok {
				metadata[metaKey] = value
			}
		}
		if len(metadata) > 0 {
			metaData, err := json.Marshal(metadata)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal metadata: %w", err)
			}
			event.Metadata = metaData
		}
	}

	return event, nil
}

// getFieldValue extracts a value from a nested map using dot notation
func getFieldValue(payload map[string]interface{}, fieldPath string) (interface{}, bool) {
	// JSONPath expects a root object, so wrap in "$."
	expr := "$." + fieldPath
	result, err := jsonpath.Get(expr, payload)
	if err != nil || result == nil {
		return nil, false
	}

	// jsonpath.Get may return []interface{} if multiple matches
	if slice, ok := result.([]interface{}); ok && len(slice) > 0 {
		return slice[0], true
	}
	return result, true
}

// generateEventID creates a unique event identifier
func generateEventID() string {
	return fmt.Sprintf("evt_%d", time.Now().UnixNano())
}

// SyncMappings refreshes the mapping cache from the underlying storage
func (h *eventBridgeHandler) syncMappings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if err := h.service.Sync(ctx); err != nil {
		_ = serverops.Error(w, r, fmt.Errorf("failed to sync mappings: %w", err), serverops.UpdateOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, "mappings synchronized") // @response string
}

// ListMappings returns all configured event mappings
func (h *eventBridgeHandler) listMappings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	mappings, err := h.service.ListMappings(ctx)
	if err != nil {
		_ = serverops.Error(w, r, err, serverops.ListOperation)
		return
	}

	_ = serverops.Encode(w, r, http.StatusOK, mappings) // @response []eventstore.MappingConfig
}

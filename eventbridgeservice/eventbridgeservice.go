package eventbridgeservice

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/contenox/runtime/eventmappingservice"
	"github.com/contenox/runtime/eventsourceservice"
	"github.com/contenox/runtime/eventstore"
	"github.com/contenox/runtime/internal/apiframework"
)

var (
	ErrMappingNotFound = fmt.Errorf("mapping not found: %w", apiframework.ErrNotFound)
)

type Service interface {
	Bus
	ListMappings(ctx context.Context) ([]eventstore.MappingConfig, error)
	GetMapping(ctx context.Context, path string) (*eventstore.MappingConfig, error)
	Renderer
}

type Renderer interface {
	Sync(ctx context.Context) error
}

type Bus interface {
	Ingest(ctx context.Context, event ...eventstore.Event) error
}

type eventBridge struct {
	eventMapping    eventmappingservice.Service
	eventsource     eventsourceservice.Service
	mappingCache    atomic.Pointer[map[string]*eventstore.MappingConfig]
	lastSync        atomic.Int64 // Unix nanoseconds
	syncInProgress  atomic.Bool
	callInitialSync atomic.Bool
	syncInterval    time.Duration
}

// New creates a new eventBridge instance with initial synchronization
func New(
	eventMapping eventmappingservice.Service,
	eventsource eventsourceservice.Service,
	syncInterval time.Duration,
) Service {
	bridge := &eventBridge{
		eventMapping:    eventMapping,
		eventsource:     eventsource,
		syncInterval:    syncInterval,
		callInitialSync: atomic.Bool{},
		syncInProgress:  atomic.Bool{},
	}

	// Initialize with empty map
	emptyCache := make(map[string]*eventstore.MappingConfig)
	bridge.mappingCache.Store(&emptyCache)
	bridge.callInitialSync.Store(true)

	return bridge
}

// GetMapping implements Service
func (e *eventBridge) GetMapping(ctx context.Context, path string) (*eventstore.MappingConfig, error) {
	if path == "" {
		return nil, fmt.Errorf("path is required")
	}

	// Ensure cache is up to date
	if _, err := e.syncMappings(ctx, false); err != nil {
		return nil, err
	}

	cache := *e.mappingCache.Load()
	if mapping, exists := cache[path]; exists {
		return mapping, nil
	}

	return nil, ErrMappingNotFound
}

// ListMappings implements Service
func (e *eventBridge) ListMappings(ctx context.Context) ([]eventstore.MappingConfig, error) {
	// Ensure cache is up to date
	cache, err := e.syncMappings(ctx, false)
	if err != nil {
		return nil, err
	}

	mappings := make([]eventstore.MappingConfig, 0, len(cache))
	for _, mapping := range cache {
		if mapping != nil {
			mappings = append(mappings, *mapping)
		}
	}

	return mappings, nil
}

// syncMappings synchronizes the mapping cache with the event mapping service
// It only performs I/O operations when necessary and uses atomic flags to prevent redundant operations
func (e *eventBridge) syncMappings(ctx context.Context, force bool) (map[string]*eventstore.MappingConfig, error) {
	// Check if we need to sync
	lastSync := time.Unix(0, e.lastSync.Load())
	needSync := force || e.callInitialSync.Load() || time.Since(lastSync) > e.syncInterval

	if needSync && e.syncInProgress.CompareAndSwap(false, true) {
		defer e.syncInProgress.Store(false)

		mappings, err := e.eventMapping.ListMappings(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list mappings: %w", err)
		}

		// Build new cache
		newCache := make(map[string]*eventstore.MappingConfig)
		for _, mapping := range mappings {
			if mapping != nil && mapping.Path != "" {
				newCache[mapping.Path] = mapping
			}
		}

		// Atomically update cache
		e.mappingCache.Store(&newCache)
		e.lastSync.Store(time.Now().UnixNano())
		e.callInitialSync.Store(false)

		return newCache, nil
	}

	// If no sync needed or sync in progress, return current cache
	return *e.mappingCache.Load(), nil
}

// Sync implements Renderer interface
func (e *eventBridge) Sync(ctx context.Context) error {
	_, err := e.syncMappings(ctx, true)
	return err
}

// Ingest implements Bus interface
func (e *eventBridge) Ingest(ctx context.Context, events ...eventstore.Event) error {
	for i := range events {
		// Pass by pointer as AppendEvent expects *Event
		if err := e.eventsource.AppendEvent(ctx, &events[i]); err != nil {
			return fmt.Errorf("failed to append event: %w", err)
		}
	}
	return nil
}

package eventdispatch

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/contenox/runtime/eventstore"
	"github.com/contenox/runtime/functionservice"
	"github.com/contenox/runtime/functionstore"
	"github.com/contenox/runtime/libtracker"
)

// Trigger defines the interface for handling events and executing associated functions.
type Trigger interface {
	// HandleEvent processes one or more events and triggers any associated functions.
	HandleEvent(ctx context.Context, events ...*eventstore.Event)
}

// FunctionsHandler manages the caching and execution of functions triggered by events.
// It maintains caches of functions and event triggers that are periodically synchronized.
type FunctionsHandler struct {
	functionCache     atomic.Pointer[map[string]*functionstore.Function]
	triggerCache      atomic.Pointer[map[string][]*functionstore.EventTrigger]
	lastFunctionsSync atomic.Int64 // Unix nanoseconds
	lastTriggersSync  atomic.Int64 // Unix nanoseconds
	callInitialSync   atomic.Bool
	syncInterval      time.Duration
	functions         functionservice.Service
	onError           func(error)
	tracker           libtracker.ActivityTracker
	triggersInUpdate  atomic.Bool
	functionsInUpdate atomic.Bool
}

// New creates a new FunctionsHandler instance with initial synchronization.
// It returns a Trigger implementation that can handle events.
//
// Parameters:
//   - ctx: Context for the initialization operations
//   - functions: Service for retrieving functions and triggers
//   - onError: Error handler callback
//   - syncInterval: How often to synchronize with the function service
//   - tracker: Activity tracker for monitoring (optional)
func New(
	ctx context.Context,
	functions functionservice.Service,
	onError func(error),
	syncInterval time.Duration,
	tracker libtracker.ActivityTracker) (Trigger, error) {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}

	repo := &FunctionsHandler{
		functions:         functions,
		onError:           onError,
		syncInterval:      syncInterval,
		tracker:           tracker,
		callInitialSync:   atomic.Bool{},
		triggersInUpdate:  atomic.Bool{},
		functionsInUpdate: atomic.Bool{},
	}

	// Initialize with empty maps
	fc := make(map[string]*functionstore.Function)
	repo.functionCache.Store(&fc)
	tc := make(map[string][]*functionstore.EventTrigger)
	repo.triggerCache.Store(&tc)

	// Perform initial sync
	repo.callInitialSync.Store(true)
	if _, err := repo.syncFunctions(ctx); err != nil {
		return nil, err
	}
	if _, err := repo.syncTriggers(ctx); err != nil {
		return nil, err
	}
	repo.callInitialSync.Store(false)

	return repo, nil
}

// syncFunctions synchronizes the function cache with the function service.
// It only performs I/O operations when necessary and uses atomic flags to prevent redundant operations.
func (r *FunctionsHandler) syncFunctions(ctx context.Context) (map[string]*functionstore.Function, error) {
	// Check if we need to sync
	lastSync := time.Unix(0, r.lastFunctionsSync.Load())
	needSync := r.callInitialSync.Load() || time.Since(lastSync) > r.syncInterval

	if needSync && r.functionsInUpdate.CompareAndSwap(false, true) {
		defer r.functionsInUpdate.Store(false)

		functions, err := r.functions.ListAllFunctions(ctx)
		if err != nil {
			return nil, err
		}

		functionCache := make(map[string]*functionstore.Function)
		for _, f := range functions {
			functionCache[f.Name] = f
		}

		r.functionCache.Store(&functionCache)
		r.lastFunctionsSync.Store(time.Now().UnixNano())
		r.callInitialSync.Store(false)

		return functionCache, nil
	}

	// If no sync needed or sync in progress, return current cache
	return *r.functionCache.Load(), nil
}

// syncTriggers synchronizes the trigger cache with the function service.
// It only performs I/O operations when necessary and uses atomic flags to prevent redundant operations.
func (r *FunctionsHandler) syncTriggers(ctx context.Context) (map[string][]*functionstore.EventTrigger, error) {
	// Check if a sync is needed and try to acquire the non-blocking lock
	lastSync := time.Unix(0, r.lastTriggersSync.Load())
	needSync := r.callInitialSync.Load() || time.Since(lastSync) > r.syncInterval

	if needSync && r.triggersInUpdate.CompareAndSwap(false, true) {
		defer r.triggersInUpdate.Store(false)

		triggers, err := r.functions.ListAllEventTriggers(ctx)
		if err != nil {
			return nil, err
		}

		// Use a local map to build the new cache with deduplication
		triggerCache := make(map[string][]*functionstore.EventTrigger)
		seenTriggers := make(map[string]map[string]bool) // eventType -> functionName -> exists

		for _, t := range triggers {
			if t == nil {
				continue
			}

			eventType := t.ListenFor.Type
			functionName := t.Function

			// Initialize the inner map if needed
			if _, exists := seenTriggers[eventType]; !exists {
				seenTriggers[eventType] = make(map[string]bool)
			}

			// Deduplicate by function name within the same event type
			if !seenTriggers[eventType][functionName] {
				triggerCache[eventType] = append(triggerCache[eventType], t)
				seenTriggers[eventType][functionName] = true
			}
		}

		r.triggerCache.Store(&triggerCache)
		r.lastTriggersSync.Store(time.Now().UnixNano())
		r.callInitialSync.Store(false)

		return triggerCache, nil
	}

	// If a sync isn't needed or one is in progress, return the existing cache
	return *r.triggerCache.Load(), nil
}

// FunctionWithTrigger represents a function and its associated trigger.
type FunctionWithTrigger struct {
	Function *functionstore.Function
	Trigger  *functionstore.EventTrigger
}

// GetFunctions retrieves all functions associated with the given event types.
// It returns a mapping from event type to a list of function-trigger pairs.
func (r *FunctionsHandler) GetFunctions(ctx context.Context, eventTypes ...string) (map[string][]*FunctionWithTrigger, error) {
	functionsCache, err := r.syncFunctions(ctx)
	if err != nil {
		return nil, err
	}

	triggerCache, err := r.syncTriggers(ctx)
	if err != nil {
		return nil, err
	}

	result := make(map[string][]*FunctionWithTrigger)

	for _, eventType := range eventTypes {
		triggers, ok := triggerCache[eventType]
		if !ok {
			continue
		}

		for _, trigger := range triggers {
			function, ok := functionsCache[trigger.Function]
			if !ok {
				continue
			}

			result[eventType] = append(result[eventType], &FunctionWithTrigger{
				Function: function,
				Trigger:  trigger,
			})
		}
	}

	return result, nil
}

// HandleEvent processes incoming events and triggers any associated functions.
func (r *FunctionsHandler) HandleEvent(ctx context.Context, events ...*eventstore.Event) {
	eventTypes := make([]string, 0, len(events))
	for _, event := range events {
		eventTypes = append(eventTypes, event.EventType)
	}

	functionsWithTrigger, err := r.GetFunctions(ctx, eventTypes...)
	if err != nil {
		r.onError(err)
		return
	}

	// Execute the associated functions
	for eventType, functionList := range functionsWithTrigger {
		for _, functionWithTrigger := range functionList {
			// TODO: Implement goja execution here
			_ = eventType
			_ = functionWithTrigger
		}
	}
}

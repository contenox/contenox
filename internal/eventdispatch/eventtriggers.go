package eventdispatch

import (
	"context"
	"sync"
	"time"

	"github.com/contenox/runtime/eventstore"
	"github.com/contenox/runtime/functionservice"
	"github.com/contenox/runtime/functionstore"
	"github.com/contenox/runtime/libtracker"
)

type Trigger interface {
	HandleEvent(ctx context.Context, events ...*eventstore.Event)
}

type FunctionsHandler struct {
	functionCache map[string]*functionstore.Function
	triggerCache  map[string][]*functionstore.EventTrigger
	lastSync      time.Time
	initialSync   bool
	syncInterval  time.Duration
	lock          sync.RWMutex
	functions     functionservice.Service
	onError       func(error)
	tracker       libtracker.ActivityTracker
}

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
		functionCache: make(map[string]*functionstore.Function),
		triggerCache:  make(map[string][]*functionstore.EventTrigger),
		lock:          sync.RWMutex{},
		functions:     functions,
		onError:       onError,
		syncInterval:  syncInterval,
		tracker:       tracker,
		initialSync:   true,
	}
	if _, err := repo.syncFunctions(ctx); err != nil {
		return nil, err
	}
	if _, err := repo.syncTriggers(ctx); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *FunctionsHandler) syncFunctions(ctx context.Context) (map[string]*functionstore.Function, error) {
	functionCache := make(map[string]*functionstore.Function)
	if r.initialSync || r.lastSync.Before(time.Now().Add(-r.syncInterval)) {
		functions, err := r.functions.ListAllFunctions(ctx)
		if err != nil {
			return nil, err
		}
		r.initialSync = false
		for _, f := range functions {
			functionCache[f.Name] = f
		}
		r.lock.RLock()
		r.functionCache = functionCache
		r.lock.RUnlock()
	}
	if len(functionCache) == 0 {
		r.lock.RLock()
		defer r.lock.RUnlock()
		return r.functionCache, nil
	}

	return functionCache, nil
}

func (r *FunctionsHandler) syncTriggers(ctx context.Context) (map[string][]*functionstore.EventTrigger, error) {
	triggerCache := make(map[string][]*functionstore.EventTrigger)
	if r.initialSync || r.lastSync.Before(time.Now().Add(-r.syncInterval)) {
		triggers, err := r.functions.ListAllEventTriggers(ctx)
		if err != nil {
			return nil, err
		}
		r.initialSync = false
		for _, t := range triggers {
			if t == nil {
				continue
			}
			tt := t.ListenFor.Type
			triggerCache[tt] = append(triggerCache[tt], t)
		}
		r.lock.RLock()
		r.triggerCache = triggerCache
		r.lock.RUnlock()
	}
	if len(triggerCache) == 0 {
		r.lock.RLock()
		defer r.lock.RUnlock()
		return r.triggerCache, nil
	}

	return triggerCache, nil
}

func (r *FunctionsHandler) GetFunctions(ctx context.Context, eventTypes ...string) ([]*functionstore.Function, error) {
	functionsCache, err := r.syncFunctions(ctx)
	if err != nil {
		return nil, err
	}
	functions := make([]*functionstore.Function, 0, len(functionsCache))
	triggerCache, err := r.syncTriggers(ctx)
	if err != nil {
		return nil, err
	}
	for _, eventType := range eventTypes {
		triggerfromCache, ok := triggerCache[eventType]
		if !ok {
			continue
		}
		for _, et := range triggerfromCache {
			functionsFromCache, ok := functionsCache[et.Function]
			if !ok {
				continue
			}
			functions = append(functions, functionsFromCache)
		}
	}

	return functions, nil
}

func (r *FunctionsHandler) HandleEvent(ctx context.Context, events ...*eventstore.Event) {
	eventTypes := make([]string, 0, len(events))
	for _, event := range events {
		eventTypes = append(eventTypes, event.EventType)
	}
	functions, err := r.GetFunctions(ctx, eventTypes...)
	if err != nil {
		r.onError(err)
	}
	for _, function := range functions {
		_ = function
		// TODO trigger goja
	}
}

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
	lock          sync.Mutex
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
		r.lock.Lock()
		r.functionCache = functionCache
		r.lock.Unlock()
	}
	if len(functionCache) == 0 {
		r.lock.Lock()
		defer r.lock.Unlock()
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
		r.lock.Lock()
		r.triggerCache = triggerCache
		r.lock.Unlock()
	}
	if len(triggerCache) == 0 {
		r.lock.Lock()
		defer r.lock.Unlock()
		return r.triggerCache, nil
	}

	return triggerCache, nil
}

type FunctionWithTrigger struct {
	Function *functionstore.Function
	Trigger  *functionstore.EventTrigger
}

func (r *FunctionsHandler) GetFunctions(ctx context.Context, eventTypes ...string) (map[string][]*FunctionWithTrigger, error) {
	functionsCache, err := r.syncFunctions(ctx)
	if err != nil {
		return nil, err
	}
	functions := make(map[string][]*FunctionWithTrigger)
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
			functions[eventType] = append(functions[eventType], &FunctionWithTrigger{
				Function: functionsFromCache,
				Trigger:  et,
			})
		}
	}

	return functions, nil
}

func (r *FunctionsHandler) HandleEvent(ctx context.Context, events ...*eventstore.Event) {
	eventTypes := make([]string, 0, len(events))
	for _, event := range events {
		eventTypes = append(eventTypes, event.EventType)
	}
	functionsWithTrigger, err := r.GetFunctions(ctx, eventTypes...)
	if err != nil {
		r.onError(err)
	}
	for _, function := range functionsWithTrigger {
		_ = function
		// TODO trigger goja
	}
}

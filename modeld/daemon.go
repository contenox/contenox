package modeld

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Daemon is the process-wide owner of model-repository state. It is the
// in-process singleton that a transport (see the transport subpackage) serves:
// it holds the configured backends and their resolved catalog providers behind
// a mutex so concurrent requests observe one authoritative, consistent state.
//
// modeld is being split out of the runtime package to expose the modelrepo API
// over a wire transport and to own backend lifecycle independently; Daemon is
// the seam for that split.
type Daemon struct {
	mu       sync.RWMutex
	factory  CatalogFactory
	backends map[string]*backendEntry
}

type backendEntry struct {
	spec    BackendSpec
	catalog CatalogProvider
}

// DaemonOption configures a Daemon at construction.
type DaemonOption func(*Daemon)

// WithCatalogFactory overrides the CatalogFactory used to resolve backends.
// Defaults to DefaultCatalogFactory().
func WithCatalogFactory(f CatalogFactory) DaemonOption {
	return func(d *Daemon) {
		if f != nil {
			d.factory = f
		}
	}
}

// NewDaemon constructs a Daemon with no backends registered.
func NewDaemon(opts ...DaemonOption) *Daemon {
	d := &Daemon{
		factory:  DefaultCatalogFactory(),
		backends: map[string]*backendEntry{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(d)
		}
	}
	return d
}

var (
	defaultDaemonOnce sync.Once
	defaultDaemon     *Daemon
)

// Default returns the process-wide singleton Daemon, constructing it on first
// use. Production callers use this; tests construct isolated instances with
// NewDaemon.
func Default() *Daemon {
	defaultDaemonOnce.Do(func() {
		defaultDaemon = NewDaemon()
	})
	return defaultDaemon
}

// RegisterBackend resolves spec into a catalog provider and stores it under id,
// replacing any existing backend with the same id.
func (d *Daemon) RegisterBackend(id string, spec BackendSpec, opts ...CatalogOption) error {
	if id == "" {
		return fmt.Errorf("modeld: backend id is required")
	}
	catalog, err := d.factory.NewCatalogProvider(spec, opts...)
	if err != nil {
		return fmt.Errorf("modeld: register backend %q: %w", id, err)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.backends[id] = &backendEntry{spec: spec, catalog: catalog}
	return nil
}

// RemoveBackend removes the backend registered under id, if any.
func (d *Daemon) RemoveBackend(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.backends, id)
}

// ListBackends returns the ids of all registered backends, sorted.
func (d *Daemon) ListBackends() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	ids := make([]string, 0, len(d.backends))
	for id := range d.backends {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (d *Daemon) catalogFor(id string) (CatalogProvider, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	entry, ok := d.backends[id]
	if !ok {
		return nil, fmt.Errorf("modeld: unknown backend %q", id)
	}
	return entry.catalog, nil
}

// ListModels observes the models exposed by the backend registered under id.
func (d *Daemon) ListModels(ctx context.Context, id string) ([]ObservedModel, error) {
	catalog, err := d.catalogFor(id)
	if err != nil {
		return nil, err
	}
	return catalog.ListModels(ctx)
}

// ProviderFor turns an observed model from backend id into an execution Provider.
func (d *Daemon) ProviderFor(id string, model ObservedModel) (Provider, error) {
	catalog, err := d.catalogFor(id)
	if err != nil {
		return nil, err
	}
	return catalog.ProviderFor(model), nil
}

// Stop drains backend resources by running the registered shutdown hooks (see
// RegisterShutdownHook). It is safe to call when no hooks are registered.
func (d *Daemon) Stop() error {
	return Shutdown()
}

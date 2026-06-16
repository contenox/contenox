package modeld_test

import (
	"context"
	"errors"
	"testing"

	"github.com/contenox/runtime/modeld"
)

// fakeCatalog is a CatalogProvider stub so daemon tests don't depend on any
// concrete backend (and never touch the global catalog registry).
type fakeCatalog struct {
	models []modeld.ObservedModel
}

func (f *fakeCatalog) Type() string { return "fake" }

func (f *fakeCatalog) ListModels(context.Context) ([]modeld.ObservedModel, error) {
	return f.models, nil
}

func (f *fakeCatalog) ProviderFor(model modeld.ObservedModel) modeld.Provider {
	return &modeld.MockProvider{ID: model.Name, Name: model.Name, CanChatFlag: true}
}

type fakeFactory struct {
	catalog modeld.CatalogProvider
	err     error
}

func (f fakeFactory) NewCatalogProvider(modeld.BackendSpec, ...modeld.CatalogOption) (modeld.CatalogProvider, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.catalog, nil
}

func TestUnit_Daemon_RegisterAndListModels(t *testing.T) {
	cat := &fakeCatalog{models: []modeld.ObservedModel{{Name: "m1"}, {Name: "m2"}}}
	d := modeld.NewDaemon(modeld.WithCatalogFactory(fakeFactory{catalog: cat}))

	if err := d.RegisterBackend("b1", modeld.BackendSpec{Type: "fake"}); err != nil {
		t.Fatalf("RegisterBackend: %v", err)
	}
	if got := d.ListBackends(); len(got) != 1 || got[0] != "b1" {
		t.Fatalf("ListBackends = %v, want [b1]", got)
	}

	models, err := d.ListModels(context.Background(), "b1")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("ListModels len = %d, want 2", len(models))
	}

	p, err := d.ProviderFor("b1", models[0])
	if err != nil {
		t.Fatalf("ProviderFor: %v", err)
	}
	if !p.CanChat() {
		t.Fatal("provider from fake catalog should report CanChat")
	}

	d.RemoveBackend("b1")
	if got := d.ListBackends(); len(got) != 0 {
		t.Fatalf("ListBackends after remove = %v, want empty", got)
	}
}

func TestUnit_Daemon_UnknownBackend(t *testing.T) {
	d := modeld.NewDaemon(modeld.WithCatalogFactory(fakeFactory{catalog: &fakeCatalog{}}))
	if _, err := d.ListModels(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestUnit_Daemon_RegisterFactoryError(t *testing.T) {
	d := modeld.NewDaemon(modeld.WithCatalogFactory(fakeFactory{err: errors.New("boom")}))
	if err := d.RegisterBackend("b1", modeld.BackendSpec{Type: "fake"}); err == nil {
		t.Fatal("expected error propagated from catalog factory")
	}
}

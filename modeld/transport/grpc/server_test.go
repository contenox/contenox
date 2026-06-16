package grpc_test

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/owner"
	"github.com/contenox/runtime/modeld/transport"
	modeldgrpc "github.com/contenox/runtime/modeld/transport/grpc"
	"github.com/contenox/runtime/modeld/transport/leader"
)

func TestUnit_GRPCServer_StartStop(t *testing.T) {
	d := modeld.NewDaemon()
	srv := modeldgrpc.NewServer(transport.FromDaemon(d))

	addr, err := srv.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if addr == nil {
		t.Fatal("expected a resolved listener address")
	}
	srv.Stop()
}

type fakeCatalog struct {
	models []modeld.ObservedModel
}

func (f fakeCatalog) Type() string { return "fake" }

func (f fakeCatalog) ListModels(context.Context) ([]modeld.ObservedModel, error) {
	return f.models, nil
}

func (f fakeCatalog) ProviderFor(model modeld.ObservedModel) modeld.Provider {
	return &modeld.MockProvider{ID: model.Name, Name: model.Name}
}

type fakeFactory struct {
	catalog modeld.CatalogProvider
}

func (f fakeFactory) NewCatalogProvider(modeld.BackendSpec, ...modeld.CatalogOption) (modeld.CatalogProvider, error) {
	return f.catalog, nil
}

func TestUnit_GRPCClient_RoundTrip(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	modified := time.Unix(123, 456)
	d := modeld.NewDaemon(modeld.WithCatalogFactory(fakeFactory{catalog: fakeCatalog{models: []modeld.ObservedModel{
		{
			Name:          "m1",
			ContextLength: 4096,
			ModifiedAt:    modified,
			Size:          42,
			Digest:        "sha256:abc",
			CapabilityConfig: modeld.CapabilityConfig{
				ContextLength:   4096,
				MaxOutputTokens: 512,
				CanChat:         true,
				CanEmbed:        true,
				CanStream:       true,
				CanPrompt:       true,
				CanThink:        true,
			},
			Meta: map[string]string{"display_name": "Model One"},
		},
	}}}))

	srv := modeldgrpc.NewServer(transport.FromDaemon(d))
	addr, err := srv.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	client, err := modeldgrpc.Dial(ctx, addr.String())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = client.Close() }()

	if err := client.RegisterBackend(ctx, "b1", modeld.BackendSpec{Type: "fake"}); err != nil {
		t.Fatalf("RegisterBackend: %v", err)
	}
	ids, err := client.ListBackends(ctx)
	if err != nil {
		t.Fatalf("ListBackends: %v", err)
	}
	if len(ids) != 1 || ids[0] != "b1" {
		t.Fatalf("ListBackends = %v, want [b1]", ids)
	}

	models, err := client.ListModels(ctx, "b1")
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("ListModels len = %d, want 1", len(models))
	}
	got := models[0]
	if got.Name != "m1" || got.ContextLength != 4096 || got.Size != 42 || got.Digest != "sha256:abc" {
		t.Fatalf("ListModels model = %#v", got)
	}
	if !got.ModifiedAt.Equal(modified) {
		t.Fatalf("ModifiedAt = %s, want %s", got.ModifiedAt, modified)
	}
	if got.MaxOutputTokens != 512 || !got.CanChat || !got.CanEmbed || !got.CanStream || !got.CanPrompt || !got.CanThink {
		t.Fatalf("capabilities not preserved: %#v", got.CapabilityConfig)
	}
	if got.Meta["display_name"] != "Model One" {
		t.Fatalf("Meta = %#v", got.Meta)
	}

	if err := client.RemoveBackend(ctx, "b1"); err != nil {
		t.Fatalf("RemoveBackend: %v", err)
	}
	ids, err = client.ListBackends(ctx)
	if err != nil {
		t.Fatalf("ListBackends after remove: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ListBackends after remove = %v, want empty", ids)
	}
}

func TestUnit_GRPCOwnerFenceRejectsStaleToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	d := modeld.NewDaemon()
	srv := modeldgrpc.NewServer(transport.FromDaemon(d), modeldgrpc.WithOwnerToken("owner-1"))
	addr, err := srv.Start("127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer srv.Stop()

	okClient, err := modeldgrpc.DialLeader(ctx, addr.String(), "owner-1")
	if err != nil {
		t.Fatalf("DialLeader ok: %v", err)
	}
	defer func() { _ = okClient.Close() }()
	if _, err := okClient.ListBackends(ctx); err != nil {
		t.Fatalf("ListBackends with current token: %v", err)
	}

	staleClient, err := modeldgrpc.DialLeader(ctx, addr.String(), "owner-0")
	if err != nil {
		t.Fatalf("DialLeader stale: %v", err)
	}
	defer func() { _ = staleClient.Close() }()
	if _, err := staleClient.ListBackends(ctx); err == nil {
		t.Fatal("expected stale token to be rejected")
	}
}

func TestUnit_LeaderDialUsesLeaseEndpointAndFence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := lis.Addr().String()

	leasePath := filepath.Join(t.TempDir(), "modeld.lease")
	l, err := liblease.Acquire(leasePath, time.Minute, liblease.WithMeta(map[string]string{
		owner.EndpointMetaKey: addr,
	}))
	if err != nil {
		_ = lis.Close()
		t.Fatalf("Acquire: %v", err)
	}
	defer func() { _ = l.Release() }()

	d := modeld.NewDaemon()
	srv := modeldgrpc.NewServer(transport.FromDaemon(d), modeldgrpc.WithOwnerToken(l.InstanceID()))
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	c, err := leader.Dial(ctx, leasePath)
	if err != nil {
		t.Fatalf("leader Dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	if c.Endpoint != addr {
		t.Fatalf("Endpoint = %q, want %q", c.Endpoint, addr)
	}
	if c.InstanceID != l.InstanceID() {
		t.Fatalf("InstanceID = %q, want %q", c.InstanceID, l.InstanceID())
	}
	if _, err := c.ListBackends(ctx); err != nil {
		t.Fatalf("ListBackends through leader connection: %v", err)
	}
}

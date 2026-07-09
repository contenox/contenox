package runtimestate

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/modeld/slot"
	"github.com/contenox/runtime/runtime/modelcapability"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
	"github.com/stretchr/testify/require"
)

// startFakeModeldNode serves a real slot.Service (the same shape production
// modeld serves) on a loopback port, rooted at modelsDir. Used to exercise
// processModeldBackend against real wire traffic instead of a mock.
func startFakeModeldNode(t *testing.T, instance, backend, modelsDir string) (addr string) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	svc := slot.New(
		transport.NewMemoryService(transport.WithOwnerFence(instance)),
		slot.WithOwner(instance),
		slot.WithBackend(backend),
		slot.WithModelsDir(modelsDir),
	)
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = transportgrpc.Serve(ctx, lis, svc, instance, backend) }()
	t.Cleanup(func() {
		cancel()
		modeldconn.CloseEndpoint("modeld-under-test")
	})
	return lis.Addr().String()
}

func writeFakeModel(t *testing.T, modelsDir, name string, content []byte) {
	t.Helper()
	dir := filepath.Join(modelsDir, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.gguf"), content, 0o644))
}

func TestUnit_ProcessModeldBackend_RemoteAddrDialsDirectlyAndListsModels(t *testing.T) {
	modelsDir := t.TempDir()
	writeFakeModel(t, modelsDir, "qwen", []byte("weights"))
	addr := startFakeModeldNode(t, "instance-remote", "llama", modelsDir)

	// applyCapabilityOverrides touches the DB once a model is actually found,
	// so a zero-value State (fine for the error-only tests below) would
	// panic on a nil dbInstance here.
	_, s, _ := newCapabilityOverrideTestState(t)
	backend := &runtimetypes.Backend{ID: "modeld-under-test", Name: "remote-node", Type: "modeld", BaseURL: addr}
	s.processModeldBackend(context.Background(), backend, nil)

	st := loadState(t, s, "modeld-under-test")
	require.Empty(t, st.Error)
	require.Len(t, st.PulledModels, 1)
	require.Equal(t, "qwen", st.PulledModels[0].Model)
	require.NotEmpty(t, st.PulledModels[0].Digest)
	require.Equal(t, int64(len("weights")), st.PulledModels[0].Size)
	// `contenox model list` reads these fields directly (not the constructed
	// llmresolver Provider) — they must reflect that this node is actually
	// servable, not silently disagree with what LocalProviderAdapter reports.
	require.True(t, st.PulledModels[0].CanChat, "modeld-listed model must display as chat-capable")
	require.True(t, st.PulledModels[0].CanPrompt)
	require.True(t, st.PulledModels[0].CanStream)
	require.True(t, st.PulledModels[0].CanEmbed)
}

func TestUnit_ProcessModeldBackend_UnreachableRemoteStoresError(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close()) // nothing listens here

	s := &State{}
	backend := &runtimetypes.Backend{ID: "modeld-dead", Name: "dead-node", Type: "modeld", BaseURL: addr}
	s.processModeldBackend(context.Background(), backend, nil)

	st := loadState(t, s, "modeld-dead")
	require.NotEmpty(t, st.Error)
}

// The local sentinel resolves through the SAME lease-based discovery the hot
// chat-serving path uses (modeldconn.LocalEndpointAddr), not a stored
// address — proving a "modeld on the same device" backend row reaches the
// identical daemon without needing its address written into the DB.
func TestUnit_ProcessModeldBackend_LocalSentinelResolvesViaLease(t *testing.T) {
	dir := t.TempDir()
	modeldconn.SetDataRoot(dir)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })

	modelsDir := t.TempDir()
	addr := startFakeModeldNode(t, "instance-local", "llama", modelsDir)

	lease, err := liblease.Acquire(
		filepath.Join(dir, "modeld.lease"), 30*time.Second,
		liblease.WithMeta(map[string]string{"backend": "llama", "endpoint": addr}),
		func(r *liblease.Record) { r.InstanceID = "instance-local" },
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lease.Release() })

	s := &State{}
	backend := &runtimetypes.Backend{ID: "modeld-under-test", Name: "local", Type: "modeld", BaseURL: modeldconn.LocalSentinel}
	s.processModeldBackend(context.Background(), backend, nil)

	st := loadState(t, s, "modeld-under-test")
	require.Empty(t, st.Error)
}

func TestUnit_ProcessModeldBackend_LocalSentinelNoLeaseStoresError(t *testing.T) {
	modeldconn.SetDataRoot(t.TempDir())
	t.Cleanup(func() { modeldconn.SetDataRoot("") })

	s := &State{}
	backend := &runtimetypes.Backend{ID: "modeld-under-test", Name: "local", Type: "modeld", BaseURL: modeldconn.LocalSentinel}
	s.processModeldBackend(context.Background(), backend, nil)

	st := loadState(t, s, "modeld-under-test")
	require.NotEmpty(t, st.Error)
}

// processBackend must actually dispatch Type "modeld" to processModeldBackend
// rather than falling through to the generic "Unsupported backend type"
// branch — that fallthrough was the behavior before this backend type
// existed, and is exactly the regression this test guards against.
func TestUnit_ProcessBackend_DispatchesModeldTypeInsteadOfUnsupported(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := lis.Addr().String()
	require.NoError(t, lis.Close())

	s := &State{}
	backend := &runtimetypes.Backend{ID: "modeld-dispatch", Name: "n", Type: "modeld", BaseURL: addr}
	s.processBackend(context.Background(), backend, nil)

	st := loadState(t, s, "modeld-dispatch")
	require.NotContains(t, st.Error, "Unsupported backend type")
}

func TestUnit_ProcessModeldBackend_AppliesCapabilityOverrides(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime-modeld.db")
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	store := runtimetypes.New(db.WithoutTransaction())
	_, err = modelcapability.New(store).SetThink(ctx, "modeld", "qwen", true)
	require.NoError(t, err)

	modelsDir := t.TempDir()
	writeFakeModel(t, modelsDir, "qwen", []byte("weights"))
	addr := startFakeModeldNode(t, "instance-remote", "llama", modelsDir)

	s := &State{dbInstance: db}
	backend := &runtimetypes.Backend{ID: "modeld-under-test", Name: "remote-node", Type: "modeld", BaseURL: addr}
	s.processModeldBackend(ctx, backend, nil)

	st := loadState(t, s, "modeld-under-test")
	require.Empty(t, st.Error)
	require.Len(t, st.PulledModels, 1)
	require.True(t, st.PulledModels[0].CanThink)
}

// Full reconcile through the DB-backed backend registry (RunBackendCycle),
// not a direct method call — proving a "modeld" backend row registered like
// any other backend type reconciles correctly end to end.
func TestUnit_RunBackendCycle_ReconcilesModeldBackend(t *testing.T) {
	ctx := context.Background()
	modelsDir := t.TempDir()
	writeFakeModel(t, modelsDir, "qwen", []byte("weights"))
	addr := startFakeModeldNode(t, "instance-remote", "llama", modelsDir)

	path := filepath.Join(t.TempDir(), "runtime-cycle-modeld.db")
	db, err := libdb.NewSQLiteDBManager(ctx, path, runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	defer db.Close()

	store := runtimetypes.New(db.WithoutTransaction())
	require.NoError(t, store.CreateBackend(ctx, &runtimetypes.Backend{
		ID: "modeld-node-1", Name: "remote-node", Type: "modeld", BaseURL: addr,
	}))

	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()
	state, err := New(ctx, db, bus, WithAutoDiscoverModels())
	require.NoError(t, err)
	require.NoError(t, state.RunBackendCycle(ctx))

	rt := state.Get(ctx)
	require.Contains(t, rt, "modeld-node-1")
	require.Empty(t, rt["modeld-node-1"].Error)
	require.Len(t, rt["modeld-node-1"].PulledModels, 1)
	require.Equal(t, "qwen", rt["modeld-node-1"].PulledModels[0].Model)

	// Verify that the adapter produces targeted providers for the remote modeld backend.
	adapter := LocalProviderAdapter(ctx, nil, rt)
	provs, err := adapter(ctx, "remote-node")
	require.NoError(t, err)
	require.NotEmpty(t, provs)
	// The provider should report the specific backend name in its backend IDs (for selection).
	var target modelrepo.Provider
	for _, pr := range provs {
		for _, bid := range pr.GetBackendIDs() {
			if bid == "remote-node" {
				target = pr
			}
		}
	}
	require.NotNil(t, target, "expected provider reporting 'remote-node' backend ID for targeted execution")

	// This is the check that would have caught the bug where a targeted
	// provider's Can* methods consulted this test process's own (nonexistent)
	// local modeld lease instead of the fact that the target node was already
	// proven live by the reconcile above: CanChat() etc. must not depend on
	// whether SOMETHING happens to be running locally on the test machine.
	require.True(t, target.CanChat(), "targeted provider must report CanChat from the reconciled target, not local machine state")
	require.True(t, target.CanPrompt())
	require.True(t, target.CanStream())
	require.True(t, target.CanEmbed())

	// Drive an actual chat call through GetChatConnection — the same gate a
	// real llmresolver-selected request would go through — proving the
	// targeted provider is not just advertised as usable but actually is.
	chatClient, err := target.GetChatConnection(ctx, "")
	require.NoError(t, err)
	resp, err := chatClient.Chat(ctx, []modelrepo.Message{{Role: "user", Content: "hello"}})
	require.NoError(t, err)
	require.NotEmpty(t, resp.Message.Content)
}

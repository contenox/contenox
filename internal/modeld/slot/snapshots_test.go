package slot

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/modeld/contextasm"
	"github.com/contenox/contenox/internal/transport"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
	"github.com/stretchr/testify/require"
)

type snapshotBackend struct {
	transport.Service
	opens    int
	restores int
	closes   int
	reject   bool
}

func (b *snapshotBackend) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	sess, err := b.Service.OpenSession(ctx, req)
	if err != nil {
		return nil, err
	}
	b.opens++
	return &snapshotSession{Session: sess, backend: b}, nil
}

type snapshotSession struct {
	transport.Session
	backend *snapshotBackend
}

func (s *snapshotSession) Snapshot(ctx context.Context) (transport.SessionSnapshot, error) {
	snap, err := s.Session.Snapshot(ctx)
	snap.State = []byte("native KV bytes")
	snap.ColdKVBlocks = []transport.ColdKVBlock{{Tokens: []int{1}, KV: []byte("cold KV bytes")}}
	return snap, err
}

func (s *snapshotSession) Restore(ctx context.Context, snap transport.SessionSnapshot) error {
	s.backend.restores++
	if s.backend.reject {
		_ = s.Session.Close()
		return errors.New("partially restored native state")
	}
	if string(snap.State) != "native KV bytes" || len(snap.ColdKVBlocks) != 1 || string(snap.ColdKVBlocks[0].KV) != "cold KV bytes" {
		return errors.New("snapshot lost native bytes")
	}
	return s.Session.Restore(ctx, snap)
}

func (s *snapshotSession) Close() error {
	s.backend.closes++
	return s.Session.Close()
}

func snapshotFixture(t *testing.T) (*Service, *snapshotBackend, transport.OpenSessionRequest, transport.PrefixInput) {
	t.Helper()
	t.Setenv("CONTENOX_WARM_SNAPSHOT_DISABLE", "")
	t.Setenv("CONTENOX_WARM_SNAPSHOT_DIR", "")
	model := filepath.Join(t.TempDir(), "model.gguf")
	require.NoError(t, os.WriteFile(model, []byte("model identity"), 0600))
	backend := &snapshotBackend{Service: transport.NewMemoryService()}
	svc := New(backend, WithDataRoot(t.TempDir()), WithBackend("llama"), WithIdleTTL(time.Minute))
	req := transport.OpenSessionRequest{ModelName: "a", Type: "llama", Path: model, Config: transport.Config{NumCtx: 1024}}
	prefix := transport.PrefixInput{Text: "stable instruction", Manifest: contextasm.ContextManifest{Backend: "llama", RuntimeDigest: "runtime", StableByteHash: contextasm.HashString("stable instruction")}}
	return svc, backend, req, prefix
}

func prefillSnapshot(t *testing.T, svc *Service, req transport.OpenSessionRequest, prefix transport.PrefixInput) {
	t.Helper()
	sess, err := svc.OpenSession(context.Background(), req)
	require.NoError(t, err)
	_, err = sess.EnsurePrefix(context.Background(), prefix)
	require.NoError(t, err)
	require.NoError(t, sess.Close())
}

func assertSnapshotReuse(t *testing.T, svc *Service, req transport.OpenSessionRequest, prefix transport.PrefixInput, warm bool) {
	t.Helper()
	sess, err := svc.OpenSession(context.Background(), req)
	require.NoError(t, err)
	status, err := sess.EnsurePrefix(context.Background(), prefix)
	require.NoError(t, err)
	require.Equal(t, warm, status.ReusedTokens > 0)
	require.NoError(t, sess.Close())
}

func TestUnit_SnapshotLifecycle(t *testing.T) {
	for _, event := range []string{"idle", "unload", "switch", "shutdown", "release", "embed"} {
		t.Run(event, func(t *testing.T) {
			svc, backend, req, prefix := snapshotFixture(t)
			if event == "release" {
				svc.idleTTL = 0
			}
			prefillSnapshot(t, svc, req, prefix)
			switch event {
			case "idle":
				svc.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
				svc.reapIdle()
			case "unload":
				require.NoError(t, svc.UnloadModel(context.Background(), transport.UnloadModelRequest{}))
			case "switch":
				_, err := svc.LoadModel(context.Background(), transport.LoadModelRequest{ModelName: "b", Type: "llama", Path: req.Path, Config: req.Config})
				require.NoError(t, err)
				require.NoError(t, svc.UnloadModel(context.Background(), transport.UnloadModelRequest{}))
			case "shutdown":
				require.NoError(t, svc.Shutdown(context.Background()))
				svc = New(backend, WithDataRoot(svc.dataRoot), WithBackend("llama"), WithIdleTTL(time.Minute))
			case "embed":
				_, _ = svc.Embed(context.Background(), transport.EmbedRequest{ModelName: "embed", Type: "llama", Path: req.Path})
			}
			assertSnapshotReuse(t, svc, req, prefix, true)
			require.Positive(t, backend.restores)
		})
	}
}

func TestUnit_SnapshotFailedRestoreReopensCleanSession(t *testing.T) {
	svc, backend, req, prefix := snapshotFixture(t)
	prefillSnapshot(t, svc, req, prefix)
	require.NoError(t, svc.UnloadModel(context.Background(), transport.UnloadModelRequest{}))
	backend.reject = true
	assertSnapshotReuse(t, svc, req, prefix, false)
	require.Equal(t, 3, backend.opens)
	require.Equal(t, 1, backend.restores)
	require.Equal(t, 2, backend.closes)
}

func TestUnit_SnapshotInvalidation(t *testing.T) {
	for _, change := range []string{"model", "template", "config", "corrupt", "expired", "disabled", "owner_lost"} {
		t.Run(change, func(t *testing.T) {
			svc, backend, req, prefix := snapshotFixture(t)
			prefillSnapshot(t, svc, req, prefix)
			if change == "owner_lost" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				svc.snapshotContext = ctx
			}
			require.NoError(t, svc.UnloadModel(context.Background(), transport.UnloadModelRequest{}))
			switch change {
			case "model":
				require.NoError(t, os.WriteFile(req.Path, []byte("changed model identity"), 0600))
			case "template":
				require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(req.Path), "chat_template.jinja"), []byte("new template"), 0600))
			case "config":
				req.Config.KVCacheType = "q8_0"
			case "corrupt":
				files, err := filepath.Glob(filepath.Join(svc.snapshotDir(), "*.snap"))
				require.NoError(t, err)
				require.Len(t, files, 1)
				require.NoError(t, os.WriteFile(files[0], []byte("broken"), 0600))
			case "expired":
				svc.now = func() time.Time { return time.Now().Add(48 * time.Hour) }
			case "disabled":
				t.Setenv("CONTENOX_WARM_SNAPSHOT_DISABLE", "1")
			}
			assertSnapshotReuse(t, svc, req, prefix, false)
			require.Zero(t, backend.restores)
		})
	}
}

func TestUnit_SnapshotDiskBudgetAndCleanup(t *testing.T) {
	svc, _, _, _ := snapshotFixture(t)
	dir := svc.snapshotDir()
	require.NoError(t, os.MkdirAll(dir, 0700))
	for _, name := range []string{"old.snap", "new.snap"} {
		file, err := os.Create(filepath.Join(dir, name))
		require.NoError(t, err)
		require.NoError(t, file.Truncate(3<<30))
		require.NoError(t, file.Close())
	}
	old := time.Now().Add(-time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(dir, "old.snap"), old, old))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".pending-interrupted"), []byte("partial"), 0600))
	svc.pruneSnapshots(dir)
	_, err := os.Stat(filepath.Join(dir, "old.snap"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, ".pending-interrupted"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(filepath.Join(dir, "new.snap"))
	require.NoError(t, err)
}

func TestUnit_SnapshotGRPCClientReopensWarmAfterUnload(t *testing.T) {
	svc, _, req, prefix := snapshotFixture(t)
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- transportgrpc.Serve(ctx, lis, svc, "owner", "llama") }()
	client, err := transportgrpc.DialLeader(lis.Addr().String(), "owner")
	require.NoError(t, err)
	defer func() { _ = client.Close(); cancel(); <-done }()
	requestCtx, requestCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer requestCancel()
	first, err := client.OpenSession(requestCtx, req)
	require.NoError(t, err)
	_, err = first.EnsurePrefix(requestCtx, prefix)
	require.NoError(t, err)
	require.NoError(t, first.Close())
	require.NoError(t, client.UnloadModel(requestCtx, transport.UnloadModelRequest{}))
	second, err := client.OpenSession(requestCtx, req)
	require.NoError(t, err)
	status, err := second.EnsurePrefix(requestCtx, prefix)
	require.NoError(t, err)
	require.Positive(t, status.ReusedTokens)
	require.NoError(t, second.Close())
}

func TestUnit_SnapshotShutdownStopsNewSessions(t *testing.T) {
	svc, _, req, prefix := snapshotFixture(t)
	prefillSnapshot(t, svc, req, prefix)
	require.NoError(t, svc.Shutdown(context.Background()))
	_, err := svc.OpenSession(context.Background(), req)
	require.ErrorIs(t, err, transport.ErrSessionClosed)
}

func TestUnit_SnapshotIdentityIncludesEngineAndAdaptersNotOwner(t *testing.T) {
	svc, _, req, _ := snapshotFixture(t)
	info := transport.ModelInfo{RuntimeDigest: "one", EffectiveContext: 1024}
	key := svc.snapshotKey(context.Background(), req, info)
	require.NotEmpty(t, key)
	req.Fence.OwnerInstanceID = "new owner"
	require.Equal(t, key, svc.snapshotKey(context.Background(), req, info))
	info.RuntimeDigest = "two"
	require.NotEqual(t, key, svc.snapshotKey(context.Background(), req, info))
	info.RuntimeDigest = "one"
	req.Adapters = []transport.AdapterSpec{{Name: "adapter", Path: req.Path, Digest: "digest", Scale: 1}}
	require.NotEqual(t, key, svc.snapshotKey(context.Background(), req, info))
}

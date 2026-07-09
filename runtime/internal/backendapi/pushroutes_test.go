package backendapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/modeld/slot"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
	"github.com/stretchr/testify/require"
)

// startFakeModeldNode serves a real slot.Service (the same shape production
// modeld serves) on a loopback port, rooted at modelsDir — so PushModel is
// exercised against real wire traffic and a real on-disk install, not a mock.
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
	t.Cleanup(cancel)
	return lis.Addr().String()
}

func newBackendMux(t *testing.T) (*http.ServeMux, libdb.DBManager, runtimetypes.Store) {
	t.Helper()
	db, err := libdb.NewSQLiteDBManager(context.Background(), filepath.Join(t.TempDir(), "push.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mux := http.NewServeMux()
	backendapi.AddBackendRoutes(mux, backendservice.New(db), &stubStateService{})
	return mux, db, runtimetypes.New(db.WithoutTransaction())
}

func TestPushModel_StreamsFileToModeldNodeAndInstallsIt(t *testing.T) {
	modelsDir := t.TempDir()
	addr := startFakeModeldNode(t, "instance-push", "llama", modelsDir)

	mux, _, store := newBackendMux(t)
	backend := &runtimetypes.Backend{ID: "backend-push", Name: "gpu-box", Type: "modeld", BaseURL: addr}
	require.NoError(t, store.CreateBackend(context.Background(), backend))

	content := []byte("fake gguf weights")
	req := httptest.NewRequest(http.MethodPost, "/backends/backend-push/models/push?name=my-qwen", bytes.NewReader(content))
	req.ContentLength = int64(len(content))
	req.Header.Set("Content-Type", "application/octet-stream")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("push status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Name           string `json:"name"`
		AlreadyPresent bool   `json:"alreadyPresent"`
		BytesWritten   int64  `json:"bytesWritten"`
	}
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	require.Equal(t, "my-qwen", resp.Name)
	require.False(t, resp.AlreadyPresent)
	require.Equal(t, int64(len(content)), resp.BytesWritten)

	// The file must actually be installed on the fake node's disk, proving the
	// upload streamed all the way through modeldconn.Endpoint's PushModel RPC.
	installed, err := os.ReadFile(filepath.Join(modelsDir, "my-qwen", "model.gguf"))
	require.NoError(t, err)
	require.Equal(t, content, installed)

	// Pushing the identical bytes again must be recognized as already present
	// (idempotent re-push), not re-installed as a duplicate.
	req2 := httptest.NewRequest(http.MethodPost, "/backends/backend-push/models/push?name=my-qwen", bytes.NewReader(content))
	req2.ContentLength = int64(len(content))
	rr2 := httptest.NewRecorder()
	mux.ServeHTTP(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)
	var resp2 struct {
		AlreadyPresent bool `json:"alreadyPresent"`
	}
	require.NoError(t, json.NewDecoder(rr2.Body).Decode(&resp2))
	require.True(t, resp2.AlreadyPresent)
}

func TestPushModel_MissingNameReturnsBadRequest(t *testing.T) {
	mux, _, store := newBackendMux(t)
	backend := &runtimetypes.Backend{ID: "backend-noname", Name: "gpu-box", Type: "modeld", BaseURL: "127.0.0.1:1"}
	require.NoError(t, store.CreateBackend(context.Background(), backend))

	req := httptest.NewRequest(http.MethodPost, "/backends/backend-noname/models/push", bytes.NewReader([]byte("x")))
	req.ContentLength = 1
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)
}

func TestPushModel_WrongBackendTypeRejected(t *testing.T) {
	mux, _, store := newBackendMux(t)
	backend := &runtimetypes.Backend{ID: "backend-ollama", Name: "cloud", Type: "ollama", BaseURL: "http://127.0.0.1:11434"}
	require.NoError(t, store.CreateBackend(context.Background(), backend))

	req := httptest.NewRequest(http.MethodPost, "/backends/backend-ollama/models/push?name=x", bytes.NewReader([]byte("x")))
	req.ContentLength = 1
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusUnprocessableEntity, rr.Code)
}

func TestPushModel_PathTraversalNameRejected(t *testing.T) {
	mux, _, store := newBackendMux(t)
	backend := &runtimetypes.Backend{ID: "backend-traversal", Name: "gpu-box", Type: "modeld", BaseURL: "127.0.0.1:1"}
	require.NoError(t, store.CreateBackend(context.Background(), backend))

	for _, name := range []string{"../evil", "a/../../evil", "..", "sub/dir"} {
		req := httptest.NewRequest(http.MethodPost, "/backends/backend-traversal/models/push?name="+name, bytes.NewReader([]byte("x")))
		req.ContentLength = 1
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		require.Equalf(t, http.StatusBadRequest, rr.Code, "name=%q: %s", name, rr.Body.String())
	}
}

func TestPushModel_UnknownBackendReturnsNotFound(t *testing.T) {
	mux, _, _ := newBackendMux(t)

	req := httptest.NewRequest(http.MethodPost, "/backends/does-not-exist/models/push?name=x", bytes.NewReader([]byte("x")))
	req.ContentLength = 1
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNotFound, rr.Code)
}

package contenoxcli

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/modeld/modelstore"
	"github.com/contenox/contenox/internal/modeld/owner"
	"github.com/contenox/contenox/internal/models/backendservice"
	"github.com/contenox/contenox/internal/models/modelregistry"
	"github.com/contenox/contenox/internal/models/modelrepo/modeldconn"
	"github.com/contenox/contenox/internal/services/clikv"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/transport"
	transportgrpc "github.com/contenox/contenox/internal/transport/grpc"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

type autoTestWorker struct{ *transport.MemoryService }

func (w *autoTestWorker) Describe(context.Context, transport.OpenSessionRequest) (transport.ModelInfo, error) {
	return transport.ModelInfo{EffectiveContext: 262144, HotContextTokens: 131072}, nil
}
func (w *autoTestWorker) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	req.Config.NumCtx = 262144
	s, err := w.MemoryService.OpenSession(ctx, req)
	return &autoTestSession{Session: s}, err
}

type autoTestSession struct{ transport.Session }

func (s *autoTestSession) ExplainContext() transport.ContextReport {
	report := s.Session.ExplainContext()
	report.HotContextTokens = 131072
	return report
}

func (s *autoTestSession) Decode(context.Context, transport.DecodeConfig) (<-chan transport.StreamChunk, error) {
	out := make(chan transport.StreamChunk, 1)
	call := transport.ToolCall{ID: "setup", Type: "function"}
	call.Function.Name = "contenox_ready"
	call.Function.Arguments = "{}"
	out <- transport.StreamChunk{ToolCalls: []transport.ToolCall{call}, FinishReason: "tool_calls"}
	close(out)
	return out, nil
}

func TestSystem_Auto_VerifiesToolAndOpenedHotContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	root := t.TempDir()
	oldRoot := modeldconn.DataRoot()
	modeldconn.SetDataRoot(root)
	defer modeldconn.SetDataRoot(oldRoot)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	lease, err := owner.Join(ctx, owner.Config{LeasePath: filepath.Join(root, "modeld.lease"), TTL: time.Minute, Endpoint: listener.Addr().String(), Backend: "llama"})
	require.NoError(t, err)
	defer lease.Release()
	go func() {
		_ = transportgrpc.Serve(ctx, listener, &autoTestWorker{transport.NewMemoryService()}, lease.InstanceID(), "llama")
	}()
	hot, err := verifyAutoModel(ctx, io.Discard, modelregistry.ModelDescriptor{Name: "setup-test", Backend: "llama"}, 131072)
	require.NoError(t, err)
	require.Equal(t, 131072, hot)
}

func TestUnit_Auto_DefaultsKeepRemoteBackendAndReplaceCloudFallback(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "runtime.db")
	cmd := &cobra.Command{}
	cmd.Flags().String("db", path, "")
	db, err := OpenDBAt(ctx, path)
	require.NoError(t, err)
	svc := backendservice.New(db)
	require.NoError(t, svc.Create(ctx, &runtimetypes.Backend{ID: "remote", Name: "remote", Type: "modeld", BaseURL: "192.0.2.1:9000"}))
	store := runtimetypes.New(db.WithoutTransaction())
	require.NoError(t, clikv.WriteConfig(ctx, store, "", "default-alt-provider", "openai"))
	require.NoError(t, db.Close())
	require.NoError(t, saveAutoDefaults(ctx, cmd, modelregistry.ModelDescriptor{Name: "selected"}))
	db, err = OpenDBAt(ctx, path)
	require.NoError(t, err)
	defer db.Close()
	store = runtimetypes.New(db.WithoutTransaction())
	require.Equal(t, "modeld", clikv.Read(ctx, store, "default-alt-provider"))
	require.Equal(t, "selected", clikv.Read(ctx, store, "default-model"))
	require.Empty(t, clikv.Read(ctx, store, "default-token-limit"),
		"auto selects a model; the window mask stays unset because the worker reports its own capacity")
	entries, err := backendservice.New(db).List(ctx, nil, 100)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	for _, entry := range entries {
		if entry.ID == "remote" {
			require.Equal(t, "192.0.2.1:9000", entry.BaseURL)
		}
	}
}

func TestUnit_Auto_PreviousSelectionDoesNotCreateDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	cmd := &cobra.Command{}
	cmd.Flags().String("db", path, "")
	_, found := previousAutoModel(context.Background(), cmd, "", "llama", 16<<30)
	require.False(t, found)
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
}

func TestSystem_Auto_NativeToolProbe(t *testing.T) {
	modelPath := os.Getenv("CONTENOX_AUTO_TEST_GGUF")
	if modelPath == "" {
		t.Skip("set CONTENOX_AUTO_TEST_GGUF and CONTENOX_MODELD_BIN for native inference")
	}
	testAutoNativeToolProbe(t, "llama", modelPath)
}

func TestSystem_Auto_OpenVINOToolProbe(t *testing.T) {
	modelPath := os.Getenv("CONTENOX_AUTO_TEST_OPENVINO")
	if modelPath == "" {
		t.Skip("set CONTENOX_AUTO_TEST_OPENVINO and CONTENOX_MODELD_BIN for native inference")
	}
	testAutoNativeToolProbe(t, "openvino", modelPath)
}

func testAutoNativeToolProbe(t *testing.T, backend, modelPath string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("CONTENOX_DATA_ROOT", root)
	t.Setenv("CONTENOX_MODELD_BACKEND", backend)
	oldRoot := modeldconn.DataRoot()
	modeldconn.SetDataRoot(root)
	defer modeldconn.SetDataRoot(oldRoot)
	defer modeldconn.StopAutoStarted()
	dir := filepath.Join(root, "models", "setup-native")
	if backend == "openvino" {
		require.NoError(t, os.MkdirAll(dir, 0700))
		entries, err := os.ReadDir(modelPath)
		require.NoError(t, err)
		for _, entry := range entries {
			if entry.Name() != "contenox-openvino.json" {
				require.NoError(t, os.Symlink(filepath.Join(modelPath, entry.Name()), filepath.Join(dir, entry.Name())))
			}
		}
		require.NoError(t, modelstore.WriteModelProfile(backend, dir, "openvino:json_schema_tool_calls", "", ""))
	} else {
		require.NoError(t, os.MkdirAll(dir, 0700))
		require.NoError(t, os.Symlink(modelPath, filepath.Join(dir, "model.gguf")))
	}
	hot, err := verifyAutoModel(context.Background(), io.Discard, modelregistry.ModelDescriptor{Name: "setup-native", Backend: backend}, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, hot, 4096)
}

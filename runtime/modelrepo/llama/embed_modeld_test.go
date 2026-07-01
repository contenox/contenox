package llama

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// recordingEmbedService is a transport.Service double that captures the embed
// request the runtime sends and returns a fixed vector, so the daemon-side embed
// contract (model identity + text reaching modeld) can be asserted without real
// inference.
type recordingEmbedService struct {
	base   *transport.MemoryService
	vector []float32

	mu       sync.Mutex
	embed    transport.EmbedRequest
	describe transport.OpenSessionRequest
	info     transport.ModelInfo
}

func (s *recordingEmbedService) OpenSession(ctx context.Context, req transport.OpenSessionRequest) (transport.Session, error) {
	return s.base.OpenSession(ctx, req)
}

func (s *recordingEmbedService) Describe(ctx context.Context, req transport.OpenSessionRequest) (transport.ModelInfo, error) {
	s.mu.Lock()
	s.describe = req
	info := s.info
	s.mu.Unlock()
	if info.EffectiveContext > 0 || info.PlannerEffectiveContext > 0 || info.ModelMaxContext > 0 || info.RuntimeName != "" || info.RuntimeDigest != "" {
		return info, nil
	}
	return s.base.Describe(ctx, req)
}

func (s *recordingEmbedService) Embed(_ context.Context, req transport.EmbedRequest) (transport.EmbedResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embed = req
	return transport.EmbedResult{Vector: s.vector}, nil
}

func (s *recordingEmbedService) lastEmbedRequest() transport.EmbedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.embed
}

func (s *recordingEmbedService) lastDescribeRequest() transport.OpenSessionRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.describe
}

func serveLlamaModeldForTest(t *testing.T, svc transport.Service) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()
	dataRoot := t.TempDir()
	leasePath := filepath.Join(dataRoot, "modeld.lease")
	lease, err := liblease.Acquire(leasePath, 30*time.Second, liblease.WithMeta(map[string]string{"endpoint": endpoint, "backend": "llama"}))
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	t.Cleanup(func() { _ = lease.Release() })
	rec, err := liblease.Inspect(leasePath)
	if err != nil {
		t.Fatalf("inspect lease: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = transportgrpc.Serve(ctx, lis, svc, rec.InstanceID, "llama") }()
	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
	deadline := time.Now().Add(2 * time.Second)
	for {
		pingCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		client, err := transportgrpc.DialLeader(endpoint, "")
		if err == nil {
			h, healthErr := client.Health(pingCtx)
			_ = client.Close()
			if healthErr == nil && h.Ready && h.InstanceID == rec.InstanceID {
				cancel()
				break
			}
		}
		cancel()
		if time.Now().After(deadline) {
			t.Fatalf("modeld test server did not become healthy at %s", endpoint)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestUnit_LlamaProvider_EmbeddingUsesModeldTransport proves llama embeddings now
// have the same daemon arm as chat: with no native backend compiled in (pure-Go),
// the provider still advertises embeddings when modeld serves llama, and Embed
// routes over runtime/transport carrying the model identity. Before, the embed
// path had only an in-process arm, so the shipped pure-Go CLI reported llama
// embeddings as unavailable (notWired) while chat worked over the daemon.
func TestUnit_LlamaProvider_EmbeddingUsesModeldTransport(t *testing.T) {
	old := sessionFactory
	sessionFactory = nil
	t.Cleanup(func() { sessionFactory = old })
	oldEmbed := embedFunc
	embedFunc = nil
	t.Cleanup(func() { embedFunc = oldEmbed })

	root := t.TempDir()
	modelDir := filepath.Join(root, "embedder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, profileFileName), []byte(`{"model_digest":"sha256:test"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := &recordingEmbedService{base: transport.NewMemoryService(), vector: []float32{1.25, -0.5, 3}}
	serveLlamaModeldForTest(t, svc)

	p := newProvider("embedder", root, modelrepo.CapabilityConfig{ContextLength: 8192})
	if !p.CanEmbed() {
		t.Fatal("llama should advertise embeddings when modeld serves the llama backend")
	}

	conn, err := p.GetEmbedConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetEmbedConnection: %v", err)
	}
	got, err := conn.Embed(context.Background(), "hello embeddings")
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	want := []float64{1.25, -0.5, 3}
	if len(got) != len(want) {
		t.Fatalf("embedding length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("embedding[%d] = %v, want %v", i, got[i], want[i])
		}
	}

	req := svc.lastEmbedRequest()
	if req.Type != "llama" || req.ModelName != "embedder" || req.Path != filepath.Join(modelDir, "model.gguf") || req.Digest != "sha256:test" {
		t.Fatalf("Embed request missing model identity: %+v", req)
	}
	if req.Text != "hello embeddings" {
		t.Fatalf("Embed text = %q", req.Text)
	}
}

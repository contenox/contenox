package openvino

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// TestE2E_RuntimeOpenVINODialsModeld proves the full runtime->modeld loop for the
// reshaped OpenVINO provider: client.Chat builds the prompt plan, resolves the
// lease leader, dials it, and drives EnsurePrefix -> PrefillSuffix -> Decode on
// the resident session. modeld serves the noop MemoryService here, isolating the
// wiring from real inference (proven separately by the OpenVINO adapter
// benchmark).
func TestE2E_RuntimeOpenVINODialsModeld(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()

	dataRoot := t.TempDir()
	leasePath := filepath.Join(dataRoot, "modeld.lease")
	lease, err := liblease.Acquire(leasePath, 30*time.Second, liblease.WithMeta(map[string]string{"endpoint": endpoint}))
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
	go func() { _ = transportgrpc.Serve(ctx, lis, transport.NewMemoryService(), rec.InstanceID) }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })

	c := &client{
		modelPath:       "/ir/coder",
		profileID:       "coder",
		modelDigest:     "coder",
		cfg:             Config{NumCtx: 512, PromptFormat: "chatml"},
		maxOutputTokens: 8,
	}
	res, err := c.Chat(context.Background(), []modelrepo.Message{
		{Role: "system", Content: "You are a precise coding assistant."},
		{Role: "user", Content: "hello"},
	})
	if err != nil {
		t.Fatalf("Chat over the wire: %v", err)
	}
	got := strings.TrimSpace(res.Message.Content)
	if got == "" {
		t.Fatal("Chat produced no content over the wire")
	}
	if got != "xxxxxxxx" { // MemoryService emits maxOutputTokens 'x' tokens
		t.Fatalf("decoded %q over the wire, want 8 'x' tokens", got)
	}
}

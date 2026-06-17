package llama

import (
	"context"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// TestE2E_RuntimeLlamaDialsModeld proves the full runtime->modeld loop over the
// gRPC transport: the runtime-side llama session resolves the lease leader,
// dials it, opens a session, and drives EnsurePrefix -> PrefillSuffix -> Decode
// on the resident (here, in-memory) session. No CGO: modeld serves the noop
// MemoryService, so this isolates the wiring from real inference (proven
// separately by the OpenVINO adapter benchmark).
func TestE2E_RuntimeLlamaDialsModeld(t *testing.T) {
	// Reserve the port first so the lease advertises it, then serve the lease's
	// own instance id (a consistent, healthy owner).
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

	// Runtime side: open a session purely through the package API. newSession
	// dials modeld; the returned Session is resident in the daemon.
	sess, err := newSession("/models/foo/model.gguf", Config{NumCtx: 100})
	if err != nil {
		t.Fatalf("newSession (dial modeld): %v", err)
	}
	defer sess.Close()

	manifest := ContextManifest{
		Backend:              "llama",
		ModelDigest:          "d1",
		PromptFormat:         "chatml",
		PromptTemplateDigest: "t1",
		RuntimeDigest:        "r1",
		StableByteHash:       contextasm.HashString("hello"),
	}
	if _, err := sess.EnsurePrefix(context.Background(), PrefixInput{Text: "hello", Manifest: manifest}); err != nil {
		t.Fatalf("EnsurePrefix over wire: %v", err)
	}
	if _, err := sess.PrefillSuffix(context.Background(), SuffixInput{Text: " world", Manifest: manifest}); err != nil {
		t.Fatalf("PrefillSuffix over wire: %v", err)
	}
	ch, err := sess.Decode(context.Background(), DecodeConfig{MaxTokens: 3})
	if err != nil {
		t.Fatalf("Decode over wire: %v", err)
	}
	var out strings.Builder
	for c := range ch {
		if c.Error != nil {
			t.Fatalf("stream error: %v", c.Error)
		}
		out.WriteString(c.Text)
	}
	if out.String() != "xxx" {
		t.Fatalf("decoded %q over the wire, want %q", out.String(), "xxx")
	}
}

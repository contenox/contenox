//go:build openvino && openvino_genai

package openvino

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	modeldopenvino "github.com/contenox/runtime/modeld/openvino"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// TestSystem_RuntimeOpenVINOEndToEndOnDevice is the full, real-inference loop:
// the runtime-side OpenVINO provider dials a modeld serving the actual CGO
// OpenVINO GenAI backend in-process, and a chat turn produces real tokens on the
// configured device. This connects the two halves that were proven separately
// (the transport wire, and the GenAI adapter) into one end-to-end path.
//
// Provision + run: make -f Makefile.openvino test-s1-5  (sets the model/device).
func TestSystem_RuntimeOpenVINOEndToEndOnDevice(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL (see Makefile.openvino)")
	}

	// modeld, in-process, serving the real OpenVINO backend.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()
	dataRoot := t.TempDir()
	leasePath := filepath.Join(dataRoot, "modeld.lease")
	lease, err := liblease.Acquire(leasePath, 60*time.Second, liblease.WithMeta(map[string]string{"endpoint": endpoint}))
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
	go func() { _ = transportgrpc.Serve(ctx, lis, &modeldopenvino.Service{}, rec.InstanceID) }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })

	// Runtime side: point the client straight at the IR model dir, with identity
	// derived from the model's own files (template digest from its chat_template).
	modelDigest, templateDigest := modelIdentity(modelDir)
	c := &client{
		modelPath:   modelDir,
		profileID:   "test",
		modelDigest: modelDigest,
		cfg: Config{
			NumCtx:               2048,
			PromptFormat:         "openvino-chat-template",
			PromptTemplateDigest: templateDigest,
		},
		maxOutputTokens: 24,
	}
	if templateDigest == "" {
		t.Log("warning: model has no chat_template; adapter will fall back to raw text")
	}
	start := time.Now()
	res, err := c.Chat(context.Background(), []modelrepo.Message{
		{Role: "user", Content: "Write one short line of Go that prints hello."},
	})
	if err != nil {
		t.Fatalf("Chat end-to-end: %v", err)
	}
	t.Logf("device=%s  %s  -> %q", os.Getenv("CONTENOX_OPENVINO_TEST_DEVICE"), time.Since(start), res.Message.Content)
	if strings.TrimSpace(res.Message.Content) == "" {
		t.Fatal("end-to-end chat produced no tokens")
	}
}

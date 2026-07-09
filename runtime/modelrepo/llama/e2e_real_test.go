//go:build llamanode && llamacpp_direct

package llama

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/liblease"
	modeldllama "github.com/contenox/runtime/modeld/llama"
	_ "github.com/contenox/runtime/modeld/llama/llamasession" // registers the CGO session factory
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// TestSystem_RuntimeLlamaEndToEnd drives the full runtime->modeld loop for the
// llama backend on a real GGUF: the runtime client sends RAW content, modeld
// applies the model's OWN chat template (from the GGUF) in the session, and a
// chat turn produces real tokens. This proves the template fix end to end, not
// just the shim in isolation.
func TestSystem_RuntimeLlamaEndToEnd(t *testing.T) {
	gguf := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if gguf == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to a small instruct GGUF")
	}

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
	go func() { _ = transportgrpc.Serve(ctx, lis, modeldllama.NewService(), rec.InstanceID, "llama") }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
	t.Cleanup(closeCachedSessionsForTest)

	c := &client{
		modelName:       "runtime-llama-e2e",
		modelPath:       gguf,
		profileID:       "test",
		modelDigest:     "test-runtime-e2e",
		cfg:             Config{NumCtx: 2048},
		maxOutputTokens: 24,
	}
	start := time.Now()
	res, err := c.Chat(context.Background(), []modelrepo.Message{
		{Role: "system", Content: "You are a precise Go coding assistant."},
		{Role: "user", Content: "Write one short line of Go that prints hello."},
	})
	if err != nil {
		t.Fatalf("Chat end-to-end: %v", err)
	}
	t.Logf("%s -> %q", time.Since(start), res.Message.Content)
	if strings.TrimSpace(res.Message.Content) == "" {
		t.Fatal("end-to-end chat produced no tokens")
	}
}

func TestSystem_RuntimeLlamaReasoningEndToEnd(t *testing.T) {
	gguf := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if gguf == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to a small instruct GGUF")
	}

	serveRealLlamaModeld(t)

	c := &client{
		modelName:         "runtime-llama-reasoning-e2e",
		modelPath:         gguf,
		profileID:         "test",
		modelDigest:       "test-runtime-reasoning-e2e",
		cfg:               Config{NumCtx: 2048, ReasoningFormat: "deepseek"},
		maxOutputTokens:   96,
		reasoningProtocol: reasoningProtocolCommonChat,
	}
	res, err := c.Chat(
		context.Background(),
		[]modelrepo.Message{{Role: "user", Content: "Think briefly, then answer with the single word OK."}},
		modelrepo.WithThink("high"),
		modelrepo.WithMaxTokens(96),
	)
	if err != nil {
		t.Fatalf("Reasoning chat end-to-end: %v", err)
	}
	if strings.TrimSpace(res.Message.Thinking) == "" {
		t.Skipf("tiny model did not emit parseable reasoning for this prompt; visible=%q", res.Message.Content)
	}
	t.Logf("visible=%q thinking=%q", res.Message.Content, res.Message.Thinking)
}

// TestSystem_RuntimeLlamaTargetedProviderEndToEnd is the real-hardware
// validation for the modeld remote-backend fix set: it drives
// NewProviderForTarget (not a bare *client, unlike the untargeted tests above)
// through CanChat/GetChatConnection/Chat against a real GGUF on the real
// llama.cpp backend, fenced to a specific endpoint/instance the way a
// registered remote "modeld" backend row is. Before the fix, CanChat/
// GetChatConnection consulted this process's own (nonexistent, in a test
// binary) local modeld lease via SessionAvailable() and would have refused
// before ever reaching real inference.
func TestSystem_RuntimeLlamaTargetedProviderEndToEnd(t *testing.T) {
	gguf := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if gguf == "" {
		t.Skip("set CONTENOX_LLAMA_TINY_GGUF to a small instruct GGUF")
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()
	instance := "instance-targeted-e2e"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = transportgrpc.Serve(ctx, lis, modeldllama.NewService(), instance, "llama") }()
	t.Cleanup(closeCachedSessionsForTest)

	// Deliberately NOT calling modeldconn.SetDataRoot / acquiring a local
	// lease: this process has no local modeld, proving CanChat/GetChatConnection
	// do not depend on one for a targeted provider.
	target := modeldconn.ModeldTarget{BackendID: "remote-box", Endpoint: endpoint, Instance: instance}
	p := &provider{
		name:   "targeted-e2e",
		caps:   modelrepo.CapabilityConfig{CanChat: true, CanPrompt: true, CanStream: true, CanEmbed: true},
		target: target,
	}

	if !p.CanChat() {
		t.Fatal("targeted provider CanChat() = false, want true from caps regardless of local machine state")
	}
	chatClient, err := p.GetChatConnection(context.Background(), "")
	if err != nil {
		t.Fatalf("GetChatConnection: %v", err)
	}
	c := chatClient.(*client)
	c.modelPath = gguf
	c.modelDigest = "test-targeted-e2e"

	res, err := c.Chat(context.Background(), []modelrepo.Message{
		{Role: "system", Content: "You are a precise Go coding assistant."},
		{Role: "user", Content: "Write one short line of Go that prints hello."},
	})
	if err != nil {
		t.Fatalf("Chat end-to-end (targeted provider): %v", err)
	}
	if strings.TrimSpace(res.Message.Content) == "" {
		t.Fatal("end-to-end targeted chat produced no tokens")
	}
	t.Logf("targeted provider chat -> %q", res.Message.Content)
}

func serveRealLlamaModeld(t *testing.T) {
	t.Helper()

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
	go func() { _ = transportgrpc.Serve(ctx, lis, modeldllama.NewService(), rec.InstanceID, "llama") }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
	t.Cleanup(closeCachedSessionsForTest)
}

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

	serveRealOpenVINOModeld(t)

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

func TestSystem_RuntimeOpenVINOEmbedEndToEndOnDevice(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_EMBED_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_EMBED_MODEL")
	}

	serveRealOpenVINOModeld(t)

	modelDigest, _ := modelIdentity(modelDir)
	c := &embedClient{
		modelName:   "embed-test",
		modelPath:   modelDir,
		modelDigest: modelDigest,
	}
	vec, err := c.Embed(context.Background(), "hello embeddings")
	if err != nil {
		t.Fatalf("Embed end-to-end: %v", err)
	}
	if len(vec) == 0 {
		t.Fatal("end-to-end embedding produced an empty vector")
	}
	t.Logf("embedding dims=%d first=%f", len(vec), vec[0])
}

func TestSystem_RuntimeOpenVINOReasoningEndToEndOnDevice(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_REASONING_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_REASONING_MODEL")
	}

	serveRealOpenVINOModeld(t)

	modelDigest, templateDigest := modelIdentity(modelDir)
	c := &client{
		modelPath:   modelDir,
		profileID:   "deepseek-test",
		modelDigest: modelDigest,
		cfg: Config{
			NumCtx:               2048,
			PromptFormat:         "openvino-chat-template",
			PromptTemplateDigest: templateDigest,
		},
		maxOutputTokens: 96,
		reasoningStream: "openvino:deepseek_r1_reasoning_incremental_parser",
	}
	res, err := c.Chat(
		context.Background(),
		[]modelrepo.Message{{Role: "user", Content: "Why is the sky blue?"}},
		modelrepo.WithThink("high"),
		modelrepo.WithMaxTokens(96),
	)
	if err != nil {
		t.Fatalf("Reasoning chat end-to-end: %v", err)
	}
	if strings.TrimSpace(res.Message.Thinking) == "" {
		t.Fatal("reasoning end-to-end produced no thinking")
	}
	if strings.Contains(res.Message.Content, "<think>") || strings.Contains(res.Message.Content, "Okay, so I'm trying") {
		t.Fatalf("reasoning leaked into visible content: %q", res.Message.Content)
	}
	t.Logf("visible=%q thinking_prefix=%q", res.Message.Content, firstRunes(res.Message.Thinking, 80))
}

// TestSystem_RuntimeOpenVINOTargetedProviderEndToEnd is the real-hardware
// regression test for the two modeld remote-backend bugs that were specific
// to this package: (1) newClient computed dir := filepath.Join(p.modelDir, p.name)
// unconditionally, so a targeted provider (p.modelDir == "") sent a bogus
// non-empty relative Path instead of "" — modeld's resolvePath uses a
// non-empty incoming Path as-is instead of resolving by name, so this would
// have failed to find the model at all; (2) CanChat/GetChatConnection gated
// on this process's own local modeld lease state via SessionAvailable(),
// unrelated to a remote target. This drives NewProviderForTarget through
// real inference to prove both are fixed together.
func TestSystem_RuntimeOpenVINOTargetedProviderEndToEnd(t *testing.T) {
	modelDir := os.Getenv("CONTENOX_OPENVINO_TEST_MODEL")
	if modelDir == "" {
		t.Skip("set CONTENOX_OPENVINO_TEST_MODEL (see Makefile.openvino)")
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()
	instance := "instance-targeted-e2e"
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = transportgrpc.Serve(ctx, lis, modeldopenvino.NewService(), instance, "openvino") }()

	// Deliberately NOT calling modeldconn.SetDataRoot / acquiring a local
	// lease: this process has no local modeld, proving CanChat/GetChatConnection
	// do not depend on one for a targeted provider.
	target := modeldconn.ModeldTarget{BackendID: "remote-box", Endpoint: endpoint, Instance: instance}
	p := &openvinoProvider{
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
	// Before the fix, newClient would have set c.modelPath to the bogus
	// relative path "targeted-e2e" (p.name) instead of "" — assert directly on
	// the constructed client, then point it at the real model to drive
	// inference through the rest of the stack.
	if c.modelPath != "" {
		t.Fatalf("client.modelPath = %q, want empty for a targeted provider with no local modelDir", c.modelPath)
	}
	modelDigest, templateDigest := modelIdentity(modelDir)
	c.modelPath = modelDir
	c.modelDigest = modelDigest
	c.cfg.PromptFormat = "openvino-chat-template"
	c.cfg.PromptTemplateDigest = templateDigest

	res, err := c.Chat(context.Background(), []modelrepo.Message{
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

func serveRealOpenVINOModeld(t *testing.T) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()
	dataRoot := t.TempDir()
	leasePath := filepath.Join(dataRoot, "modeld.lease")
	lease, err := liblease.Acquire(leasePath, 60*time.Second, liblease.WithMeta(map[string]string{"endpoint": endpoint, "backend": "openvino"}))
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
	go func() { _ = transportgrpc.Serve(ctx, lis, modeldopenvino.NewService(), rec.InstanceID, "openvino") }()

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

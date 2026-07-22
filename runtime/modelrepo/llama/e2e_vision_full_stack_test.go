//go:build llamanode && llamacpp_direct

// Package llama_test (external) hosts the full-stack vision e2e: it needs
// enginesvc/runtimestate/compatapi, which import this package — an in-package
// test file would be an import cycle.
package llama_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/liblease"
	"github.com/contenox/runtime/libtracker"
	modeldllama "github.com/contenox/runtime/modeld/llama"
	_ "github.com/contenox/runtime/modeld/llama/llamasession" // registers the real CGO session factory on the daemon side
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/enginesvc"
	"github.com/contenox/runtime/runtime/internal/compatapi"
	"github.com/contenox/runtime/runtime/localfileservice"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/taskchainservice"
	"github.com/contenox/runtime/runtime/taskengine"
	transportgrpc "github.com/contenox/runtime/runtime/transport/grpc"
)

// TestSystem_VisionFullStackCompatHTTPE2E is the flagship proof for the vision
// program: a real image travels the entire serving stack —
//
//	HTTP POST /openai/v1/chat/completions (OpenAI content-parts, data: URI)
//	→ compatapi decode (image_url → taskengine.ImagePart)
//	→ agentservice → taskengine chat_completion → llmrepo
//	→ llmresolver (RequiresVision derived from the messages, CanVision gate)
//	→ llama provider (MediaMarker + SuffixInput.Images over the transport wire)
//	→ live modeld daemon (real CGO llama.cpp + mtmd session)
//	→ SmolVLM2 on the device → answer
//
// with NO mocked piece. The catalog leg is the production one too: a "llama"
// backend row over a models dir, reconciled by RunBackendCycle, with the live
// daemon's Describe answering SupportsVision (authoritative over the offline
// mmproj file signal).
//
// The negative path runs live as well: the same image request pinned (via a
// second chain) to a text-only model must be refused with the typed
// no-vision-capable resolver error at the API layer — never silently degraded
// to a text model that would drop the image.
//
// Run: make -f Makefile.llamacpp-direct test-runtime-vision
// (fetch models first: deps-vlm-model + deps-tiny-model)
func TestSystem_VisionFullStackCompatHTTPE2E(t *testing.T) {
	vlmGGUF := os.Getenv("CONTENOX_LLAMA_VLM_GGUF")
	vlmMMProj := os.Getenv("CONTENOX_LLAMA_VLM_MMPROJ")
	if vlmGGUF == "" || vlmMMProj == "" {
		t.Skip("set CONTENOX_LLAMA_VLM_GGUF and CONTENOX_LLAMA_VLM_MMPROJ (make -f Makefile.llamacpp-direct deps-vlm-model)")
	}
	tinyGGUF := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")

	serveVisionModeld(t)

	// Models dir in the production two-file layout: <dir>/<name>/model.gguf
	// (+ mmproj.gguf for the VLM). Symlinks keep the multi-GB weights shared
	// with the session-level test fixtures.
	modelsDir := t.TempDir()
	linkModel(t, modelsDir, "smolvlm2-e2e", vlmGGUF, vlmMMProj)
	if tinyGGUF != "" {
		linkModel(t, modelsDir, "tiny-text-e2e", tinyGGUF, "")
	}

	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "vision-e2e.db"), runtimetypes.SchemaSQLite)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := runtimetypes.New(db.WithoutTransaction())
	if err := store.CreateBackend(ctx, &runtimetypes.Backend{
		ID:      "local-llama",
		Name:    "local-llama",
		Type:    "llama",
		BaseURL: modelsDir,
	}); err != nil {
		t.Fatalf("create backend: %v", err)
	}

	eng, err := enginesvc.Build(ctx, db, enginesvc.Config{
		DefaultModel:    "smolvlm2-e2e",
		DefaultProvider: "llama",
		WorkspaceID:     "vision-e2e",
	})
	if err != nil {
		t.Fatalf("build engine: %v", err)
	}
	t.Cleanup(eng.Stop)

	// Resolver-gate cross-check on the REAL catalog path: the providers the
	// resolver will see, built by LocalProviderAdapter from the reconciled
	// backend state, must carry CanVision as certified by the live daemon's
	// Describe (SmolVLM2 true; the text-only tiny model false).
	getModels := runtimestate.LocalProviderAdapter(ctx, libtracker.NoopTracker{}, eng.State.Get(ctx))
	providers, err := getModels(ctx, "llama")
	if err != nil {
		t.Fatalf("provider adapter: %v", err)
	}
	visionSeen, textSeen := false, false
	for _, p := range providers {
		switch p.ModelName() {
		case "smolvlm2-e2e":
			visionSeen = true
			if !p.CanVision() {
				t.Fatal("live catalog: smolvlm2-e2e CanVision = false, want true from the daemon's Describe (SupportsVision)")
			}
		case "tiny-text-e2e":
			textSeen = true
			if p.CanVision() {
				t.Fatal("live catalog: tiny-text-e2e CanVision = true, want false (no mmproj, Describe says no vision)")
			}
		}
	}
	if !visionSeen {
		t.Fatalf("live catalog did not list smolvlm2-e2e; providers=%d", len(providers))
	}
	if tinyGGUF != "" && !textSeen {
		t.Fatalf("live catalog did not list tiny-text-e2e; providers=%d", len(providers))
	}

	// Real chain store (the same taskchainservice serve uses), with one
	// single-task chat_completion chain per model under test.
	chainFiles, err := localfileservice.NewPrivileged(t.TempDir())
	if err != nil {
		t.Fatalf("chain files: %v", err)
	}
	chains := taskchainservice.NewLocal(chainFiles)
	if err := chains.CreateAtPath(ctx, "vision-e2e.json", visionE2EChain("vision-e2e", "smolvlm2-e2e")); err != nil {
		t.Fatalf("create vision chain: %v", err)
	}
	if tinyGGUF != "" {
		if err := chains.CreateAtPath(ctx, "text-e2e.json", visionE2EChain("text-e2e", "tiny-text-e2e")); err != nil {
			t.Fatalf("create text chain: %v", err)
		}
	}

	agent := agentservice.New(agentservice.Deps{Engine: eng, DB: db, WorkspaceID: "vision-e2e"})
	mux := http.NewServeMux()
	compatapi.AddOpenAIRoutes(mux, compatapi.CompatDeps{
		Agent:  agent,
		Chains: chains,
		Defaults: stateservice.RuntimeDefaults{
			ChainRef: "vision-e2e",
			Model:    "smolvlm2-e2e",
			Provider: "llama",
		},
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	httpClient := &http.Client{Timeout: 15 * time.Minute}

	body := compatImageRequestBody(t)

	t.Run("image answered by the vision model", func(t *testing.T) {
		start := time.Now()
		resp, err := httpClient.Post(srv.URL+"/openai/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST chat completion: %v", err)
		}
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200; body: %s", resp.StatusCode, payload)
		}
		var out struct {
			Object  string `json:"object"`
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &out); err != nil {
			t.Fatalf("decode response: %v; body: %s", err, payload)
		}
		if out.Object != "chat.completion" || len(out.Choices) == 0 {
			t.Fatalf("unexpected completion shape: %s", payload)
		}
		answer := out.Choices[0].Message.Content
		t.Logf("full-stack vision answer in %s: %q", time.Since(start), answer)
		if !strings.Contains(strings.ToLower(answer), "red") {
			t.Fatalf("answer %q does not identify the solid-red image", answer)
		}
	})

	t.Run("image refused by a text-only model", func(t *testing.T) {
		if tinyGGUF == "" {
			t.Skip("set CONTENOX_LLAMA_TINY_GGUF for the live negative path")
		}
		resp, err := httpClient.Post(srv.URL+"/openai/text-e2e/v1/chat/completions", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("POST chat completion: %v", err)
		}
		defer resp.Body.Close()
		payload, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("image request to a text-only model returned 200 — images were silently dropped; body: %s", payload)
		}
		bodyText := string(payload)
		if !strings.Contains(bodyText, "vision") {
			t.Fatalf("error does not carry the typed no-vision-capable diagnosis; status=%d body: %s", resp.StatusCode, bodyText)
		}
		t.Logf("typed refusal surfaced at the API layer (status %d): %.300s", resp.StatusCode, bodyText)
	})
}

// visionE2EChain is a minimal single-task chat chain pinned to one model on
// the llama provider — deterministic decode, bounded output.
func visionE2EChain(id, model string) *taskengine.TaskChainDefinition {
	temp := float32(0)
	maxTokens := 48
	return &taskengine.TaskChainDefinition{
		ID:          id,
		Description: "Vision e2e fixture: one chat_completion task pinned to " + model,
		TokenLimit:  8192,
		Tasks: []taskengine.TaskDefinition{{
			ID:          "chat",
			Description: "Answer the user's (possibly image-bearing) chat turn.",
			Handler:     taskengine.HandleChatCompletion,
			ExecuteConfig: &taskengine.LLMExecutionConfig{
				Model:       model,
				Provider:    "llama",
				Temperature: &temp,
				MaxTokens:   &maxTokens,
			},
		}},
	}
}

// compatImageRequestBody builds the OpenAI wire form: content parts with a
// text question and a synthetic solid-red PNG as a base64 data: URI.
func compatImageRequestBody(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 224, 224))
	for y := 0; y < 224; y++ {
		for x := 0; x < 224; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	dataURI := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())

	req := map[string]any{
		"model":  "default",
		"stream": false,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "What color is this image? Answer with one word."},
				{"type": "image_url", "image_url": map[string]any{"url": dataURI}},
			},
		}},
	}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return body
}

// linkModel lays out <modelsDir>/<name>/model.gguf (+ mmproj.gguf) as symlinks
// to the shared test fixtures.
func linkModel(t *testing.T, modelsDir, name, gguf, mmproj string) {
	t.Helper()
	dir := filepath.Join(modelsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	abs := func(p string) string {
		a, err := filepath.Abs(p)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	if err := os.Symlink(abs(gguf), filepath.Join(dir, "model.gguf")); err != nil {
		t.Fatal(err)
	}
	if mmproj != "" {
		if err := os.Symlink(abs(mmproj), filepath.Join(dir, "mmproj.gguf")); err != nil {
			t.Fatal(err)
		}
	}
}

// serveVisionModeld runs a real modeld llama daemon (actual CGO sessions) on a
// loopback port and installs its lease as this process's local modeld, exactly
// like serveRealLlamaModeld in the in-package e2e tests — duplicated here
// because that helper is unexported to this external test package. The lease
// carries the backend meta key so modeldprobe reports the llama engine
// (SessionAvailable gates the catalog on it), and is renewed in the background
// because this test runs longer than the TTL.
func serveVisionModeld(t *testing.T) {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	endpoint := lis.Addr().String()
	dataRoot := t.TempDir()
	leasePath := filepath.Join(dataRoot, "modeld.lease")
	lease, err := liblease.Acquire(leasePath, 60*time.Second, liblease.WithMeta(map[string]string{
		"endpoint": endpoint,
		"backend":  "llama",
	}))
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}
	rec, err := liblease.Inspect(leasePath)
	if err != nil {
		t.Fatalf("inspect lease: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = transportgrpc.Serve(ctx, lis, modeldllama.NewService(), rec.InstanceID, "llama") }()
	go func() {
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := lease.Renew(); err != nil {
					fmt.Printf("vision e2e: lease renew failed: %v\n", err)
					return
				}
			}
		}
	}()
	t.Cleanup(func() { _ = lease.Release() })

	modeldconn.SetDataRoot(dataRoot)
	t.Cleanup(func() { modeldconn.SetDataRoot("") })
}

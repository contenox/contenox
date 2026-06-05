package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/agent/runtime/modelrepo"
	"github.com/ollama/ollama/api"
)

func TestUnit_OllamaHTTPClient_ListUsesBearerAuthAndNormalizesAPIPath(t *testing.T) {
	t.Parallel()

	var (
		gotPath string
		gotAuth string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"models":[]}`)
	}))
	defer srv.Close()

	client, err := newOllamaHTTPClient(srv.URL+"/api", "test-key", srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	if _, err := client.List(context.Background()); err != nil {
		t.Fatal(err)
	}

	if gotPath != "/api/tags" {
		t.Fatalf("path = %q, want /api/tags", gotPath)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("authorization = %q, want Bearer test-key", gotAuth)
	}
}

func TestUnit_OllamaHTTPClient_GenerateStreamsNDJSON(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		fmt.Fprintln(w, `{"response":"hel","done":false}`)
		fmt.Fprintln(w, `{"response":"lo","done":true,"done_reason":"stop"}`)
	}))
	defer srv.Close()

	client, err := newOllamaHTTPClient(srv.URL, "", srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	var chunks []string
	err = client.Generate(context.Background(), &api.GenerateRequest{Model: "test"}, func(resp api.GenerateResponse) error {
		chunks = append(chunks, resp.Response)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chunks) != 2 || chunks[0] != "hel" || chunks[1] != "lo" {
		t.Fatalf("unexpected streamed chunks: %#v", chunks)
	}
}

func TestUnit_BuildOllamaThink_NormalizesLevels(t *testing.T) {
	cfg := &modelrepo.ChatConfig{}
	if got := buildOllamaThink(cfg); got != nil {
		t.Fatalf("nil think config should omit Ollama think, got %#v", got)
	}

	modelrepo.WithThink("auto").Apply(cfg)
	if got := buildOllamaThink(cfg); got != nil {
		t.Fatalf("auto should omit Ollama think, got %#v", got)
	}

	cfg = &modelrepo.ChatConfig{}
	modelrepo.WithThink("off").Apply(cfg)
	got := buildOllamaThink(cfg)
	if got == nil || got.Value != false {
		t.Fatalf("off = %#v, want bool false", got)
	}

	cfg = &modelrepo.ChatConfig{}
	modelrepo.WithThink("minimal").Apply(cfg)
	got = buildOllamaThink(cfg)
	if got == nil || got.Value != "low" {
		t.Fatalf("minimal = %#v, want low", got)
	}

	cfg = &modelrepo.ChatConfig{}
	modelrepo.WithThink("xhigh").Apply(cfg)
	got = buildOllamaThink(cfg)
	if got == nil || got.Value != "high" {
		t.Fatalf("xhigh = %#v, want high", got)
	}
}

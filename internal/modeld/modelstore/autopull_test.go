package modelstore

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/contenox/contenox/internal/models/modelregistry"
	"github.com/contenox/contenox/internal/transport"
)

type mockRegistry struct {
	models map[string]*modelregistry.ModelDescriptor
}

func (m *mockRegistry) Resolve(ctx context.Context, name string) (*modelregistry.ModelDescriptor, error) {
	if d, ok := m.models[name]; ok {
		return d, nil
	}
	return nil, modelregistry.ErrNotFound
}

func (m *mockRegistry) List(ctx context.Context) ([]modelregistry.ModelDescriptor, error) {
	var out []modelregistry.ModelDescriptor
	for _, v := range m.models {
		out = append(out, *v)
	}
	return out, nil
}

func (m *mockRegistry) OptimalFor(ctx context.Context, name string) (string, error) {
	return "", modelregistry.ErrNotFound
}

func TestUnit_EnsureModelAvailable_AlreadyPresent(t *testing.T) {
	tempDataRoot := t.TempDir()
	modelsDir := filepath.Join(tempDataRoot, "models")
	modelDir := filepath.Join(modelsDir, "test-model")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("dummy weights"), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &mockRegistry{models: map[string]*modelregistry.ModelDescriptor{}}
	err := EnsureModelAvailable(context.Background(), reg, "test-model", AutoPullOptions{
		DataRoot: tempDataRoot,
	})
	if err != nil {
		t.Fatalf("expected nil error for existing model, got: %v", err)
	}
}

func TestUnit_EnsureModelAvailable_AutoPullsFromRegistry(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mock gguf file content"))
	}))
	defer ts.Close()

	tempDataRoot := t.TempDir()
	reg := &mockRegistry{
		models: map[string]*modelregistry.ModelDescriptor{
			"qwen-test": {
				Name:         "qwen-test",
				SourceURL:    ts.URL,
				SizeBytes:    22,
				Curated:      true,
				ToolProtocol: "llama:common_chat_tool_parser",
			},
		},
	}

	var progressBuf bytes.Buffer
	err := EnsureModelAvailable(context.Background(), reg, "qwen-test", AutoPullOptions{
		DataRoot:    tempDataRoot,
		ProgressOut: &progressBuf,
	})
	if err != nil {
		t.Fatalf("expected successful auto-pull, got: %v", err)
	}

	// Verify model.gguf exists
	destPath := filepath.Join(tempDataRoot, "models", "qwen-test", "model.gguf")
	data, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("expected downloaded model.gguf, read error: %v", err)
	}
	if string(data) != "mock gguf file content" {
		t.Fatalf("unexpected content: %q", string(data))
	}

	// Verify profile was written
	profilePath := filepath.Join(tempDataRoot, "models", "qwen-test", "contenox-llama.json")
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("expected profile JSON at %s: %v", profilePath, err)
	}
}

func TestUnit_EnsureModelAvailable_NotFoundInRegistry(t *testing.T) {
	tempDataRoot := t.TempDir()
	reg := &mockRegistry{models: map[string]*modelregistry.ModelDescriptor{}}

	err := EnsureModelAvailable(context.Background(), reg, "nonexistent-model", AutoPullOptions{
		DataRoot: tempDataRoot,
	})
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
	if !errors.Is(err, transport.ErrModelNotFound) && !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("expected ErrModelNotFound, got: %v", err)
	}
}

func TestUnit_EnsureModelAvailable_ConcurrentDeduplication(t *testing.T) {
	var downloadCalls int
	var mu sync.Mutex

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		downloadCalls++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("mock concurrent gguf content"))
	}))
	defer ts.Close()

	tempDataRoot := t.TempDir()
	reg := &mockRegistry{
		models: map[string]*modelregistry.ModelDescriptor{
			"concurrent-model": {
				Name:      "concurrent-model",
				SourceURL: ts.URL,
				SizeBytes: 27,
			},
		},
	}

	var wg sync.WaitGroup
	errs := make([]error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = EnsureModelAvailable(context.Background(), reg, "concurrent-model", AutoPullOptions{
				DataRoot: tempDataRoot,
			})
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d got error: %v", i, err)
		}
	}

	mu.Lock()
	calls := downloadCalls
	mu.Unlock()

	if calls != 1 {
		t.Fatalf("expected exactly 1 download call for 5 concurrent requests, got %d", calls)
	}
}

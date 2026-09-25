package modelstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestUnit_HuggingFaceMirrorAndAuthentication(t *testing.T) {
	var mu sync.Mutex
	paths := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Errorf("missing authentication for %s", r.URL.Path)
		}
		mu.Lock()
		paths[r.URL.Path] = true
		mu.Unlock()
		switch r.URL.Path {
		case "/mirror/api/models/owner/ir":
			fmt.Fprint(w, `{"siblings":[{"rfilename":"openvino_model.xml"},{"rfilename":"weights.bin"}]}`)
		default:
			fmt.Fprint(w, "model-data")
		}
	}))
	defer server.Close()
	t.Setenv("HF_TOKEN", "private-token")
	t.Setenv("HF_ENDPOINT", server.URL+"/mirror")
	dir := t.TempDir()
	if err := DownloadFile(context.Background(), "https://huggingface.co/owner/gguf/resolve/main/model.gguf", filepath.Join(dir, "model.gguf"), io.Discard); err != nil {
		t.Fatal(err)
	}
	if err := downloadOpenVINOIRRepo(context.Background(), "owner/ir", dir, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/mirror/owner/gguf/resolve/main/model.gguf", "/mirror/api/models/owner/ir", "/mirror/owner/ir/resolve/main/openvino_model.xml", "/mirror/owner/ir/resolve/main/weights.bin"} {
		if !paths[path] {
			t.Errorf("missing request %s", path)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "weights.bin"))
	if err != nil || string(b) != "model-data" {
		t.Fatalf("download=%q err=%v", b, err)
	}
}

func TestUnit_HuggingFaceTokenNeverReachesForeignOrigin(t *testing.T) {
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("token leaked to foreign origin")
		}
		fmt.Fprint(w, "weights")
	}))
	defer foreign.Close()
	trusted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer private-token" {
			t.Error("missing trusted token")
		}
		http.Redirect(w, r, foreign.URL+"/weights", http.StatusFound)
	}))
	defer trusted.Close()
	t.Setenv("HF_ENDPOINT", trusted.URL)
	t.Setenv("HF_TOKEN", "private-token")
	for _, source := range []string{"https://huggingface.co/owner/repo/resolve/main/model.gguf", foreign.URL + "/direct"} {
		if err := DownloadFile(context.Background(), source, filepath.Join(t.TempDir(), "model"), io.Discard); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUnit_HuggingFaceAccessErrors(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				fmt.Fprint(w, "sensitive-response private-token")
			}))
			defer server.Close()
			t.Setenv("HF_ENDPOINT", server.URL)
			t.Setenv("HF_TOKEN", "private-token")
			for _, source := range []string{"https://huggingface.co/owner/gated/resolve/main/weights.gguf", "https://huggingface.co/api/models/owner/gated"} {
				_, err := modelDownload(context.Background(), source)
				if err == nil || !strings.Contains(err.Error(), "https://huggingface.co/owner/gated") || !strings.Contains(err.Error(), "HF_TOKEN") {
					t.Fatalf("error=%v", err)
				}
				if strings.Contains(err.Error(), "private-token") || strings.Contains(err.Error(), "sensitive-response") {
					t.Fatalf("sensitive error: %v", err)
				}
			}
		})
	}
}

func TestUnit_HuggingFacePublicDownloadWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("unexpected token")
		}
		fmt.Fprint(w, "public")
	}))
	defer server.Close()
	t.Setenv("HF_ENDPOINT", server.URL)
	t.Setenv("HF_TOKEN", "")
	resp, err := modelDownload(context.Background(), "https://huggingface.co/owner/public/resolve/main/model.gguf")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

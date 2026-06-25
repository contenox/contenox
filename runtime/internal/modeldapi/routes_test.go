package modeldapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/internal/modeldprobe"
	"github.com/contenox/runtime/runtime/transport"
)

type fakeStatusProvider struct {
	detected     modeldprobe.Status
	daemon       transport.DaemonStatus
	statusErr    error
	statusCalled bool
}

func (f *fakeStatusProvider) Detect(context.Context) modeldprobe.Status {
	return f.detected
}

func (f *fakeStatusProvider) Status(context.Context) (transport.DaemonStatus, error) {
	f.statusCalled = true
	return f.daemon, f.statusErr
}

func TestStatusReturnsSanitizedActiveModel(t *testing.T) {
	provider := &fakeStatusProvider{
		detected: modeldprobe.Status{
			State:    modeldprobe.StateRunning,
			Binary:   "/bin/modeld",
			Endpoint: "127.0.0.1:4000",
			Instance: "owner-1",
			Backend:  "llama",
		},
		daemon: transport.DaemonStatus{
			OwnerInstanceID: "owner-1",
			Backend:         "llama",
			State:           transport.SlotReady,
			Active: &transport.ActiveModel{
				ModelName:  "qwen3-8b",
				Type:       "llama",
				Digest:     "sha256:abc",
				Path:       "/private/models/qwen3-8b/model.gguf",
				Config:     transport.Config{NumCtx: 8192, HotContextTokens: 4096},
				Generation: 7,
			},
		},
	}
	mux := http.NewServeMux()
	addRoutesWithProvider(mux, provider)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/status", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "/private/models") {
		t.Fatalf("status response leaked active model path: %s", rr.Body.String())
	}

	var got StatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !provider.statusCalled {
		t.Fatal("expected daemon status to be queried")
	}
	if !got.Available || got.State != "running" || got.Slot == nil || got.Slot.Active == nil {
		t.Fatalf("unexpected status response: %#v", got)
	}
	if got.Slot.Active.ModelName != "qwen3-8b" || got.Slot.Active.Generation != 7 {
		t.Fatalf("unexpected active model: %#v", got.Slot.Active)
	}
	if got.Slot.Active.Config.NumCtx != 8192 || got.Slot.Active.Config.HotContextTokens != 4096 {
		t.Fatalf("unexpected active config: %#v", got.Slot.Active.Config)
	}
}

func TestStatusDoesNotDialWhenModeldIsNotRunning(t *testing.T) {
	provider := &fakeStatusProvider{
		detected: modeldprobe.Status{
			State:  modeldprobe.StateNotRunning,
			Binary: "/bin/modeld",
		},
		statusErr: errors.New("must not be called"),
	}
	mux := http.NewServeMux()
	addRoutesWithProvider(mux, provider)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/status", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if provider.statusCalled {
		t.Fatal("did not expect daemon status to be queried")
	}

	var got StatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Available || got.State != "not-running" || got.Error == "" || got.Slot != nil {
		t.Fatalf("unexpected status response: %#v", got)
	}
}

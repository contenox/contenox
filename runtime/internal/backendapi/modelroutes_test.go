package backendapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/contenox/runtime/runtime/internal/backendapi"
	"github.com/contenox/runtime/runtime/internal/setupcheck"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
)

func TestOpenAIModelsIncludesDefaultAliasAndObservedModels(t *testing.T) {
	mux := http.NewServeMux()
	backendapi.AddModelRoutes(mux, &stubStateService{states: []statetype.BackendRuntimeState{{
		PulledModels: []statetype.ModelPullStatus{{Model: "observed-model"}},
	}}}, "contenox-default")

	req := httptest.NewRequest(http.MethodGet, "/openai/v1/models", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp backendapi.OpenAICompatibleModelList
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	seen := map[string]bool{}
	for _, model := range resp.Data {
		seen[model.ID] = true
		if model.Created == 0 {
			t.Fatalf("expected non-zero created timestamp for %q", model.ID)
		}
	}
	for _, id := range []string{"default", "contenox-default", "observed-model"} {
		if !seen[id] {
			t.Fatalf("expected model list to include %q; got %#v", id, seen)
		}
	}
}

type stubStateService struct {
	states []statetype.BackendRuntimeState
}

func (s *stubStateService) Get(_ context.Context) ([]statetype.BackendRuntimeState, error) {
	return s.states, nil
}
func (s *stubStateService) SetupStatus(_ context.Context) (setupcheck.Result, error) {
	return setupcheck.Result{}, nil
}
func (s *stubStateService) Refresh(_ context.Context) (setupcheck.Result, error) {
	return setupcheck.Result{}, nil
}
func (s *stubStateService) SetCLIConfig(_ context.Context, _ stateservice.CLIConfigPatch) (stateservice.CLIConfigSnapshot, error) {
	return stateservice.CLIConfigSnapshot{}, nil
}

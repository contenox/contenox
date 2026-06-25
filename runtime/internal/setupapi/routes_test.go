package setupapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/internal/setupcheck"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/stretchr/testify/require"
)

type fakeStateService struct {
	refreshCalled bool
	refreshResult setupcheck.Result
	setCalled     bool
	setPatch      stateservice.CLIConfigPatch
	setSnapshot   stateservice.CLIConfigSnapshot
}

func (f *fakeStateService) Get(context.Context) ([]statetype.BackendRuntimeState, error) {
	return nil, nil
}

func (f *fakeStateService) SetupStatus(context.Context) (setupcheck.Result, error) {
	return setupcheck.Result{}, nil
}

func (f *fakeStateService) Refresh(context.Context) (setupcheck.Result, error) {
	f.refreshCalled = true
	return f.refreshResult, nil
}

func (f *fakeStateService) SetCLIConfig(_ context.Context, patch stateservice.CLIConfigPatch) (stateservice.CLIConfigSnapshot, error) {
	f.setCalled = true
	f.setPatch = patch
	return f.setSnapshot, nil
}

func TestRefreshStatusRunsStateRefresh(t *testing.T) {
	svc := &fakeStateService{
		refreshResult: setupcheck.Result{
			DefaultModel:          "gemma4-e4b",
			DefaultProvider:       "local",
			BackendCount:          1,
			ReachableBackendCount: 1,
		},
	}
	mux := http.NewServeMux()
	AddSetupRoutes(mux, svc, nil)

	req := httptest.NewRequest(http.MethodPost, "/setup/refresh", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, svc.refreshCalled)

	var got setupcheck.Result
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	require.Equal(t, "gemma4-e4b", got.DefaultModel)
	require.Equal(t, 1, got.ReachableBackendCount)
}

func TestPutCLIConfigAllowsExplicitEmptyWorkspaceValues(t *testing.T) {
	svc := &fakeStateService{
		setSnapshot: stateservice.CLIConfigSnapshot{
			DefaultChain:   "default-chain.json",
			HITLPolicyName: "hitl-policy-default.json",
			ResolvedFrom: map[string]string{
				"defaultChain":   "global",
				"hitlPolicyName": "global",
			},
		},
	}
	mux := http.NewServeMux()
	AddSetupRoutes(mux, svc, nil)

	req := httptest.NewRequest(http.MethodPut, "/cli-config", strings.NewReader(`{"default-chain":"","hitl-policy-name":"","default-autocomplete-provider":"mistral","default-autocomplete-model":"codestral-latest","default-max-tokens":"8192"}`))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, svc.setCalled)
	require.NotNil(t, svc.setPatch.DefaultChain)
	require.NotNil(t, svc.setPatch.HITLPolicyName)
	require.NotNil(t, svc.setPatch.DefaultAutocompleteProvider)
	require.NotNil(t, svc.setPatch.DefaultAutocompleteModel)
	require.NotNil(t, svc.setPatch.DefaultMaxTokens)
	require.Equal(t, "", *svc.setPatch.DefaultChain)
	require.Equal(t, "", *svc.setPatch.HITLPolicyName)
	require.Equal(t, "mistral", *svc.setPatch.DefaultAutocompleteProvider)
	require.Equal(t, "codestral-latest", *svc.setPatch.DefaultAutocompleteModel)
	require.Equal(t, "8192", *svc.setPatch.DefaultMaxTokens)

	var got putCLIConfigResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&got))
	require.Equal(t, "default-chain.json", got.DefaultChain)
	require.Equal(t, "global", got.ResolvedFrom["defaultChain"])
}

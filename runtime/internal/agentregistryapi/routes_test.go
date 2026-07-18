package agentregistryapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/agentregistryservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

func setupAgentRegistryAPI(t *testing.T) (context.Context, agentregistryservice.Service, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "agentregistryapi.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	svc := agentregistryservice.New(db)

	mux := http.NewServeMux()
	AddAgentRegistryRoutes(mux, svc)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ctx, svc, srv
}

func newExternalACPAgent(t *testing.T, name string) *runtimetypes.Agent {
	t.Helper()
	agent := &runtimetypes.Agent{Name: name, Enabled: true}
	require.NoError(t, agent.SetExternalACPConfig(runtimetypes.ExternalACPConfig{
		Transport: runtimetypes.ExternalACPTransportStdio,
		Command:   "my-acp-agent",
	}))
	return agent
}

func TestIntegration_AgentRegistryAPI_ListReturnsRegisteredAgents(t *testing.T) {
	ctx, svc, srv := setupAgentRegistryAPI(t)

	for _, name := range []string{"list-a", "list-b", "list-c"} {
		require.NoError(t, svc.Create(ctx, newExternalACPAgent(t, name)))
	}

	resp, err := http.Get(srv.URL + "/agents")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var agents []*runtimetypes.Agent
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&agents))
	require.Len(t, agents, 3)

	names := map[string]bool{}
	for _, a := range agents {
		names[a.Name] = true
		require.Equal(t, runtimetypes.AgentKindExternalACP, a.Kind)
	}
	require.True(t, names["list-a"] && names["list-b"] && names["list-c"])
}

func TestIntegration_AgentRegistryAPI_GetByName(t *testing.T) {
	ctx, svc, srv := setupAgentRegistryAPI(t)

	agent := newExternalACPAgent(t, "smoke-bot")
	require.NoError(t, svc.Create(ctx, agent))

	resp, err := http.Get(srv.URL + "/agents/by-name/smoke-bot")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got runtimetypes.Agent
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, agent.ID, got.ID)
	require.Equal(t, "smoke-bot", got.Name)
	require.Equal(t, runtimetypes.AgentKindExternalACP, got.Kind)
}

func TestIntegration_AgentRegistryAPI_GetByNameUnknownReturns404(t *testing.T) {
	_, _, srv := setupAgentRegistryAPI(t)

	resp, err := http.Get(srv.URL + "/agents/by-name/does-not-exist")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestIntegration_AgentRegistryAPI_GetByID(t *testing.T) {
	ctx, svc, srv := setupAgentRegistryAPI(t)

	agent := newExternalACPAgent(t, "by-id")
	require.NoError(t, svc.Create(ctx, agent))

	resp, err := http.Get(srv.URL + "/agents/" + agent.ID)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got runtimetypes.Agent
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, agent.ID, got.ID)
}

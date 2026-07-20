package missionapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

func setupMissionAPI(t *testing.T) (context.Context, missionservice.Service, *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	db, err := libdb.NewSQLiteDBManager(ctx, filepath.Join(t.TempDir(), "missionapi.db"), runtimetypes.SchemaSQLite)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	svc := missionservice.New(db)

	mux := http.NewServeMux()
	AddRoutes(mux, svc)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return ctx, svc, srv
}

func postJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	return resp
}

func patchJSON(t *testing.T, url string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPatch, url, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestIntegration_MissionAPI_CreateThenGet(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	resp := postJSON(t, srv.URL+"/missions", missionservice.Mission{Intent: "ship the board", AgentName: "runner"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created missionservice.Mission
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NotEmpty(t, created.ID)
	require.Equal(t, missionservice.StatusOpen, created.Status)
	require.Equal(t, "ship the board", created.Intent)

	getResp, err := http.Get(srv.URL + "/missions/" + created.ID)
	require.NoError(t, err)
	defer getResp.Body.Close()
	require.Equal(t, http.StatusOK, getResp.StatusCode)

	var got missionservice.Mission
	require.NoError(t, json.NewDecoder(getResp.Body).Decode(&got))
	require.Equal(t, created.ID, got.ID)
}

func TestIntegration_MissionAPI_CreateValidationFailure(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	resp := postJSON(t, srv.URL+"/missions", missionservice.Mission{Intent: ""})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestIntegration_MissionAPI_ListReturnsMissions(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	for _, intent := range []string{"m-a", "m-b", "m-c"} {
		require.NoError(t, svc.Create(ctx, &missionservice.Mission{Intent: intent}))
	}

	resp, err := http.Get(srv.URL + "/missions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var missions []*missionservice.Mission
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&missions))
	require.Len(t, missions, 3)
}

func TestIntegration_MissionAPI_ListEmptyIsJSONArray(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	resp, err := http.Get(srv.URL + "/missions")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "[]", strings.TrimSpace(string(body)))
}

func TestIntegration_MissionAPI_GetUnknownReturns404(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	resp, err := http.Get(srv.URL + "/missions/no-such-mission")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestIntegration_MissionAPI_PatchIntentAndStatus(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "original"}
	require.NoError(t, svc.Create(ctx, m))

	newIntent := "reworded intent"
	newStatus := string(missionservice.StatusLanded)
	resp := patchJSON(t, srv.URL+"/missions/"+m.ID, MissionPatch{Intent: &newIntent, Status: &newStatus})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got missionservice.Mission
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "reworded intent", got.Intent)
	require.Equal(t, missionservice.StatusLanded, got.Status)
}

func TestIntegration_MissionAPI_PatchBind(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "bind via patch"}
	require.NoError(t, svc.Create(ctx, m))

	resp := patchJSON(t, srv.URL+"/missions/"+m.ID, MissionPatch{SessionID: "session-1", InstanceID: "instance-1"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got missionservice.Mission
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, []string{"session-1"}, got.SessionIDs)
	require.Equal(t, []string{"instance-1"}, got.InstanceIDs)
}

func TestIntegration_MissionAPI_PatchInvalidStatusIsUnprocessable(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "status guard"}
	require.NoError(t, svc.Create(ctx, m))

	bad := "bogus"
	resp := patchJSON(t, srv.URL+"/missions/"+m.ID, MissionPatch{Status: &bad})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestIntegration_MissionAPI_PatchUnknownReturns404(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	intent := "ghost"
	resp := patchJSON(t, srv.URL+"/missions/no-such-mission", MissionPatch{Intent: &intent})
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

func TestIntegration_MissionAPI_Delete(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "to be deleted"}
	require.NoError(t, svc.Create(ctx, m))

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/missions/"+m.ID, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	_, err = svc.Get(ctx, m.ID)
	require.Error(t, err)
}

func TestIntegration_MissionAPI_DeleteUnknownReturns404(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	req, err := http.NewRequest(http.MethodDelete, srv.URL+"/missions/no-such-mission", nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

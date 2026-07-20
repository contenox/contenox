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
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/missionservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/require"
)

const testPolicy = "hitl-policy-default.json"

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

	resp := postJSON(t, srv.URL+"/missions", missionservice.Mission{Intent: "ship the board", AgentName: "runner", HITLPolicyName: testPolicy})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created missionservice.Mission
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NotEmpty(t, created.ID)
	require.Equal(t, missionservice.StatusOpen, created.Status)
	require.Equal(t, "ship the board", created.Intent)
	require.Equal(t, testPolicy, created.HITLPolicyName)

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

	resp := postJSON(t, srv.URL+"/missions", missionservice.Mission{Intent: "", HITLPolicyName: testPolicy})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// A mission without an envelope has no bounds, which mission mode must not
// permit: POST /missions rejects a missing HITLPolicyName.
func TestIntegration_MissionAPI_CreateMissingEnvelopeFailure(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	resp := postJSON(t, srv.URL+"/missions", missionservice.Mission{Intent: "no envelope"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestIntegration_MissionAPI_ListReturnsMissions(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	for _, intent := range []string{"m-a", "m-b", "m-c"} {
		require.NoError(t, svc.Create(ctx, &missionservice.Mission{Intent: intent, HITLPolicyName: testPolicy}))
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

	m := &missionservice.Mission{Intent: "original", HITLPolicyName: testPolicy}
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

	m := &missionservice.Mission{Intent: "bind via patch", HITLPolicyName: testPolicy}
	require.NoError(t, svc.Create(ctx, m))

	resp := patchJSON(t, srv.URL+"/missions/"+m.ID, MissionPatch{SessionID: "session-1", InstanceID: "instance-1"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got missionservice.Mission
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "session-1", got.SessionID)
	require.Equal(t, "instance-1", got.InstanceID)
}

// Rebinding a different session id over one already bound is a conflict, not
// an append: this is mission mode's one-mission-one-unit invariant enforced
// over HTTP.
func TestIntegration_MissionAPI_PatchBindConflictIsConflict(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "conflicting bind via patch", HITLPolicyName: testPolicy}
	require.NoError(t, svc.Create(ctx, m))

	first := patchJSON(t, srv.URL+"/missions/"+m.ID, MissionPatch{SessionID: "session-1"})
	first.Body.Close()
	require.Equal(t, http.StatusOK, first.StatusCode)

	second := patchJSON(t, srv.URL+"/missions/"+m.ID, MissionPatch{SessionID: "session-2"})
	defer second.Body.Close()
	require.Equal(t, http.StatusConflict, second.StatusCode)
}

func TestIntegration_MissionAPI_PatchInvalidStatusIsUnprocessable(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "status guard", HITLPolicyName: testPolicy}
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

	m := &missionservice.Mission{Intent: "to be deleted", HITLPolicyName: testPolicy}
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

// ─── Reports ────────────────────────────────────────────────────────────────

func TestIntegration_MissionAPI_AddReportThenList(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "reported mission", HITLPolicyName: testPolicy}
	require.NoError(t, svc.Create(ctx, m))

	resp := postJSON(t, srv.URL+"/missions/"+m.ID+"/reports", missionservice.Report{
		Kind:    missionservice.ReportKindProgress,
		Summary: "halfway there",
		Detail:  "long-form notes",
		Refs:    []string{"/tmp/output.log"},
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created missionservice.Report
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	require.NotEmpty(t, created.ID)
	require.Equal(t, m.ID, created.MissionID)
	require.Equal(t, missionservice.ReportKindProgress, created.Kind)
	require.Equal(t, "halfway there", created.Summary)
	require.False(t, created.CreatedAt.IsZero())

	listResp, err := http.Get(srv.URL + "/missions/" + m.ID + "/reports")
	require.NoError(t, err)
	defer listResp.Body.Close()
	require.Equal(t, http.StatusOK, listResp.StatusCode)

	var reports []*missionservice.Report
	require.NoError(t, json.NewDecoder(listResp.Body).Decode(&reports))
	require.Len(t, reports, 1)
	require.Equal(t, "halfway there", reports[0].Summary)
}

func TestIntegration_MissionAPI_ListReportsEmptyIsJSONArray(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "no reports yet", HITLPolicyName: testPolicy}
	require.NoError(t, svc.Create(ctx, m))

	resp, err := http.Get(srv.URL + "/missions/" + m.ID + "/reports")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "[]", strings.TrimSpace(string(body)))
}

func TestIntegration_MissionAPI_ListReportsNewestFirst(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "multi-report mission", HITLPolicyName: testPolicy}
	require.NoError(t, svc.Create(ctx, m))

	for _, summary := range []string{"first", "second", "third"} {
		resp := postJSON(t, srv.URL+"/missions/"+m.ID+"/reports", missionservice.Report{
			Kind:    missionservice.ReportKindProgress,
			Summary: summary,
		})
		resp.Body.Close()
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		time.Sleep(1 * time.Millisecond) // force distinct createdAt for a stable newest-first order
	}

	resp, err := http.Get(srv.URL + "/missions/" + m.ID + "/reports")
	require.NoError(t, err)
	defer resp.Body.Close()

	var reports []*missionservice.Report
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reports))
	require.Len(t, reports, 3)
	require.Equal(t, "third", reports[0].Summary)
	require.Equal(t, "second", reports[1].Summary)
	require.Equal(t, "first", reports[2].Summary)
}

func TestIntegration_MissionAPI_AddReportInvalidKindIsUnprocessable(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "bad kind mission", HITLPolicyName: testPolicy}
	require.NoError(t, svc.Create(ctx, m))

	resp := postJSON(t, srv.URL+"/missions/"+m.ID+"/reports", missionservice.Report{Kind: "bogus", Summary: "ok"})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestIntegration_MissionAPI_AddReportMultiLineSummaryIsUnprocessable(t *testing.T) {
	ctx, svc, srv := setupMissionAPI(t)

	m := &missionservice.Mission{Intent: "multi-line summary mission", HITLPolicyName: testPolicy}
	require.NoError(t, svc.Create(ctx, m))

	resp := postJSON(t, srv.URL+"/missions/"+m.ID+"/reports", missionservice.Report{
		Kind:    missionservice.ReportKindProgress,
		Summary: "line one\nline two",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestIntegration_MissionAPI_AddReportUnknownMissionReturns404(t *testing.T) {
	_, _, srv := setupMissionAPI(t)

	resp := postJSON(t, srv.URL+"/missions/no-such-mission/reports", missionservice.Report{
		Kind:    missionservice.ReportKindProgress,
		Summary: "orphan report",
	})
	defer resp.Body.Close()
	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

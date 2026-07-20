package approvalapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

// fakeApprovals is a hand-rolled hitlservice.Service double. These tests
// prove only ROUTING, status-code mapping, and DTO (de)serialization —
// exactly like runtime/internal/fleetapi/routes_test.go's fakeFleet — the
// service behavior itself (durability, the CAS, the wake-up channel) is
// runtime/hitlservice's own responsibility and is covered by its tests.
type fakeApprovals struct {
	mu sync.Mutex

	listItems []*runtimetypes.HITLApproval
	listErr   error
	listLimit int

	respondErr   error
	respondCalls []respondArgs
}

type respondArgs struct {
	id       string
	approved bool
}

func (f *fakeApprovals) Evaluate(context.Context, string, string, map[string]any) (hitlservice.EvaluationResult, error) {
	return hitlservice.EvaluationResult{}, nil
}

func (f *fakeApprovals) RequestApproval(context.Context, hitlservice.ApprovalRequest, taskengine.TaskEventSink) (bool, error) {
	return false, nil
}

func (f *fakeApprovals) SweepExpired(context.Context) (int, error) {
	return 0, nil
}

func (f *fakeApprovals) Respond(_ context.Context, id string, approved bool) error {
	f.mu.Lock()
	f.respondCalls = append(f.respondCalls, respondArgs{id: id, approved: approved})
	f.mu.Unlock()
	return f.respondErr
}

func (f *fakeApprovals) ListPending(_ context.Context, limit int) ([]*runtimetypes.HITLApproval, error) {
	f.mu.Lock()
	f.listLimit = limit
	f.mu.Unlock()
	if f.listErr != nil {
		return nil, f.listErr
	}
	if f.listItems == nil {
		return []*runtimetypes.HITLApproval{}, nil
	}
	return f.listItems, nil
}

func (f *fakeApprovals) responds() []respondArgs {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]respondArgs(nil), f.respondCalls...)
}

var _ hitlservice.Service = (*fakeApprovals)(nil)

func setupApprovalAPI(t *testing.T, svc hitlservice.Service) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	AddRoutes(mux, svc)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func mkRow(id string) *runtimetypes.HITLApproval {
	now := time.Now().UTC()
	rule := 2
	diff := "--- a\n+++ b\n"
	return &runtimetypes.HITLApproval{
		ID:          id,
		ToolsName:   "local_fs",
		ToolName:    "write_file",
		ArgsSummary: "/workspace/main.go",
		Diff:        &diff,
		PolicyName:  "hitl-policy-default.json",
		MatchedRule: &rule,
		OnTimeout:   "deny",
		State:       runtimetypes.HITLApprovalPending,
		CreatedAt:   now,
		ExpiresAt:   now.Add(time.Hour),
	}
}

// ─── List ────────────────────────────────────────────────────────────────

func TestUnit_ApprovalAPI_ListReturnsPending(t *testing.T) {
	srv := setupApprovalAPI(t, &fakeApprovals{listItems: []*runtimetypes.HITLApproval{mkRow("a-1"), mkRow("a-2")}})

	resp, err := http.Get(srv.URL + "/approvals")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got []*runtimetypes.HITLApproval
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Len(t, got, 2)
	require.Equal(t, "a-1", got[0].ID)
	require.Equal(t, "local_fs", got[0].ToolsName)
	require.Equal(t, "write_file", got[0].ToolName)
	require.Equal(t, "/workspace/main.go", got[0].ArgsSummary)
	require.NotNil(t, got[0].Diff)
	require.Equal(t, "hitl-policy-default.json", got[0].PolicyName)
	require.NotNil(t, got[0].MatchedRule)
	require.Equal(t, 2, *got[0].MatchedRule)
}

func TestUnit_ApprovalAPI_ListEmptyIsJSONArray(t *testing.T) {
	srv := setupApprovalAPI(t, &fakeApprovals{})

	resp, err := http.Get(srv.URL + "/approvals")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, "[]", strings.TrimSpace(string(body)))
}

func TestUnit_ApprovalAPI_ListForwardsLimitParam(t *testing.T) {
	f := &fakeApprovals{}
	srv := setupApprovalAPI(t, f)

	resp, err := http.Get(srv.URL + "/approvals?limit=7")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 7, f.listLimit)
}

func TestUnit_ApprovalAPI_ListDefaultLimitIs100(t *testing.T) {
	f := &fakeApprovals{}
	srv := setupApprovalAPI(t, f)

	resp, err := http.Get(srv.URL + "/approvals")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Equal(t, 100, f.listLimit)
}

// TestUnit_ApprovalAPI_ListInvalidLimitIsRejected pins that a non-numeric
// limit is rejected rather than silently ignored, with 400 — a malformed
// query parameter, not a missing resource.
//
// This used to assert 404, and the 404 was not a decision: the handler wrapped
// the parse error in a bare fmt.Errorf and passed apiframework.ListOperation,
// so apiframework.mapErrorToStatus fell through to its ListOperation default,
// which is 404. Parsing now goes through apiframework.LimitParam, whose
// classified ErrInvalidParameterValue maps to 400 whatever Operation the
// handler passes.
func TestUnit_ApprovalAPI_ListInvalidLimitIsRejected(t *testing.T) {
	srv := setupApprovalAPI(t, &fakeApprovals{})

	resp, err := http.Get(srv.URL + "/approvals?limit=not-a-number")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestUnit_ApprovalAPI_ListNonPositiveLimitIsRejected pins the tightening that
// came with the shared parser: an explicit limit below 1 is now refused rather
// than handed to hitlservice, which read it as "apply my own default". An
// *absent* limit still means the handler's own default (100) — see
// TestUnit_ApprovalAPI_ListDefaultLimitIs100 — so only an explicitly
// nonsensical value is rejected.
func TestUnit_ApprovalAPI_ListNonPositiveLimitIsRejected(t *testing.T) {
	for _, limit := range []string{"0", "-1"} {
		f := &fakeApprovals{}
		srv := setupApprovalAPI(t, f)

		resp, err := http.Get(srv.URL + "/approvals?limit=" + limit)
		require.NoError(t, err)
		require.NoError(t, resp.Body.Close())
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, "limit=%s", limit)
		require.Zero(t, f.listLimit, "limit=%s must not reach the service", limit)
	}
}

// TestUnit_ApprovalAPI_ListServiceErrorPropagatesStatus proves the handler
// does no error-mapping of its own beyond apiframework.Error: whatever
// status the service's error carries (via the shared error taxonomy) is
// what the wire gets — mirrors
// fleetapi_test.go's TestUnit_FleetAPI_DispatchServiceErrorMapsToStatus.
func TestUnit_ApprovalAPI_ListServiceErrorPropagatesStatus(t *testing.T) {
	srv := setupApprovalAPI(t, &fakeApprovals{listErr: apiframework.InternalServerError("boom")})

	resp, err := http.Get(srv.URL + "/approvals")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

// ─── Answer ──────────────────────────────────────────────────────────────

func answerPost(t *testing.T, url, id string, approved bool) *http.Response {
	t.Helper()
	raw, err := json.Marshal(AnswerRequest{Approved: approved})
	require.NoError(t, err)
	resp, err := http.Post(url+"/approvals/"+id, "application/json", bytes.NewReader(raw))
	require.NoError(t, err)
	return resp
}

func TestUnit_ApprovalAPI_AnswerApprovedCallsRespondAndReturns200(t *testing.T) {
	f := &fakeApprovals{}
	srv := setupApprovalAPI(t, f)

	resp := answerPost(t, srv.URL, "appr-1", true)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "approved", got)

	require.Equal(t, []respondArgs{{id: "appr-1", approved: true}}, f.responds())
}

func TestUnit_ApprovalAPI_AnswerDeniedCallsRespondAndReturns200(t *testing.T) {
	f := &fakeApprovals{}
	srv := setupApprovalAPI(t, f)

	resp := answerPost(t, srv.URL, "appr-2", false)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&got))
	require.Equal(t, "denied", got)

	require.Equal(t, []respondArgs{{id: "appr-2", approved: false}}, f.responds())
}

func TestUnit_ApprovalAPI_AnswerMalformedBodyIsUnprocessable(t *testing.T) {
	srv := setupApprovalAPI(t, &fakeApprovals{})

	resp, err := http.Post(srv.URL+"/approvals/appr-1", "application/json", strings.NewReader("{not json"))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

// TestUnit_ApprovalAPI_AnswerErrorMapping is the honest-mapping requirement:
// not-found is 404; already-resolved and expired are both 409 but must NOT
// collapse into the same message — an operator needs to know which
// happened.
func TestUnit_ApprovalAPI_AnswerErrorMapping(t *testing.T) {
	cases := []struct {
		name        string
		err         error
		wantStatus  int
		wantMessage string
	}{
		{"unknown id -> 404", hitlservice.ErrApprovalNotFound, http.StatusNotFound, hitlservice.ErrApprovalNotFound.Error()},
		{"already resolved -> 409", hitlservice.ErrApprovalAlreadyResolved, http.StatusConflict, hitlservice.ErrApprovalAlreadyResolved.Error()},
		{"expired -> 409", hitlservice.ErrApprovalExpired, http.StatusConflict, hitlservice.ErrApprovalExpired.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := setupApprovalAPI(t, &fakeApprovals{respondErr: tc.err})

			resp := answerPost(t, srv.URL, "appr-1", true)
			defer resp.Body.Close()
			require.Equal(t, tc.wantStatus, resp.StatusCode)

			var body struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			require.Equal(t, tc.wantMessage, body.Error.Message)
		})
	}
}

// TestUnit_ApprovalAPI_AnswerAlreadyResolvedAndExpiredAreDistinguishable
// pins the "do not collapse them into one" requirement directly: both status
// codes are 409, but the two messages must differ from each other.
func TestUnit_ApprovalAPI_AnswerAlreadyResolvedAndExpiredAreDistinguishable(t *testing.T) {
	alreadyResolved := setupApprovalAPI(t, &fakeApprovals{respondErr: hitlservice.ErrApprovalAlreadyResolved})
	expired := setupApprovalAPI(t, &fakeApprovals{respondErr: hitlservice.ErrApprovalExpired})

	r1 := answerPost(t, alreadyResolved.URL, "appr-1", true)
	defer r1.Body.Close()
	r2 := answerPost(t, expired.URL, "appr-1", true)
	defer r2.Body.Close()

	require.Equal(t, http.StatusConflict, r1.StatusCode)
	require.Equal(t, http.StatusConflict, r2.StatusCode)

	b1, err := io.ReadAll(r1.Body)
	require.NoError(t, err)
	b2, err := io.ReadAll(r2.Body)
	require.NoError(t, err)
	require.NotEqual(t, string(b1), string(b2), "an already-resolved ask and an expired one must not read as the same conflict")
}

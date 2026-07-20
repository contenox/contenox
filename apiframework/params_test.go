package apiframework

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestUnit_ListParams_BehaviorTable pins the full absent/valid/invalid matrix
// for both pagination parameters. The distinction that matters is absent vs.
// invalid: an absent limit must keep meaning "use the call site's default"
// (several stores read a non-positive limit as "apply my own default"), while
// an explicitly invalid one must be refused.
func TestUnit_ListParams_BehaviorTable(t *testing.T) {
	const defaultLimit = 100
	const validCursor = "2024-03-01T12:00:00.123456789Z"

	for _, tc := range []struct {
		name       string
		query      string
		wantCursor *time.Time
		wantLimit  int
		wantErr    bool
		wantParam  string
	}{
		{
			name:      "absent cursor and absent limit",
			query:     "",
			wantLimit: defaultLimit,
		},
		{
			name:      "empty cursor and empty limit are absent, not invalid",
			query:     "cursor=&limit=",
			wantLimit: defaultLimit,
		},
		{
			name:       "valid cursor",
			query:      "cursor=" + validCursor,
			wantCursor: mustParseCursor(t, validCursor),
			wantLimit:  defaultLimit,
		},
		{
			name:      "malformed cursor",
			query:     "cursor=garbage",
			wantErr:   true,
			wantParam: "cursor",
		},
		{
			name:      "valid limit",
			query:     "limit=7",
			wantLimit: 7,
		},
		{
			name:      "limit of 1 is the smallest accepted value",
			query:     "limit=1",
			wantLimit: 1,
		},
		{
			name:      "malformed limit",
			query:     "limit=not-a-number",
			wantErr:   true,
			wantParam: "limit",
		},
		{
			name:      "limit of zero is refused",
			query:     "limit=0",
			wantErr:   true,
			wantParam: "limit",
		},
		{
			name:      "negative limit is refused",
			query:     "limit=-5",
			wantErr:   true,
			wantParam: "limit",
		},
		{
			name:       "valid cursor and valid limit together",
			query:      "cursor=" + validCursor + "&limit=3",
			wantCursor: mustParseCursor(t, validCursor),
			wantLimit:  3,
		},
		{
			name:      "cursor is reported first when both are malformed",
			query:     "cursor=garbage&limit=nonsense",
			wantErr:   true,
			wantParam: "cursor",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/things?"+tc.query, nil)

			cursor, limit, err := ListParams(r, defaultLimit)

			if tc.wantErr {
				require.Error(t, err)
				require.True(t, errors.Is(err, ErrInvalidParameterValue),
					"error %v must wrap ErrInvalidParameterValue so it maps to a status", err)
				require.Equal(t, tc.wantParam, GetErrorParam(err))
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantLimit, limit)
			if tc.wantCursor == nil {
				require.Nil(t, cursor)
				return
			}
			require.NotNil(t, cursor)
			require.True(t, cursor.Equal(*tc.wantCursor), "cursor = %v, want %v", *cursor, *tc.wantCursor)
		})
	}
}

// TestUnit_ListParams_InvalidInputMapsTo400 is the whole point of routing the
// parse errors through a classified framework error: mapErrorToStatus's
// fallback for an unclassified error is 404 under ListOperation, which reads
// as "no such collection" for what is really a malformed parameter, and 422
// under CreateOperation. Classified, it is 400 for every Operation.
func TestUnit_ListParams_InvalidInputMapsTo400(t *testing.T) {
	for _, query := range []string{"cursor=garbage", "limit=not-a-number", "limit=0"} {
		t.Run(query, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/things?"+query, nil)
			_, _, err := ListParams(r, 100)
			require.Error(t, err)
			for _, op := range []Operation{ListOperation, GetOperation, DeleteOperation, CreateOperation} {
				require.Equal(t, http.StatusBadRequest, mapErrorToStatus(op, err),
					"operation %d must not change the status of a malformed parameter", op)
			}
		})
	}
}

// TestUnit_ListParams_ErrorBodyNamesTheParameter proves the classified error
// survives into the wire payload with its parameter name and code intact, so
// a client can tell which of the two parameters it got wrong.
func TestUnit_ListParams_ErrorBodyNamesTheParameter(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/things?cursor=garbage", nil)
	_, _, err := ListParams(r, 100)
	require.Error(t, err)

	rec := httptest.NewRecorder()
	require.NoError(t, Error(rec, r, err, ListOperation))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":{
		"message":"invalid cursor format, expected an RFC3339Nano timestamp",
		"type":"invalid_request_error",
		"param":"cursor",
		"code":"invalid_parameter_value"
	}}`, rec.Body.String())
}

// TestUnit_LimitParam_AbsentKeepsCallSiteDefault pins that the default is the
// call site's to choose, including a non-positive one that a store reads as
// "apply my own default" — LimitParam must not second-guess it.
func TestUnit_LimitParam_AbsentKeepsCallSiteDefault(t *testing.T) {
	for _, defaultLimit := range []int{0, -1, 25, 100} {
		r := httptest.NewRequest(http.MethodGet, "/things", nil)
		got, err := LimitParam(r, defaultLimit)
		require.NoError(t, err)
		require.Equal(t, defaultLimit, got)
	}
}

// TestUnit_LimitParam_IgnoresCursor pins that the limit-only endpoints keep
// ignoring a stray cursor rather than newly rejecting one they never accepted.
func TestUnit_LimitParam_IgnoresCursor(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/things?cursor=garbage&limit=4", nil)
	got, err := LimitParam(r, 100)
	require.NoError(t, err)
	require.Equal(t, 4, got)
}

func mustParseCursor(t *testing.T, raw string) *time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	require.NoError(t, err)
	return &parsed
}

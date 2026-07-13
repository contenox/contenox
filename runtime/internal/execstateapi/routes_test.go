package execstateapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libkvstore"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

func testDB(t *testing.T) libdb.DBManager {
	t.Helper()
	db, err := libdb.NewSQLiteDBManager(context.Background(), filepath.Join(t.TempDir(), "state.db"), libkvstore.SQLiteSchema)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestUnit_ExecutionState_ReturnsCapturedUnits(t *testing.T) {
	const reqID = "req-execstate-1"
	db := testDB(t)

	// Seed exactly like the engine does: KVInspector persists sanitized steps
	// under the request ID carried by the context.
	seedCtx := context.WithValue(context.Background(), libtracker.ContextKeyRequestID, reqID)
	inspector := taskengine.NewKVInspector(taskengine.NewSimpleInspector(), libkvstore.NewSQLiteManager(db), libtracker.NoopTracker{})
	stack := inspector.Start(seedCtx)
	stack.RecordStep(taskengine.CapturedStateUnit{
		TaskID:      "step-1",
		TaskHandler: taskengine.HandleNoop.String(),
		InputType:   taskengine.DataTypeString,
		OutputType:  taskengine.DataTypeString,
		Input:       "in",
		Output:      "out",
	})

	mux := http.NewServeMux()
	AddRoutes(mux, db, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/execution-state?requestId="+reqID, nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		RequestID string                         `json:"requestId"`
		State     []taskengine.CapturedStateUnit `json:"state"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Equal(t, reqID, body.RequestID)
	require.Len(t, body.State, 1)
	require.Equal(t, "step-1", body.State[0].TaskID)
}

func TestUnit_ExecutionState_UnknownRequestIsEmptyNotError(t *testing.T) {
	mux := http.NewServeMux()
	AddRoutes(mux, testDB(t), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/execution-state?requestId=never-ran", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		State []taskengine.CapturedStateUnit `json:"state"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.NotNil(t, body.State)
	require.Empty(t, body.State)
}

func TestUnit_ExecutionEvents_ReplaysJournal(t *testing.T) {
	const reqID = "req-events-1"
	db := testDB(t)
	kv := libkvstore.NewSQLiteManager(db)
	sink := taskengine.NewKVJournalTaskEventSink(nil, kv, libtracker.NoopTracker{})
	require.NoError(t, sink.PublishTaskEvent(context.Background(), taskengine.TaskEvent{
		Kind: taskengine.TaskEventChainStarted, RequestID: reqID, ChainID: "c",
	}))
	require.NoError(t, sink.PublishTaskEvent(context.Background(), taskengine.TaskEvent{
		Kind: taskengine.TaskEventToolCall, RequestID: reqID,
		ToolName: "local_fs.write_file", ToolDiffPath: "a.txt", ToolDiffNewText: "hi",
	}))

	mux := http.NewServeMux()
	AddRoutes(mux, db, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/execution-events?requestId="+reqID, nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		RequestID string                 `json:"requestId"`
		Events    []taskengine.TaskEvent `json:"events"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Len(t, body.Events, 2)
	require.Equal(t, taskengine.TaskEventChainStarted, body.Events[0].Kind)
	require.Equal(t, "a.txt", body.Events[1].ToolDiffPath)
}

func TestUnit_ExecutionEvents_UnknownRequestIsEmptyNotError(t *testing.T) {
	mux := http.NewServeMux()
	AddRoutes(mux, testDB(t), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/execution-events?requestId=never-ran", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Events []taskengine.TaskEvent `json:"events"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.NotNil(t, body.Events)
	require.Empty(t, body.Events)
}

func TestUnit_ExecutionState_MissingRequestIDIsBadRequest(t *testing.T) {
	mux := http.NewServeMux()
	AddRoutes(mux, testDB(t), nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/execution-state", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)

	require.Equal(t, http.StatusBadRequest, res.Code)
}

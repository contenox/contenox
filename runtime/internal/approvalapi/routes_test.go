package approvalapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

type nopKVReader struct{}

func (nopKVReader) GetKV(context.Context, string, interface{}) error {
	return nil
}

type captureSink struct {
	events chan taskengine.TaskEvent
}

func (s *captureSink) PublishTaskEvent(_ context.Context, event taskengine.TaskEvent) error {
	s.events <- event
	return nil
}

func (s *captureSink) Enabled() bool {
	return true
}

func TestRespondApprovesPendingRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	svc := hitlservice.New(
		hitlservice.NewFSPolicySource(t.TempDir()),
		"tenant",
		nopKVReader{},
		libtracker.NoopTracker{},
	)
	sink := &captureSink{events: make(chan taskengine.TaskEvent, 1)}
	resultCh := make(chan bool, 1)
	errCh := make(chan error, 1)

	go func() {
		approved, err := svc.RequestApproval(ctx, hitlservice.ApprovalRequest{
			ToolsName: "local_fs",
			ToolName:  "write_file",
			Args:      map[string]any{"path": "README.md"},
		}, sink)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- approved
	}()

	var event taskengine.TaskEvent
	select {
	case event = <-sink.events:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Equal(t, taskengine.TaskEventApprovalRequested, event.Kind)
	require.NotEmpty(t, event.ApprovalID)

	mux := http.NewServeMux()
	AddRoutes(mux, svc, nil)

	req := httptest.NewRequest(http.MethodPost, "/approvals/"+event.ApprovalID, strings.NewReader(`{"approved":true}`))
	req.Header.Set("Content-Type", "application/json")
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	require.Equal(t, http.StatusNoContent, res.Code)

	select {
	case approved := <-resultCh:
		require.True(t, approved)
	case err := <-errCh:
		require.NoError(t, err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

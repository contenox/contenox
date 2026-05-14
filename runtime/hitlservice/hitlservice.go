// Package hitlservice evaluates approval policies for tool calls.
// Policy decisions (allow / deny / approve) are returned to the caller; the
// caller (typically a ToolsRepo decorator like localtools.HITLWrapper) is
// responsible for actually pausing execution and sourcing the human decision.
package hitlservice

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/contenox/contenox/runtime/vfsservice"
	"github.com/google/uuid"
)

type KVReader interface {
	GetKV(ctx context.Context, key string, out interface{}) error
}

type PolicyEvaluator interface {
	Evaluate(ctx context.Context, toolsName, toolName string, args map[string]any) (EvaluationResult, error)
}

type Service interface {
	PolicyEvaluator

	RequestApproval(ctx context.Context, req ApprovalRequest, sink taskengine.TaskEventSink) (bool, error)

	Respond(approvalID string, approved bool) bool
}

type service struct {
	vfs     vfsservice.Service
	store   KVReader
	tracker libtracker.ActivityTracker

	mu      sync.Mutex
	pending map[string]chan bool
}

func New(vfs vfsservice.Service, store KVReader, tracker libtracker.ActivityTracker) Service {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	return &service{
		vfs:     vfs,
		store:   store,
		tracker: tracker,
		pending: make(map[string]chan bool),
	}
}

var _ Service = (*service)(nil)

const kvPrefixHITLPolicy = "cli.hitl-policy-name"

func (s *service) readActivePolicyName(ctx context.Context) string {
	var val string
	if err := s.store.GetKV(ctx, kvPrefixHITLPolicy, &val); err != nil {
		return ""
	}
	return strings.TrimSpace(val)
}

func (s *service) Evaluate(ctx context.Context, toolsName, toolName string, args map[string]any) (EvaluationResult, error) {
	reportErr, reportChange, end := s.tracker.Start(ctx, "hitl", "evaluate", "toolsName", toolsName, "toolName", toolName)
	defer end()
	policyPath := s.readActivePolicyName(ctx)
	if policyPath == "" {
		policyPath = "hitl-policy-default.json"
	}
	p, err := loadPolicy(ctx, s.vfs, policyPath)
	if err != nil {
		reportErr(fmt.Errorf("hitl: falling back to built-in default policy: %w", err))
		p = defaultPolicy()
	}
	reportChange("policy", policyPath)
	return evaluate(p, toolsName, toolName, args), nil
}

func (s *service) RequestApproval(ctx context.Context, req ApprovalRequest, sink taskengine.TaskEventSink) (bool, error) {
	approvalID := uuid.NewString()

	ch := make(chan bool, 1)
	s.mu.Lock()
	s.pending[approvalID] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.pending, approvalID)
		s.mu.Unlock()
	}()

	ev := taskengine.NewTaskEvent(ctx, taskengine.TaskEventApprovalRequested)
	ev.ApprovalID = approvalID
	ev.HookName = req.ToolsName
	ev.ToolName = req.ToolName
	ev.ApprovalArgs = req.Args
	ev.ApprovalDiff = req.Diff
	if err := sink.PublishTaskEvent(ctx, ev); err != nil {
		slog.Warn("hitl: failed to publish approval_requested event", "error", err)
	}

	select {
	case approved := <-ch:
		return approved, nil
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func (s *service) Respond(approvalID string, approved bool) bool {
	s.mu.Lock()
	ch, ok := s.pending[approvalID]
	s.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- approved:
		return true
	default:
		return false
	}
}

// Package hitlservice evaluates approval policies for tool calls.
// Policy decisions (allow / deny / approve) are returned to the caller; the
// caller (typically a ToolsRepo decorator like localtools.HITLWrapper) is
// responsible for actually pausing execution and sourcing the human decision.
package hitlservice

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/taskengine"
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

const defaultPolicyName = "hitl-policy-default.json"

type service struct {
	src            PolicySource
	tenantID       string
	store          KVReader
	tracker        libtracker.ActivityTracker
	fallbackPolicy string

	mu      sync.Mutex
	pending map[string]chan bool
}

// New constructs a hitlservice bound to a tenant. The tenantID is forwarded to
// every policy lookup the service performs.
func New(src PolicySource, tenantID string, store KVReader, tracker libtracker.ActivityTracker) Service {
	return NewWithDefaultPolicy(src, tenantID, store, tracker, defaultPolicyName)
}

func NewWithDefaultPolicy(src PolicySource, tenantID string, store KVReader, tracker libtracker.ActivityTracker, fallbackPolicy string) Service {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	if strings.TrimSpace(fallbackPolicy) == "" {
		fallbackPolicy = defaultPolicyName
	}
	return &service{
		src:            src,
		tenantID:       tenantID,
		store:          store,
		tracker:        tracker,
		fallbackPolicy: fallbackPolicy,
		pending:        make(map[string]chan bool),
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
		policyPath = s.fallbackPolicy
	}
	if policyPath == "" {
		policyPath = defaultPolicyName
	}
	p, err := loadPolicy(ctx, s.src, s.tenantID, policyPath)
	if err != nil {
		reportErr(fmt.Errorf("hitl: falling back to built-in default policy: %w", err))
		p = defaultPolicy()
	}
	reportChange("policy", policyPath)
	result := evaluate(p, toolsName, toolName, args)
	result.PolicyName = policyPath
	return result, nil
}

func (s *service) RequestApproval(ctx context.Context, req ApprovalRequest, sink taskengine.TaskEventSink) (bool, error) {
	approvalID := uuid.NewString()

	ch := make(chan bool)
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
		return false, fmt.Errorf("hitl: publish approval request: %w", err)
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

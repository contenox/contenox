// Package hitlservice evaluates approval policies for tool calls.
// Policy decisions (allow / deny / approve) are returned to the caller; the
// caller (typically a ToolsRepo decorator like localtools.HITLWrapper) is
// responsible for actually pausing execution and sourcing the human decision.
package hitlservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/vfsservice"
)

type KVReader interface {
	GetKV(ctx context.Context, key string, out interface{}) error
}

type PolicyEvaluator interface {
	Evaluate(ctx context.Context, toolsName, toolName string, args map[string]any) (EvaluationResult, error)
}

type service struct {
	vfs     vfsservice.Service
	store   KVReader
	tracker libtracker.ActivityTracker
}

func New(vfs vfsservice.Service, store KVReader, tracker libtracker.ActivityTracker) PolicyEvaluator {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	return &service{
		vfs:     vfs,
		store:   store,
		tracker: tracker,
	}
}

var _ PolicyEvaluator = (*service)(nil)

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

package vscodeagent

import (
	"context"
	"fmt"
	"sync"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/runtime/approvalflow"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/google/uuid"
)

type ApprovalBroker struct {
	mu           sync.Mutex
	pending      map[string]chan bool
	request      func(context.Context, libacp.RequestPermissionRequest) (libacp.RequestPermissionResponse, error)
	activePolicy func(context.Context) hitlPolicyRef
	sessionID    func(context.Context) string
	markPending  func(sessionID, toolCallID string)
	clearPending func(sessionID, toolCallID string)
}

func NewApprovalBroker(
	request func(context.Context, libacp.RequestPermissionRequest) (libacp.RequestPermissionResponse, error),
	activePolicy func(context.Context) hitlPolicyRef,
	sessionID func(context.Context) string,
	markPending func(sessionID, toolCallID string),
	clearPending func(sessionID, toolCallID string),
) *ApprovalBroker {
	return &ApprovalBroker{
		pending:      make(map[string]chan bool),
		request:      request,
		activePolicy: activePolicy,
		sessionID:    sessionID,
		markPending:  markPending,
		clearPending: clearPending,
	}
}

func (b *ApprovalBroker) AskApproval(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
	if b == nil || b.request == nil {
		return false, fmt.Errorf("vscodeagent: approval broker is not configured")
	}
	if req.ToolCallID == "" {
		req.ToolCallID = uuid.NewString()
	}

	var policy hitlPolicyRef
	if b.activePolicy != nil {
		policy = b.activePolicy(ctx)
	}
	sessionID := ""
	if b.sessionID != nil {
		sessionID = b.sessionID(ctx)
	}
	rpcReq := approvalflow.BuildRequest(req, approvalflow.BuildOptions{
		SessionID:  libacp.SessionID(sessionID),
		PolicyName: policy.Name,
		PolicyPath: policy.Path,
	})
	toolCallID := rpcReq.ToolCall.ToolCallID
	if b.markPending != nil {
		b.markPending(sessionID, toolCallID)
	}
	defer func() {
		if b.clearPending != nil {
			b.clearPending(sessionID, toolCallID)
		}
	}()

	resp, err := b.request(ctx, rpcReq)
	if err != nil {
		return false, err
	}
	switch resp.Outcome.Outcome {
	case libacp.PermissionOutcomeCancelled:
		return false, context.Canceled
	case libacp.PermissionOutcomeSelected:
		return resp.Outcome.OptionID == approvalflow.OptionAllow, nil
	default:
		return false, nil
	}
}

func (b *ApprovalBroker) Respond(approvalID string, approved bool) bool {
	b.mu.Lock()
	ch, ok := b.pending[approvalID]
	b.mu.Unlock()
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

func approvalTitle(req hitlservice.ApprovalRequest) string {
	return approvalflow.Title(req)
}

type approvalRequestEvent struct {
	ApprovalID string           `json:"approvalId"`
	ToolsName  string           `json:"toolsName,omitempty"`
	ToolName   string           `json:"toolName,omitempty"`
	Title      string           `json:"title"`
	PolicyName string           `json:"policyName,omitempty"`
	PolicyPath string           `json:"policyPath,omitempty"`
	Args       map[string]any   `json:"args,omitempty"`
	Diff       string           `json:"diff,omitempty"`
	DiffOld    string           `json:"diffOld,omitempty"`
	DiffNew    string           `json:"diffNew,omitempty"`
	Options    []approvalOption `json:"options"`
}

type approvalOption struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

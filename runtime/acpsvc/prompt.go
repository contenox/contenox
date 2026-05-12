package acpsvc

import (
	"context"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/libtracker"
	"github.com/contenox/contenox/runtime/agentservice"
	"github.com/contenox/contenox/runtime/taskengine"
)

func (t *Transport) Prompt(ctx context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	sess, ok := t.sessionFor(req.SessionID)
	if !ok {
		return libacp.PromptResponse{}, libacp.NewErrorf(libacp.ErrInvalidParams, "unknown session %q", req.SessionID)
	}
	if t.deps.ChainRegistry == nil || t.deps.ChainRegistry.Default() == nil {
		return libacp.PromptResponse{}, libacp.InternalError("no chain configured")
	}

	input := flattenPromptBlocks(req.Prompt)
	if input == "" {
		return libacp.PromptResponse{}, libacp.NewError(libacp.ErrInvalidParams, "empty prompt")
	}

	promptCtx := libtracker.WithNewRequestID(ctx)
	reqID, _ := promptCtx.Value(libtracker.ContextKeyRequestID).(string)

	rawCh := make(chan []byte, 64)
	bus := t.deps.Engine.Bus
	if bus != nil && reqID != "" {
		sub, err := bus.Stream(promptCtx, taskengine.TaskEventRequestSubject(reqID), rawCh)
		if err == nil {
			defer func() { _ = sub.Unsubscribe() }()
			go t.translateEvents(promptCtx, req.SessionID, rawCh)
		}
	}

	templateVars := map[string]string{
		"model":    t.deps.DefaultModel,
		"provider": t.defaultProvider(),
	}

	resp, err := sess.Agent.Prompt(promptCtx, agentservice.PromptRequest{
		SessionID:    sess.InternalSessionID,
		Input:        input,
		Chain:        t.deps.ChainRegistry.Default(),
		TemplateVars: templateVars,
	})
	if err != nil {
		if resp != nil {
			return libacp.PromptResponse{StopReason: mapStopReason(resp.StopReason)}, libacp.InternalError(err.Error())
		}
		return libacp.PromptResponse{}, libacp.InternalError(err.Error())
	}
	return libacp.PromptResponse{StopReason: mapStopReason(resp.StopReason)}, nil
}

func (t *Transport) defaultProvider() string {
	t.initMu.Lock()
	defer t.initMu.Unlock()
	return t.deps.DefaultProvider
}

func mapStopReason(r agentservice.StopReason) libacp.StopReason {
	switch r {
	case agentservice.StopEndTurn:
		return libacp.StopReasonEndTurn
	case agentservice.StopMaxTokens:
		return libacp.StopReasonMaxTokens
	case agentservice.StopMaxTurnRequests:
		return libacp.StopReasonMaxTurnRequests
	case agentservice.StopCancelled:
		return libacp.StopReasonCancelled
	}
	return libacp.StopReasonEndTurn
}

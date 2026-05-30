package acpsvc

import (
	"context"
	"errors"

	"github.com/contenox/agent/libacp"
	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/agentservice"
	"github.com/contenox/agent/runtime/taskengine"
)

func (t *Transport) Prompt(ctx context.Context, req libacp.PromptRequest) (libacp.PromptResponse, error) {
	reportErr, reportChange, end := t.tracker().Start(ctx, "prompt", "acp_session", "session_id", string(req.SessionID), "prompt_blocks", len(req.Prompt))
	defer end()

	sess, ok := t.sessionFor(req.SessionID)
	if !ok {
		err := libacp.NewErrorf(libacp.ErrInvalidParams, "unknown session %q", req.SessionID)
		reportErr(err)
		return libacp.PromptResponse{}, err
	}
	if t.deps.ChainRegistry == nil || t.deps.ChainRegistry.Default() == nil {
		err := libacp.InternalError("no chain configured")
		reportErr(err)
		return libacp.PromptResponse{}, err
	}

	input, droppedContentKinds := flattenPromptBlocks(req.Prompt)
	if input == "" {
		err := libacp.NewError(libacp.ErrInvalidParams, "empty prompt")
		reportErr(err)
		return libacp.PromptResponse{}, err
	}

	promptCtx := libtracker.WithNewRequestID(ctx)
	reqID, _ := promptCtx.Value(libtracker.ContextKeyRequestID).(string)

	rawCh := make(chan []byte, 64)
	bus := t.deps.Engine.Bus
	if bus != nil && reqID != "" {
		sub, err := bus.Stream(promptCtx, taskengine.TaskEventRequestSubject(reqID), rawCh)
		if err != nil {
			// The prompt still runs, but the client gets no incremental
			// updates. Surface why instead of silently degrading.
			subErr, _, subEnd := t.tracker().Start(promptCtx, "subscribe", "acp_event_stream", "session_id", string(req.SessionID), "request_id", reqID)
			subErr(err)
			subEnd()
		} else {
			translateDone := make(chan struct{})
			go func() {
				defer close(translateDone)
				t.translateEvents(promptCtx, req.SessionID, rawCh)
			}()
			defer func() {
				_ = sub.Unsubscribe()
				close(rawCh)
				<-translateDone
			}()
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
		cancelled := (resp != nil && resp.StopReason == agentservice.StopCancelled) ||
			promptCtx.Err() != nil ||
			errors.Is(err, context.Canceled)
		if cancelled {
			reportChange(string(req.SessionID), map[string]any{
				"stop_reason":           string(libacp.StopReasonCancelled),
				"request_id":            reqID,
				"dropped_content_kinds": droppedContentKinds,
			})
			return libacp.PromptResponse{StopReason: libacp.StopReasonCancelled}, nil
		}
		reportErr(err)
		if resp != nil {
			return libacp.PromptResponse{StopReason: mapStopReason(resp.StopReason)}, libacp.InternalError(err.Error())
		}
		return libacp.PromptResponse{}, libacp.InternalError(err.Error())
	}
	stopReason := mapStopReason(resp.StopReason)
	reportChange(string(req.SessionID), map[string]any{
		"stop_reason":           string(stopReason),
		"request_id":            reqID,
		"dropped_content_kinds": droppedContentKinds,
	})
	return libacp.PromptResponse{StopReason: stopReason}, nil
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

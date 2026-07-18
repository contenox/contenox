package acpsvc

import (
	"context"
	"errors"
	"time"

	"github.com/contenox/runtime/libacp"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/hitlservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/taskengine"
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

	if name, args, ok := parseCommand(input); ok {
		cmdCtx := libtracker.WithNewRequestID(ctx)
		return t.dispatchCommand(cmdCtx, req.SessionID, sess, name, args)
	}

	promptCtx := libtracker.WithNewRequestID(ctx)
	reqID, _ := promptCtx.Value(libtracker.ContextKeyRequestID).(string)

	// Make this turn cancellable and register it so session/cancel (Transport.Cancel),
	// a session Close/Delete, or a connection drop can abort the running chain.
	// promptCtx already inherits libacp's connection-level prompt context, but the
	// server owns cancellation here rather than relying solely on that. The
	// deferred unregister+cancel cleans up on turn end; cancelling produces
	// context.Canceled, which the error path below resolves as StopReasonCancelled.
	promptCtx, cancelPrompt := context.WithCancel(promptCtx)
	promptReg := t.registerPromptCancel(req.SessionID, cancelPrompt)
	defer func() {
		t.unregisterPromptCancel(req.SessionID, promptReg)
		cancelPrompt()
	}()

	// Gate this turn's tool calls under THIS session's chosen HITL policy. serve
	// runs one shared engine (one hitlservice) behind every ACP session, so a
	// concrete per-session selection must ride the request context: WithPolicyName
	// makes hitlservice.Evaluate prefer it over the process-global
	// cli.hitl-policy-name KV, letting two concurrent sessions gate independently.
	// A defaulting session resolves to "" and injects nothing, leaving the global-
	// KV/fallback chain intact (byte-identical to pre-per-session behavior). The
	// context threads synchronously prompt -> agentservice -> taskengine tool
	// gating -> HITLWrapper.Exec -> hitlservice.Evaluate.
	if policyName := t.resolveSessionHITLPolicy(sess); policyName != "" {
		promptCtx = hitlservice.WithPolicyName(promptCtx, policyName)
	}

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

	templateVars := t.chainTemplateVars(sess)
	templateVars["think"] = sess.think()
	var toolsAllowlist []string
	if t.deps.DB != nil {
		var err error
		toolsAllowlist, err = t.runtimeToolsAllowlist(promptCtx, runtimetypes.New(t.deps.DB.WithoutTransaction()), sess.McpServerNames)
		if err != nil {
			reportErr(err)
			return libacp.PromptResponse{}, libacp.InternalError(err.Error())
		}
	}

	// Use the session's effective token budget (chain token_limit or override set via config)
	// as the context window for this prompt. This is clamped to model cap (if known).
	// This makes indicators (which now use the session budget as "size") and engine shifting
	// consistent with the value the user sees and switches.
	contextLen := sess.effectiveTokenLimit()
	if contextLen == 0 {
		// fallback to model cap (for indicator size) if no explicit session budget
		currentModel := sess.modelOrDefault(t.model())
		for _, state := range t.runtimeStates(promptCtx) {
			for _, pulled := range state.PulledModels {
				if pulled.Model == currentModel && pulled.ContextLength > 0 {
					contextLen = pulled.ContextLength
					break
				}
			}
			if contextLen > 0 {
				break
			}
		}
		if contextLen == 0 {
			for _, state := range t.runtimeStates(promptCtx) {
				for _, pulled := range state.PulledModels {
					if pulled.ContextLength > 0 && (pulled.CanChat || pulled.CanPrompt) {
						contextLen = pulled.ContextLength
						break
					}
				}
				if contextLen > 0 {
					break
				}
			}
		}
	}

	resp, err := sess.Agent.Prompt(promptCtx, agentservice.PromptRequest{
		SessionID:      sess.InternalSessionID,
		Input:          input,
		Chain:          t.deps.ChainRegistry.Default(),
		TemplateVars:   templateVars,
		ToolsAllowlist: toolsAllowlist,
		ContextLength:  contextLen,
	})
	if err != nil {
		// Distinguish a genuine user cancellation from an execution failure that
		// merely SURFACED as a timeout. Only context.Canceled is a cancellation
		// (the client sent session/cancel, or the connection/parent context was
		// torn down). context.DeadlineExceeded — e.g. modeld refusing to load a
		// model, or waiting on a busy single GPU slot until an inner LLM call
		// deadlines — is a FAILURE the client must SEE, not a silent clean stop.
		// agentservice.InferStopReason maps BOTH to StopCancelled, so trusting
		// resp.StopReason (or a bare promptCtx.Err()) here would let a hard
		// failure masquerade as a cancel and vanish from the UI: the client
		// resolves the prompt with no error, drops its "prompting" state, and
		// shows nothing. Key the silent-cancel path on context.Canceled only.
		cancelled := errors.Is(err, context.Canceled) ||
			errors.Is(promptCtx.Err(), context.Canceled) ||
			errors.Is(ctx.Err(), context.Canceled)
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
	// A cancelled turn MUST resolve with the cancelled stop reason even when
	// the engine absorbed the cancellation and handed back a "successful"
	// partial result (e.g. via a recovery task) — the client sent
	// session/cancel or $/cancel_request and judges conformance by this field.
	// Keyed on context.Canceled specifically (not any ctx error): a deadline
	// that fired against a salvaged result is a timeout, not a user cancel.
	if errors.Is(promptCtx.Err(), context.Canceled) {
		stopReason = libacp.StopReasonCancelled
	}
	// Session pickers key freshness off updatedAt; push it after the turn so
	// clients don't need to re-list to notice activity. Push the derived title
	// alongside it: a session created this connection carried NO title in its
	// session/new SessionInfo, so without this the client's tab/sidebar label
	// is stuck on the raw-id fallback ("Sitzung acp-XXXX") until a full
	// session/list re-list (only on reconnect). Deriving from the first user
	// message here mirrors session/list's sessionListTitle, so the live push
	// and the re-list agree.
	libacp.AfterResponse(ctx, func() {
		update := libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateSessionInfo,
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		}
		if title := t.sessionInfoTitle(ctx, sess.InternalSessionID); title != "" {
			update.Title = title
		}
		t.sendUpdate(ctx, libacp.SessionNotification{
			SessionID: req.SessionID,
			Update:    update,
		})
	})
	reportChange(string(req.SessionID), map[string]any{
		"stop_reason":           string(stopReason),
		"request_id":            reqID,
		"dropped_content_kinds": droppedContentKinds,
	})
	return libacp.PromptResponse{StopReason: stopReason}, nil
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

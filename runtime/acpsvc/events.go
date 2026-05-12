package acpsvc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/localtools"
	"github.com/contenox/contenox/runtime/taskengine"
)

func (t *Transport) translateEvents(ctx context.Context, sid libacp.SessionID, ch <-chan []byte) {
	for {
		select {
		case <-ctx.Done():
			return
		case payload, ok := <-ch:
			if !ok {
				return
			}
			t.publishEvent(sid, payload)
		}
	}
}

func (t *Transport) publishEvent(sid libacp.SessionID, payload []byte) {
	var ev taskengine.TaskEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return
	}
	switch ev.Kind {
	case taskengine.TaskEventStepChunk:
		if ev.Content != "" {
			_ = t.conn.SessionUpdate(libacp.SessionNotification{
				SessionID: sid,
				Update:    libacp.NewAgentMessageChunk(ev.Content),
			})
		}
		if ev.Thinking != "" {
			_ = t.conn.SessionUpdate(libacp.SessionNotification{
				SessionID: sid,
				Update:    libacp.NewAgentThoughtChunk(ev.Thinking),
			})
		}
	case taskengine.TaskEventStepStarted:
		if isToolBearingHandler(ev.TaskHandler) {
			return
		}
		_ = t.conn.SessionUpdate(toolCallNotification(sid, ev, libacp.ToolCallStatusInProgress))
	case taskengine.TaskEventStepCompleted:
		if isToolBearingHandler(ev.TaskHandler) {
			return
		}
		_ = t.conn.SessionUpdate(toolCallNotification(sid, ev, libacp.ToolCallStatusCompleted))
	case taskengine.TaskEventStepFailed:
		if isToolBearingHandler(ev.TaskHandler) {
			return
		}
		_ = t.conn.SessionUpdate(toolCallNotification(sid, ev, libacp.ToolCallStatusFailed))
	case taskengine.TaskEventToolCallPending:
		_ = t.conn.SessionUpdate(toolCallPendingNotification(sid, ev))
	case taskengine.TaskEventToolCall:
		_ = t.conn.SessionUpdate(toolCallUpdateNotification(sid, ev))
	}
}

func isToolBearingHandler(handler string) bool {
	switch taskengine.TaskHandler(handler) {
	case taskengine.HandleExecuteToolCalls, taskengine.HandleTools, taskengine.HandleChatCompletion:
		return true
	}
	return false
}

func toolCallNotification(sid libacp.SessionID, ev taskengine.TaskEvent, status libacp.ToolCallStatus) libacp.SessionNotification {
	update := libacp.SessionUpdate{
		SessionUpdate: libacp.SessionUpdateToolCallUpdate,
		ToolCallID:    ev.TaskID,
		Title:         taskTitle(ev),
		Kind:          libacp.ToolKindOther,
		Status:        status,
	}
	if ev.Error != "" {
		update.Meta = json.RawMessage(`{"error":` + jsonString(ev.Error) + `}`)
	}
	return libacp.SessionNotification{SessionID: sid, Update: update}
}

func toolCallPendingNotification(sid libacp.SessionID, ev taskengine.TaskEvent) libacp.SessionNotification {
	update := libacp.SessionUpdate{
		SessionUpdate: libacp.SessionUpdateToolCall,
		ToolCallID:    toolCallID(ev),
		Title:         toolCallTitle(ev),
		Kind:          toolKindFor(ev.ToolName),
		Status:        libacp.ToolCallStatusPending,
	}
	if raw, err := json.Marshal(ev.ApprovalArgs); err == nil && len(ev.ApprovalArgs) > 0 {
		update.RawInput = raw
	}
	return libacp.SessionNotification{SessionID: sid, Update: update}
}

func toolCallUpdateNotification(sid libacp.SessionID, ev taskengine.TaskEvent) libacp.SessionNotification {
	status := libacp.ToolCallStatusCompleted
	if ev.Error != "" {
		status = libacp.ToolCallStatusFailed
	}

	update := libacp.SessionUpdate{
		SessionUpdate: libacp.SessionUpdateToolCallUpdate,
		ToolCallID:    toolCallID(ev),
		Title:         toolCallTitle(ev),
		Kind:          toolKindFor(ev.ToolName),
		Status:        status,
	}

	if raw, err := json.Marshal(ev.ApprovalArgs); err == nil && len(ev.ApprovalArgs) > 0 {
		update.RawInput = raw
	}
	if ev.Content != "" {
		update.RawOutput = json.RawMessage(jsonString(ev.Content))
	}

	if diff := diffContentFromResult(ev.Content); diff != nil {
		update.ToolContent = []libacp.ToolCallContent{*diff}
	}

	if ev.Error != "" {
		update.Meta = json.RawMessage(`{"error":` + jsonString(ev.Error) + `}`)
	}

	return libacp.SessionNotification{SessionID: sid, Update: update}
}

func toolCallID(ev taskengine.TaskEvent) string {
	if ev.ApprovalID != "" {
		return ev.ApprovalID
	}
	if ev.ToolName != "" {
		return ev.ToolName
	}
	return ev.TaskID
}

func toolCallTitle(ev taskengine.TaskEvent) string {
	if ev.ToolName != "" {
		return ev.ToolName
	}
	return taskTitle(ev)
}

func toolKindFor(toolName string) libacp.ToolKind {
	if toolName == "" {
		return libacp.ToolKindOther
	}
	base := toolName
	if idx := strings.Index(toolName, "."); idx >= 0 {
		base = toolName[idx+1:]
	}
	switch {
	case strings.HasPrefix(base, "read"), strings.HasPrefix(base, "stat"), strings.HasPrefix(base, "list"), strings.HasPrefix(base, "grep"):
		return libacp.ToolKindRead
	case strings.HasPrefix(base, "write"), base == "sed":
		return libacp.ToolKindEdit
	case strings.HasPrefix(base, "delete"):
		return libacp.ToolKindDelete
	case strings.HasPrefix(base, "move"):
		return libacp.ToolKindMove
	case strings.HasPrefix(base, "fetch"), strings.HasPrefix(base, "http"):
		return libacp.ToolKindFetch
	case strings.HasPrefix(toolName, "local_shell."), strings.HasPrefix(base, "exec"), strings.HasPrefix(base, "run"):
		return libacp.ToolKindExecute
	}
	return libacp.ToolKindOther
}

func diffContentFromResult(content string) *libacp.ToolCallContent {
	if content == "" {
		return nil
	}
	var fw localtools.FsWriteResult
	if err := json.Unmarshal([]byte(content), &fw); err != nil {
		return nil
	}
	if fw.Path == "" || !fw.Written {
		return nil
	}
	return &libacp.ToolCallContent{
		Type:    libacp.ToolCallContentDiff,
		Path:    fw.Path,
		OldText: fw.OldText,
		NewText: fw.NewText,
	}
}

func taskTitle(ev taskengine.TaskEvent) string {
	if ev.TaskID != "" {
		return ev.TaskID + " (" + ev.TaskHandler + ")"
	}
	return ev.TaskHandler
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

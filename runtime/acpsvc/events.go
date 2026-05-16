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
	for payload := range ch {
		t.publishEvent(ctx, sid, payload)
	}
}

func (t *Transport) publishEvent(ctx context.Context, sid libacp.SessionID, payload []byte) {
	var ev taskengine.TaskEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return
	}
	switch ev.Kind {
	case taskengine.TaskEventStepChunk:
		if ev.Content != "" {
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sid,
				Update:    libacp.NewAgentMessageChunk(ev.Content),
			})
		}
		if ev.Thinking != "" {
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sid,
				Update:    libacp.NewAgentThoughtChunk(ev.Thinking),
			})
		}
	case taskengine.TaskEventStepStarted:
		if isToolBearingHandler(ev.TaskHandler) {
			return
		}
		t.sendUpdate(ctx, toolCallNotification(sid, ev, libacp.ToolCallStatusInProgress))
	case taskengine.TaskEventStepCompleted:
		if isToolBearingHandler(ev.TaskHandler) {
			return
		}
		t.sendUpdate(ctx, toolCallNotification(sid, ev, libacp.ToolCallStatusCompleted))
	case taskengine.TaskEventStepFailed:
		if isToolBearingHandler(ev.TaskHandler) {
			return
		}
		t.sendUpdate(ctx, toolCallNotification(sid, ev, libacp.ToolCallStatusFailed))
	case taskengine.TaskEventPrint:
		if ev.Content != "" {
			t.sendUpdate(ctx, libacp.SessionNotification{
				SessionID: sid,
				Update:    libacp.NewAgentMessageChunk(ev.Content),
			})
		}
	case taskengine.TaskEventToolCallPending:
		t.sendToolCallUpdateGuarded(ctx, sid, toolCallID(ev), toolCallPendingNotification(sid, ev))
	case taskengine.TaskEventToolCall:
		t.sendToolCallUpdateGuarded(ctx, sid, toolCallID(ev), toolCallUpdateNotification(sid, ev))
	}
}

func isToolBearingHandler(handler string) bool {
	switch taskengine.TaskHandler(handler) {
	case taskengine.HandleExecuteToolCalls, taskengine.HandleTools, taskengine.HandleChatCompletion:
		return true
	}
	return false
}

func toolCallInProgressNotification(sid libacp.SessionID, ev taskengine.TaskEvent) libacp.SessionNotification {
	return libacp.SessionNotification{
		SessionID: sid,
		Update: libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateToolCallUpdate,
			ToolCallID:    toolCallID(ev),
			Kind:          toolKindFor(ev.ToolName),
			Status:        libacp.ToolCallStatusInProgress,
		},
	}
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
	if locs := toolCallLocations(ev); len(locs) > 0 {
		update.Locations = locs
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

	if locs := toolCallLocations(ev); len(locs) > 0 {
		update.Locations = locs
	}

	if ev.Error != "" {
		update.Meta = json.RawMessage(`{"error":` + jsonString(ev.Error) + `}`)
	}

	return libacp.SessionNotification{SessionID: sid, Update: update}
}

func toolCallIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(taskengine.ContextKeyToolCallID).(string)
	return v
}

func terminalAttachNotification(sid libacp.SessionID, toolCallID, terminalID string) libacp.SessionNotification {
	return libacp.SessionNotification{
		SessionID: sid,
		Update: libacp.SessionUpdate{
			SessionUpdate: libacp.SessionUpdateToolCall,
			ToolCallID:    toolCallID,
			Kind:          libacp.ToolKindExecute,
			Status:        libacp.ToolCallStatusInProgress,
			ToolContent: []libacp.ToolCallContent{
				{Type: libacp.ToolCallContentTerminal, TerminalID: terminalID},
			},
		},
	}
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
		if summary := summarizeToolCallArgs(ev.ToolName, ev.ApprovalArgs); summary != "" {
			return ev.ToolName + ": " + summary
		}
		return ev.ToolName
	}
	return taskTitle(ev)
}

func summarizeToolCallArgs(toolName string, args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	base := toolName
	if idx := strings.LastIndex(toolName, "."); idx >= 0 {
		base = toolName[idx+1:]
	}

	asString := func(key string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	asStringSlice := func(key string) []string {
		v, ok := args[key]
		if !ok {
			return nil
		}
		arr, ok := v.([]any)
		if !ok {
			return nil
		}
		out := make([]string, 0, len(arr))
		for _, x := range arr {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}

	var summary string
	switch base {
	case "exec", "run", "execute", "local_shell":
		cmd := asString("command")
		if cmd == "" {
			break
		}
		parts := []string{cmd}
		if a := asString("args"); a != "" {
			parts = append(parts, a)
		} else {
			parts = append(parts, asStringSlice("args")...)
		}
		summary = strings.Join(parts, " ")
	case "read_file", "read_file_range", "write_file", "stat_file", "list_dir", "delete_file":
		summary = asString("path")
	case "grep":
		if p := asString("pattern"); p != "" {
			if path := asString("path"); path != "" {
				summary = p + " in " + path
			} else {
				summary = p
			}
		}
	case "sed":
		if path := asString("path"); path != "" {
			if pat := asString("pattern"); pat != "" {
				summary = pat + " in " + path
			} else {
				summary = path
			}
		}
	case "fetch_url", "fetch", "http_get":
		summary = asString("url")
	}

	if summary == "" {
		return ""
	}
	summary = strings.TrimSpace(strings.ReplaceAll(summary, "\n", " "))
	const maxRunes = 80
	if r := []rune(summary); len(r) > maxRunes {
		summary = string(r[:maxRunes-1]) + "…"
	}
	return summary
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

func toolCallLocations(ev taskengine.TaskEvent) []libacp.ToolCallLocation {
	path := ""
	if diff := diffContentFromResult(ev.Content); diff != nil && diff.Path != "" {
		path = diff.Path
	} else if p, ok := ev.ApprovalArgs["path"].(string); ok {
		path = strings.TrimSpace(p)
	}
	if path == "" {
		return nil
	}
	return []libacp.ToolCallLocation{{Path: path}}
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

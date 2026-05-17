package acpsvc

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/hitlservice"
	"github.com/contenox/contenox/runtime/runtimetypes"
)

const (
	permissionOptionAllow = "allow"
	permissionOptionDeny  = "deny"
)

func (t *Transport) AskApproval(ctx context.Context, req hitlservice.ApprovalRequest) (bool, error) {
	contenoxSessionID, _ := ctx.Value(runtimetypes.SessionIDContextKey).(string)
	acpSessionID, ok := t.acpSessionForContenoxID(contenoxSessionID)
	if !ok {
		return false, libacp.InternalError("acpsvc: no ACP session bound to contenox session " + contenoxSessionID)
	}

	toolCallID := req.ToolCallID
	if toolCallID == "" {
		toolCallID = req.ToolsName + "." + req.ToolName
	}

	summary := summarizeToolCallArgs(req.ToolName, req.Args)
	title := req.ToolsName + "." + req.ToolName
	if summary != "" {
		title += ": " + summary
	}

	content := diffContent(diffPath(req.Args), req.DiffOld, req.DiffNew)
	if content == nil && summary != "" {
		cb := libacp.NewTextContent(summary)
		content = []libacp.ToolCallContent{
			{Type: libacp.ToolCallContentRegular, Content: &cb},
		}
	}

	rpcReq := libacp.RequestPermissionRequest{
		SessionID: acpSessionID,
		ToolCall: libacp.PermissionToolCall{
			ToolCallID: toolCallID,
			Title:      title,
			Kind:       permissionKindFor(req.ToolsName, req.ToolName),
			Status:     libacp.ToolCallStatusPending,
			RawInput:   marshalArgs(req.Args),
			Content:    content,
		},
		Options: []libacp.PermissionOption{
			{OptionID: permissionOptionAllow, Name: "Allow", Kind: libacp.PermissionAllowOnce},
			{OptionID: permissionOptionDeny, Name: "Deny", Kind: libacp.PermissionRejectOnce},
		},
	}

	t.markPermissionPending(acpSessionID, toolCallID)
	defer t.clearPermissionPending(acpSessionID, toolCallID)

	reportErr, reportChange, end := t.tracker().Start(ctx, "hitl", "acp_permission", "tool_call_id", toolCallID)
	defer end()

	resp, err := t.conn.RequestPermission(ctx, rpcReq)
	if err != nil {
		ctxErr := ""
		if e := ctx.Err(); e != nil {
			ctxErr = e.Error()
		}
		reportChange("rpc_error", err.Error())
		reportChange("ctx_err", ctxErr)
		reportErr(err)
		return false, err
	}
	reportChange("outcome", string(resp.Outcome.Outcome))
	reportChange("option_id", resp.Outcome.OptionID)
	switch resp.Outcome.Outcome {
	case libacp.PermissionOutcomeCancelled:
		return false, context.Canceled
	case libacp.PermissionOutcomeSelected:
		return resp.Outcome.OptionID == permissionOptionAllow, nil
	}
	return false, nil
}

func permissionKindFor(toolsName, toolName string) libacp.ToolKind {
	switch toolsName {
	case "local_shell":
		return libacp.ToolKindExecute
	case "local_fs":
		switch {
		case strings.Contains(toolName, "read"), strings.Contains(toolName, "list"), strings.Contains(toolName, "grep"), strings.Contains(toolName, "find"):
			return libacp.ToolKindRead
		case strings.Contains(toolName, "write"), strings.Contains(toolName, "sed"), strings.Contains(toolName, "edit"), strings.Contains(toolName, "patch"):
			return libacp.ToolKindEdit
		case strings.Contains(toolName, "move"), strings.Contains(toolName, "rename"):
			return libacp.ToolKindMove
		case strings.Contains(toolName, "delete"), strings.Contains(toolName, "remove"), strings.Contains(toolName, "rm"):
			return libacp.ToolKindDelete
		}
		return libacp.ToolKindEdit
	case "webtools":
		return libacp.ToolKindFetch
	}
	return libacp.ToolKindOther
}

func marshalArgs(args map[string]any) json.RawMessage {
	if len(args) == 0 {
		return nil
	}
	raw, err := json.Marshal(args)
	if err != nil {
		return nil
	}
	return raw
}

func diffPath(args map[string]any) string {
	if p, ok := args["path"].(string); ok {
		return strings.TrimSpace(p)
	}
	return ""
}

func diffContent(path, oldText, newText string) []libacp.ToolCallContent {
	if path == "" || oldText == newText {
		return nil
	}
	return []libacp.ToolCallContent{
		{Type: libacp.ToolCallContentDiff, Path: path, OldText: oldText, NewText: newText},
	}
}

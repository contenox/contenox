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

	rpcReq := libacp.RequestPermissionRequest{
		SessionID: acpSessionID,
		ToolCall: libacp.PermissionToolCall{
			ToolCallID: req.ToolsName + "." + req.ToolName,
			Title:      req.ToolsName + "." + req.ToolName,
			Kind:       permissionKindFor(req.ToolsName, req.ToolName),
			Status:     libacp.ToolCallStatusPending,
			RawInput:   marshalArgs(req.Args),
			Content:    diffContent(req.Diff),
		},
		Options: []libacp.PermissionOption{
			{OptionID: permissionOptionAllow, Name: "Allow", Kind: libacp.PermissionAllowOnce},
			{OptionID: permissionOptionDeny, Name: "Deny", Kind: libacp.PermissionRejectOnce},
		},
	}

	resp, err := t.conn.RequestPermission(ctx, rpcReq)
	if err != nil {
		return false, err
	}
	switch resp.Outcome.Outcome {
	case libacp.PermissionOutcomeCancelled:
		return false, context.Canceled
	case libacp.PermissionOutcomeSelected:
		return resp.Outcome.OptionID == permissionOptionAllow, nil
	}
	return false, nil
}

func permissionKindFor(toolsName, toolName string) libacp.ToolKind {
	switch {
	case toolsName == "local_shell":
		return libacp.ToolKindExecute
	case toolsName == "local_fs":
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
	case toolsName == "webtools":
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

func diffContent(diff string) []libacp.ToolCallContent {
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return nil
	}
	return []libacp.ToolCallContent{
		{Type: libacp.ToolCallContentDiff, NewText: diff},
	}
}

package acpsvc

import (
	"encoding/json"
	"testing"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/localtools"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/stretchr/testify/require"
)

func TestUnit_ToolCallUpdate_FsWriteResultProducesDiff(t *testing.T) {
	fw := localtools.FsWriteResult{
		Path:    "/tmp/abc.txt",
		OldText: "old",
		NewText: "new",
		Written: true,
	}
	raw, err := json.Marshal(fw)
	require.NoError(t, err)

	ev := taskengine.TaskEvent{
		Kind:         taskengine.TaskEventToolCall,
		ToolName:     "local_fs.write_file",
		ApprovalID:   "call-1",
		ApprovalArgs: map[string]any{"path": "/tmp/abc.txt", "content": "new"},
		Content:      string(raw),
	}

	note := toolCallUpdateNotification(libacp.SessionID("sess-1"), ev)
	upd := note.Update

	require.Equal(t, libacp.SessionUpdateToolCallUpdate, upd.SessionUpdate)
	require.Equal(t, "call-1", upd.ToolCallID)
	require.Equal(t, "local_fs.write_file", upd.Title)
	require.Equal(t, libacp.ToolKindEdit, upd.Kind)
	require.Equal(t, libacp.ToolCallStatusCompleted, upd.Status)
	require.Len(t, upd.ToolContent, 1)
	require.Equal(t, libacp.ToolCallContentDiff, upd.ToolContent[0].Type)
	require.Equal(t, "/tmp/abc.txt", upd.ToolContent[0].Path)
	require.Equal(t, "old", upd.ToolContent[0].OldText)
	require.Equal(t, "new", upd.ToolContent[0].NewText)

	wire, err := json.Marshal(upd)
	require.NoError(t, err)

	var generic map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(wire, &generic))
	require.Contains(t, generic, "content", "tool_call update must serialize ToolContent under the `content` key per ACP spec")
	require.NotContains(t, generic, "content_list")

	var contentArr []libacp.ToolCallContent
	require.NoError(t, json.Unmarshal(generic["content"], &contentArr))
	require.Len(t, contentArr, 1)
	require.Equal(t, libacp.ToolCallContentDiff, contentArr[0].Type)
}

func TestUnit_ToolCallUpdate_NonFsResultHasNoDiff(t *testing.T) {
	ev := taskengine.TaskEvent{
		Kind:     taskengine.TaskEventToolCall,
		ToolName: "echo",
		Content:  "\"hello\"",
	}
	note := toolCallUpdateNotification(libacp.SessionID("sess-1"), ev)
	require.Len(t, note.Update.ToolContent, 0)
	require.Equal(t, libacp.ToolKindOther, note.Update.Kind)
}

func TestUnit_ToolCallUpdate_ErrorMarksFailed(t *testing.T) {
	ev := taskengine.TaskEvent{
		Kind:     taskengine.TaskEventToolCall,
		ToolName: "local_shell.exec",
		Error:    "boom",
		Content:  "stderr: boom",
	}
	note := toolCallUpdateNotification(libacp.SessionID("sess-1"), ev)
	require.Equal(t, libacp.ToolCallStatusFailed, note.Update.Status)
	require.Equal(t, libacp.ToolKindExecute, note.Update.Kind)
}

func TestUnit_IsToolBearingHandler(t *testing.T) {
	require.True(t, isToolBearingHandler(string(taskengine.HandleChatCompletion)))
	require.True(t, isToolBearingHandler(string(taskengine.HandleExecuteToolCalls)))
	require.True(t, isToolBearingHandler(string(taskengine.HandleTools)))
	require.False(t, isToolBearingHandler(string(taskengine.HandleNoop)))
	require.False(t, isToolBearingHandler(string(taskengine.HandlePromptToString)))
}

package acpsvc

import (
	"encoding/json"
	"strings"
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
	require.Equal(t, "local_fs.write_file: /tmp/abc.txt", upd.Title)
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

func TestUnit_ReplayToolCall_FromAssistantMessage(t *testing.T) {
	tc := taskengine.ToolCall{
		ID:   "call-xyz",
		Type: "function",
		Function: taskengine.FunctionCall{
			Name:      "local_fs.read_file",
			Arguments: `{"path":"/tmp/foo.txt"}`,
		},
	}
	upd := toolCallUpdateFromCall(tc)

	require.Equal(t, libacp.SessionUpdateToolCall, upd.SessionUpdate)
	require.Equal(t, "call-xyz", upd.ToolCallID)
	require.Equal(t, "local_fs.read_file: /tmp/foo.txt", upd.Title)
	require.Equal(t, libacp.ToolKindRead, upd.Kind)
	require.Equal(t, libacp.ToolCallStatusCompleted, upd.Status)
	require.JSONEq(t, `{"path":"/tmp/foo.txt"}`, string(upd.RawInput))
}

func TestUnit_ReplayToolCall_InvalidArgumentsOmitsRawInput(t *testing.T) {
	tc := taskengine.ToolCall{
		ID: "call-1",
		Function: taskengine.FunctionCall{
			Name:      "local_shell.exec",
			Arguments: "not-json",
		},
	}
	upd := toolCallUpdateFromCall(tc)
	require.Empty(t, upd.RawInput, "malformed Arguments must not be forwarded as RawInput")
	require.Equal(t, libacp.ToolKindExecute, upd.Kind)
}

func TestUnit_ReplayToolResult_FromToolMessage(t *testing.T) {
	m := taskengine.Message{
		Role:       "tool",
		ToolCallID: "call-xyz",
		Content:    "hello world",
	}
	upd := toolCallUpdateFromResult(m)

	require.Equal(t, libacp.SessionUpdateToolCallUpdate, upd.SessionUpdate)
	require.Equal(t, "call-xyz", upd.ToolCallID)
	require.Equal(t, libacp.ToolCallStatusCompleted, upd.Status)
	require.JSONEq(t, `"hello world"`, string(upd.RawOutput))
	require.Empty(t, upd.ToolContent)
}

func TestUnit_SummarizeToolCallArgs(t *testing.T) {
	cases := []struct {
		name     string
		tool     string
		args     map[string]any
		expected string
	}{
		{"exec without args", "acp_terminal.exec", map[string]any{"command": "ls"}, "ls"},
		{"exec with args slice", "acp_terminal.exec", map[string]any{"command": "git", "args": []any{"status", "--short"}}, "git status --short"},
		{"read_file path", "acp_fs.read_file", map[string]any{"path": "/tmp/foo.txt"}, "/tmp/foo.txt"},
		{"local grep pattern+path", "local_fs.grep", map[string]any{"pattern": "TODO", "path": "src/"}, "TODO in src/"},
		{"grep pattern only", "grep", map[string]any{"pattern": "TODO"}, "TODO"},
		{"fetch_url", "webtools.fetch_url", map[string]any{"url": "https://example.com"}, "https://example.com"},
		{"unknown tool returns empty", "foo.bar", map[string]any{"x": "y"}, ""},
		{"missing main arg returns empty", "acp_terminal.exec", map[string]any{}, ""},
		{"newlines collapsed", "acp_terminal.exec", map[string]any{"command": "echo", "args": []any{"a\nb"}}, "echo a b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, summarizeToolCallArgs(c.tool, c.args))
		})
	}
}

func TestUnit_SummarizeToolCallArgs_LongCommandTruncates(t *testing.T) {
	long := strings.Repeat("x", 200)
	got := summarizeToolCallArgs("acp_terminal.exec", map[string]any{"command": long})
	require.LessOrEqual(t, len([]rune(got)), 80)
	require.Contains(t, got, "…")
}

func TestUnit_ReplayToolResult_FsWriteProducesDiff(t *testing.T) {
	fw := localtools.FsWriteResult{
		Path:    "/tmp/a.txt",
		OldText: "old",
		NewText: "new",
		Written: true,
	}
	raw, err := json.Marshal(fw)
	require.NoError(t, err)

	m := taskengine.Message{
		Role:       "tool",
		ToolCallID: "call-w",
		Content:    string(raw),
	}
	upd := toolCallUpdateFromResult(m)
	require.Len(t, upd.ToolContent, 1)
	require.Equal(t, libacp.ToolCallContentDiff, upd.ToolContent[0].Type)
	require.Equal(t, "/tmp/a.txt", upd.ToolContent[0].Path)
}

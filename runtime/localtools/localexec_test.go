package localtools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/contenox/contenox/runtime/localtools"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_LocalExecTools_Supports(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	names, err := h.Supports(ctx)
	require.NoError(t, err)
	require.Len(t, names, 1)
	assert.Equal(t, "local_shell", names[0])
}

func TestUnit_LocalExecTools_GetSchemasForSupportedTools(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	schemas, err := h.GetSchemasForSupportedTools(ctx)
	require.NoError(t, err)
	require.NotNil(t, schemas)
	require.Contains(t, schemas, "local_shell")
	assert.NotNil(t, schemas["local_shell"])
}

func TestUnit_LocalExecTools_GetToolsForToolsByName_OK(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	tools, err := h.GetToolsForToolsByName(ctx, "local_shell")
	require.NoError(t, err)
	require.Len(t, tools, 1)
	assert.Equal(t, "function", tools[0].Type)
	assert.Equal(t, "local_shell", tools[0].Function.Name)
	assert.Contains(t, tools[0].Function.Description, "Run a terminal command")
}

func TestUnit_LocalExecTools_GetToolsForToolsByName_Unknown(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	tools, err := h.GetToolsForToolsByName(ctx, "other")
	assert.Error(t, err)
	assert.Nil(t, tools)
}

func TestUnit_LocalExecTools_GetToolsForToolsByName_ContextPolicy_Description(t *testing.T) {
	// Tools constructed with NO static policy.
	// Context carries chain-level policy — description must reflect it.
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	ctx := taskengine.WithToolsArgs(context.Background(), "local_shell", map[string]string{
		"_allowed_commands": "git, ls",
		"_denied_commands":  "rm",
	})
	tools, err := h.GetToolsForToolsByName(ctx, "local_shell")
	require.NoError(t, err)
	require.Len(t, tools, 1)
	desc := tools[0].Function.Description
	assert.Contains(t, desc, "git")
	assert.Contains(t, desc, "ls")
	assert.Contains(t, desc, "rm")
}

func TestUnit_LocalExecTools_Exec_ContextPolicy_Enforced(t *testing.T) {
	// No static allowlist — context injects one. Command not in list must be rejected.
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	ctx := taskengine.WithToolsArgs(context.Background(), "local_shell", map[string]string{
		"_allowed_commands": "ls",
	})
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{"command": "echo", "args": "hello"},
	}
	_, _, err := h.Exec(ctx, time.Now().UTC(), nil, false, toolsCall)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in allowlist")
}

func TestUnit_LocalExecTools_Exec_ContextPolicy_Allows(t *testing.T) {
	// No static allowlist — context injects one that includes the command.
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	ctx := taskengine.WithToolsArgs(context.Background(), "local_shell", map[string]string{
		"_allowed_commands": "echo",
	})
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{"command": "echo", "args": "ctx policy works"},
	}
	out, dt, err := h.Exec(ctx, time.Now().UTC(), nil, false, toolsCall)
	require.NoError(t, err)
	assert.Equal(t, taskengine.DataTypeJSON, dt)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.True(t, res.Success)
	assert.Equal(t, "ctx policy works", res.Stdout)
}

// testAllowedCommands allows the commands used by Exec tests (echo, cat, sleep, shell, exit for shell mode).
var testAllowedCommands = []string{"echo", "cat", "sleep", "/bin/sh", "exit"}

func TestUnit_LocalExecTools_Exec_Success(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedCommands(testAllowedCommands)).(*localtools.LocalExecTools)
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "echo",
			"args":    "hello world",
		},
	}
	out, dt, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.NoError(t, err)
	assert.Equal(t, taskengine.DataTypeJSON, dt)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.True(t, res.Success)
	assert.Equal(t, 0, res.ExitCode)
	assert.Equal(t, "hello world", res.Stdout)
	assert.GreaterOrEqual(t, res.DurationSeconds, 0.0)
}

func TestUnit_LocalExecTools_Exec_Success_InputAsStdin(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedCommands(testAllowedCommands)).(*localtools.LocalExecTools)
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "cat",
		},
	}
	out, _, err := h.Exec(ctx, start, "stdin content here", false, toolsCall)
	require.NoError(t, err)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.True(t, res.Success)
	assert.Equal(t, "stdin content here", res.Stdout)
}

func TestUnit_LocalExecTools_Exec_NoPolicy_Allowed(t *testing.T) {
	// Authorization is the responsibility of upstream layers (e.g. HITLWrapper);
	// LocalExecTools without policy must not fail-close.
	ctx := context.Background()
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "echo",
			"args":    "open posture",
		},
	}
	out, _, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.NoError(t, err)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.True(t, res.Success)
	assert.Equal(t, "open posture", res.Stdout)
}

func TestUnit_LocalExecTools_Exec_ShellMode_NoPolicy_Allowed(t *testing.T) {
	// shell:true is allowed when no allowlist exists: the injection guard only
	// triggers when there is a policy for shell mode to bypass.
	ctx := context.Background()
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "echo shell test",
			"shell":   "true",
		},
	}
	out, _, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.NoError(t, err)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.True(t, res.Success)
	assert.Equal(t, "shell test", res.Stdout)
}

func TestUnit_LocalExecTools_Exec_ShellMode_WithPolicyRejected(t *testing.T) {
	// shell:true must be REJECTED when an allowlist policy is active to prevent
	// command injection (e.g. "git status; rm -rf /" bypassing allowlist checks).
	ctx := context.Background()
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedCommands(testAllowedCommands)).(*localtools.LocalExecTools)
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "echo shell test",
			"shell":   "true",
		},
	}
	_, _, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strictly forbidden")
}

func TestUnit_LocalExecTools_Exec_AllowlistReject(t *testing.T) {
	ctx := context.Background()
	// Only allow /usr/bin/env; echo should be rejected when we use allowedCommands.
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedCommands([]string{"/usr/bin/env"})).(*localtools.LocalExecTools)
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "echo",
			"args":    "forbidden",
		},
	}
	_, _, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not in allowlist")
}

func TestUnit_LocalExecTools_Exec_AllowlistDirReject(t *testing.T) {
	dir := t.TempDir()
	// allowedDir is dir; echo is typically /usr/bin/echo or /bin/echo, not under dir
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedDir(dir)).(*localtools.LocalExecTools)
	ctx := context.Background()
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{"command": "echo", "args": "x"},
	}
	_, _, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not under allowed dir")
}

func TestUnit_LocalExecTools_Exec_AllowlistDirAllow(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "script.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/sh\necho ok\n"), 0755)
	require.NoError(t, err)
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedDir(dir)).(*localtools.LocalExecTools)
	ctx := context.Background()
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{"command": scriptPath},
	}
	out, _, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.NoError(t, err)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.True(t, res.Success)
	assert.Equal(t, "ok", res.Stdout)
}

func TestUnit_LocalExecTools_Exec_Timeout(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedCommands(testAllowedCommands), localtools.WithLocalExecTimeout(50*time.Millisecond)).(*localtools.LocalExecTools)
	start := time.Now().UTC()
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "sleep",
			"args":    "2",
			"timeout": "50ms",
		},
	}
	out, _, err := h.Exec(ctx, start, nil, false, toolsCall)
	require.NoError(t, err)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.False(t, res.Success)
	// Process is killed on timeout; error may be "context deadline exceeded" or "signal: killed"
	assert.NotEmpty(t, res.Error, "expected some error on timeout")
}

func TestUnit_LocalExecTools_Exec_MissingCommand(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedCommands(testAllowedCommands)).(*localtools.LocalExecTools)
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{},
	}
	_, _, err := h.Exec(ctx, time.Now().UTC(), nil, false, toolsCall)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "command is required")
}

func TestUnit_LocalExecTools_Exec_NilTools(t *testing.T) {
	ctx := context.Background()
	h := localtools.NewLocalExecTools().(*localtools.LocalExecTools)
	_, _, err := h.Exec(ctx, time.Now().UTC(), nil, false, nil)
	require.Error(t, err)
}

func TestUnit_LocalExecTools_Exec_NonZeroExit(t *testing.T) {
	// Run a script under allowedDir WITHOUT shell mode to capture a non-zero exit.
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fail.sh")
	err := os.WriteFile(scriptPath, []byte("#!/bin/sh\nexit 3\n"), 0755)
	require.NoError(t, err)
	ctx := context.Background()
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedDir(dir)).(*localtools.LocalExecTools)
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{"command": scriptPath},
	}
	out, _, err := h.Exec(ctx, time.Now().UTC(), nil, false, toolsCall)
	require.NoError(t, err)
	res, ok := out.(*localtools.LocalExecResult)
	require.True(t, ok)
	assert.False(t, res.Success)
	assert.Equal(t, 3, res.ExitCode)
}

func TestUnit_LocalExecTools_Exec_NonZeroExit_WithPolicy_Rejected(t *testing.T) {
	// shell:true + allowlist must be rejected (security fix).
	ctx := context.Background()
	h := localtools.NewLocalExecTools(localtools.WithLocalExecAllowedCommands(testAllowedCommands)).(*localtools.LocalExecTools)
	toolsCall := &taskengine.ToolsCall{
		Name: "local_shell",
		Args: map[string]string{
			"command": "exit 3",
			"shell":   "true",
		},
	}
	_, _, err := h.Exec(ctx, time.Now().UTC(), nil, false, toolsCall)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strictly forbidden")
}

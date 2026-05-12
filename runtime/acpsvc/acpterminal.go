package acpsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/runtimetypes"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
)

// ACPTerminalToolsName is the tool-set name registered in the engine's local tools map.
const ACPTerminalToolsName = "acp_terminal"

// ACPTerminalResult is the structured result returned to the LLM after command execution.
type ACPTerminalResult struct {
	ExitCode  *int   `json:"exit_code"`
	Signal    string `json:"signal,omitempty"`
	Output    string `json:"output"`
	Truncated bool   `json:"truncated"`
	Command   string `json:"command"`
}

// ACPTerminalTools exposes the ACP terminal/* methods as tools the LLM can
// invoke. The actual command execution happens inside the client (editor) —
// Contenox sends JSON-RPC requests over the ACP connection and the editor
// creates, runs, and manages the terminal process.
//
// The tool presents a single "exec" function to the LLM that orchestrates the
// full lifecycle: terminal/create → terminal/wait_for_exit → terminal/output →
// terminal/release. This keeps the LLM interface simple while following the
// spec's lifecycle requirements.
type ACPTerminalTools struct {
	transport func() *Transport
}

// NewACPTerminalTools creates a new ToolsRepo that routes command execution to
// the ACP client's terminal subsystem.
func NewACPTerminalTools(transport func() *Transport) taskengine.ToolsRepo {
	return &ACPTerminalTools{transport: transport}
}

// Supports implements taskengine.ToolsRepo.
func (a *ACPTerminalTools) Supports(_ context.Context) ([]string, error) {
	return []string{ACPTerminalToolsName}, nil
}

// GetSchemasForSupportedTools implements taskengine.ToolsRepo.
func (a *ACPTerminalTools) GetSchemasForSupportedTools(_ context.Context) (map[string]*openapi3.T, error) {
	return map[string]*openapi3.T{}, nil
}

// GetToolsForToolsByName implements taskengine.ToolsRepo. Only advertises the
// tool when the client supports terminals (checked via clientCaps.Terminal).
func (a *ACPTerminalTools) GetToolsForToolsByName(_ context.Context, name string) ([]taskengine.Tool, error) {
	if name != ACPTerminalToolsName {
		return nil, fmt.Errorf("unknown tools: %s", name)
	}

	t := a.transport()
	if t == nil {
		return nil, taskengine.ErrToolsNotFound
	}
	caps := t.getClientCaps()
	if !caps.Terminal {
		return nil, taskengine.ErrToolsNotFound
	}

	return []taskengine.Tool{
		{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "exec",
				Description: "Execute a shell command in the editor's terminal. The command runs in a real terminal in the user's environment. Output is captured and returned. Use this for running tests, builds, git operations, linters, etc.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "The command to execute (e.g. 'npm', 'git', 'cargo')",
						},
						"args": map[string]interface{}{
							"type": "array",
							"items": map[string]interface{}{
								"type": "string",
							},
							"description": "Array of command arguments (e.g. [\"test\", \"--coverage\"])",
						},
						"cwd": map[string]interface{}{
							"type":        "string",
							"description": "Working directory for the command (absolute path). Defaults to the session's working directory.",
						},
						"timeout": map[string]interface{}{
							"type":        "string",
							"description": "Maximum duration before killing the command (e.g. '30s', '2m'). Default: 60s.",
						},
					},
					"required": []string{"command"},
				},
			},
		},
	}, nil
}

// Exec implements taskengine.ToolsRepo.
func (a *ACPTerminalTools) Exec(ctx context.Context, _ time.Time, input any, _ bool, toolsCall *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	if toolsCall == nil {
		return nil, taskengine.DataTypeAny, errors.New("acp_terminal: tools call required")
	}

	args, ok := input.(map[string]any)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("acp_terminal: input must be a map")
	}

	t := a.transport()
	if t == nil {
		return nil, taskengine.DataTypeAny, errors.New("acp_terminal: no active ACP transport")
	}

	toolName := toolsCall.ToolName
	if toolName == "" {
		toolName = toolsCall.Name
	}

	switch toolName {
	case "exec":
		return a.exec(ctx, t, args)
	default:
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_terminal: unknown tool %s", toolName)
	}
}

const defaultTerminalTimeout = 60 * time.Second

var terminalOutputByteLimit int64 = 1 * 1024 * 1024 // 1 MiB

func (a *ACPTerminalTools) exec(ctx context.Context, t *Transport, args map[string]any) (any, taskengine.DataType, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return nil, taskengine.DataTypeAny, errors.New("acp_terminal.exec: command is required")
	}

	// Build the create request.
	req := libacp.CreateTerminalRequest{
		Command:         command,
		OutputByteLimit: &terminalOutputByteLimit,
	}

	// Resolve ACP session ID.
	if sid := resolveACPSessionID(ctx, t); sid != "" {
		req.SessionID = sid
	}

	// Parse args array.
	if rawArgs, ok := args["args"]; ok {
		switch v := rawArgs.(type) {
		case []any:
			for _, a := range v {
				if s, ok := a.(string); ok {
					req.Args = append(req.Args, s)
				}
			}
		case []string:
			req.Args = v
		}
	}

	// Cwd: use provided or fall back to session cwd.
	if cwd, ok := args["cwd"].(string); ok && cwd != "" {
		req.Cwd = cwd
	} else if sid := resolveACPSessionID(ctx, t); sid != "" {
		t.sessionMu.Lock()
		for _, entry := range t.sessions {
			if entry.InternalSessionID == sessionIDFromCtx(ctx) && entry.Cwd != "" {
				req.Cwd = entry.Cwd
				break
			}
		}
		t.sessionMu.Unlock()
	}

	// Parse timeout.
	timeout := defaultTerminalTimeout
	if timeoutStr, ok := args["timeout"].(string); ok && timeoutStr != "" {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	// 1. Create terminal.
	createResp, err := t.conn.CreateTerminal(ctx, req)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_terminal.exec: create: %w", err)
	}
	termID := createResp.TerminalID

	// Ensure cleanup: always release the terminal.
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = t.conn.ReleaseTerminal(releaseCtx, libacp.ReleaseTerminalRequest{
			SessionID:  req.SessionID,
			TerminalID: termID,
		})
	}()

	// 2. Wait for exit with timeout.
	waitCtx, waitCancel := context.WithTimeout(ctx, timeout)
	defer waitCancel()

	exitResp, waitErr := t.conn.WaitForTerminalExit(waitCtx, libacp.WaitForTerminalExitRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	})

	timedOut := false
	if waitErr != nil && waitCtx.Err() != nil && ctx.Err() == nil {
		// Timeout expired, not parent cancellation. Kill the process.
		timedOut = true
		killCtx, killCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer killCancel()
		_, _ = t.conn.KillTerminal(killCtx, libacp.KillTerminalRequest{
			SessionID:  req.SessionID,
			TerminalID: termID,
		})
	} else if waitErr != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_terminal.exec: wait: %w", waitErr)
	}

	// 3. Get output.
	outputResp, err := t.conn.TerminalOutput(ctx, libacp.TerminalOutputRequest{
		SessionID:  req.SessionID,
		TerminalID: termID,
	})
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_terminal.exec: output: %w", err)
	}

	result := ACPTerminalResult{
		Command:   command,
		Output:    outputResp.Output,
		Truncated: outputResp.Truncated,
	}

	if timedOut {
		result.Output += "\n[command killed: timeout exceeded]"
		minusOne := -1
		result.ExitCode = &minusOne
	} else {
		result.ExitCode = exitResp.ExitCode
		if exitResp.Signal != nil {
			result.Signal = *exitResp.Signal
		}
	}

	b, err := json.Marshal(result)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_terminal.exec: marshal: %w", err)
	}
	return string(b), taskengine.DataTypeString, nil
}

// resolveACPSessionID maps the contenox session ID from context to the ACP session ID.
func resolveACPSessionID(ctx context.Context, t *Transport) libacp.SessionID {
	contenoxSessionID := sessionIDFromCtx(ctx)
	if contenoxSessionID == "" {
		return ""
	}
	acpSID, _ := t.acpSessionForContenoxID(contenoxSessionID)
	return acpSID
}

func sessionIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(runtimetypes.SessionIDContextKey).(string)
	return v
}

// Compile-time assertion.
var _ taskengine.ToolsRepo = (*ACPTerminalTools)(nil)

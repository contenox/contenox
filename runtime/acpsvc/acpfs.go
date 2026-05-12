package acpsvc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/taskengine"
	"github.com/getkin/kin-openapi/openapi3"
)

// ACPFSToolsName is the tool-set name registered in the engine's local tools map.
const ACPFSToolsName = "acp_fs"

// ACPFSTools exposes the ACP fs/read_text_file and fs/write_text_file methods
// as tools the LLM can invoke. The actual I/O happens inside the client (editor)
// — Contenox sends a JSON-RPC request over the ACP connection and the editor
// returns the file content (including unsaved buffer state) or performs the write.
//
// This follows the same ToolsRepo pattern as EchoTools, LocalFSTools, etc.
type ACPFSTools struct {
	// transport returns the active Transport. It is a closure because the
	// transport is not available until the first ACP connection is established
	// (same pattern as the HITL askApproval closure in acp_cmd.go).
	transport func() *Transport
}

// NewACPFSTools creates a new ToolsRepo that routes file operations to the
// ACP client. transport must return the active Transport or nil.
func NewACPFSTools(transport func() *Transport) taskengine.ToolsRepo {
	return &ACPFSTools{transport: transport}
}

// Supports implements taskengine.ToolsRepo.
func (a *ACPFSTools) Supports(_ context.Context) ([]string, error) {
	return []string{ACPFSToolsName}, nil
}

// GetSchemasForSupportedTools implements taskengine.ToolsRepo.
func (a *ACPFSTools) GetSchemasForSupportedTools(_ context.Context) (map[string]*openapi3.T, error) {
	return map[string]*openapi3.T{}, nil
}

// GetToolsForToolsByName implements taskengine.ToolsRepo. Only advertises tools
// the client actually supports (checked via clientCaps from the initialize handshake).
func (a *ACPFSTools) GetToolsForToolsByName(_ context.Context, name string) ([]taskengine.Tool, error) {
	if name != ACPFSToolsName {
		return nil, fmt.Errorf("unknown tools: %s", name)
	}

	t := a.transport()
	if t == nil {
		return nil, taskengine.ErrToolsNotFound
	}
	caps := t.getClientCaps()

	var tools []taskengine.Tool

	if caps.FS.ReadTextFile {
		tools = append(tools, taskengine.Tool{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "read_file",
				Description: "Read a text file from the editor's filesystem. Returns the file content including any unsaved changes in the editor buffer. Use absolute paths.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Absolute path to the file to read",
						},
						"line": map[string]interface{}{
							"type":        "integer",
							"description": "Optional 1-based line number to start reading from",
						},
						"limit": map[string]interface{}{
							"type":        "integer",
							"description": "Optional maximum number of lines to read",
						},
					},
					"required": []string{"path"},
				},
			},
		})
	}

	if caps.FS.WriteTextFile {
		tools = append(tools, taskengine.Tool{
			Type: "function",
			Function: taskengine.FunctionTool{
				Name:        "write_file",
				Description: "Write or create a text file via the editor's filesystem. The editor tracks the modification for undo/diff. Use absolute paths.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Absolute path to the file to write. The file is created if it doesn't exist.",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "The full text content to write to the file",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		})
	}

	if len(tools) == 0 {
		return nil, taskengine.ErrToolsNotFound
	}
	return tools, nil
}

// Exec implements taskengine.ToolsRepo.
func (a *ACPFSTools) Exec(ctx context.Context, _ time.Time, input any, _ bool, toolsCall *taskengine.ToolsCall) (any, taskengine.DataType, error) {
	if toolsCall == nil {
		return nil, taskengine.DataTypeAny, errors.New("acp_fs: tools call required")
	}

	args, ok := input.(map[string]any)
	if !ok {
		return nil, taskengine.DataTypeAny, errors.New("acp_fs: input must be a map")
	}

	t := a.transport()
	if t == nil {
		return nil, taskengine.DataTypeAny, errors.New("acp_fs: no active ACP transport")
	}

	toolName := toolsCall.ToolName
	if toolName == "" {
		toolName = toolsCall.Name
	}

	switch toolName {
	case "read_file":
		return a.readFile(ctx, t, args)
	case "write_file":
		return a.writeFile(ctx, t, args)
	default:
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_fs: unknown tool %s", toolName)
	}
}

func (a *ACPFSTools) readFile(ctx context.Context, t *Transport, args map[string]any) (any, taskengine.DataType, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, taskengine.DataTypeAny, errors.New("acp_fs.read_file: path is required")
	}

	req := libacp.ReadTextFileRequest{
		Path: path,
	}

	// Resolve the ACP session ID from the context.
	if sid := resolveACPSessionID(ctx, t); sid != "" {
		req.SessionID = sid
	}

	if line, ok := args["line"]; ok {
		if v := toIntPtr(line); v != nil {
			req.Line = v
		}
	}
	if limit, ok := args["limit"]; ok {
		if v := toIntPtr(limit); v != nil {
			req.Limit = v
		}
	}

	resp, err := t.conn.ReadTextFile(ctx, req)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_fs.read_file: %w", err)
	}
	return resp.Content, taskengine.DataTypeString, nil
}

func (a *ACPFSTools) writeFile(ctx context.Context, t *Transport, args map[string]any) (any, taskengine.DataType, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return nil, taskengine.DataTypeAny, errors.New("acp_fs.write_file: path is required")
	}
	content, _ := args["content"].(string)

	req := libacp.WriteTextFileRequest{
		Path:    path,
		Content: content,
	}

	if sid := resolveACPSessionID(ctx, t); sid != "" {
		req.SessionID = sid
	}

	_, err := t.conn.WriteTextFile(ctx, req)
	if err != nil {
		return nil, taskengine.DataTypeAny, fmt.Errorf("acp_fs.write_file: %w", err)
	}
	return fmt.Sprintf("wrote %s (%d bytes)", path, len(content)), taskengine.DataTypeString, nil
}

// toIntPtr converts a JSON-decoded number (float64 or int) to *int.
func toIntPtr(v any) *int {
	switch n := v.(type) {
	case float64:
		i := int(n)
		return &i
	case int:
		return &n
	case int64:
		i := int(n)
		return &i
	}
	return nil
}

// Compile-time assertion.
var _ taskengine.ToolsRepo = (*ACPFSTools)(nil)

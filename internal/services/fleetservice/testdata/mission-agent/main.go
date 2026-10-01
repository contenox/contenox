package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/contenox/contenox/libacp"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type stdio struct{}

func (stdio) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
func (stdio) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (stdio) Close() error                { return nil }

type agent struct {
	libacp.UnimplementedAgent
	tools *mcp.ClientSession
}

func (a *agent) Initialize(context.Context, libacp.InitializeRequest) (libacp.InitializeResponse, error) {
	return libacp.InitializeResponse{ProtocolVersion: libacp.ProtocolVersion}, nil
}

func (a *agent) NewSession(ctx context.Context, req libacp.NewSessionRequest) (libacp.NewSessionResponse, error) {
	if len(req.McpServers) != 1 {
		return libacp.NewSessionResponse{}, fmt.Errorf("expected mission MCP server, got %d", len(req.McpServers))
	}
	s := req.McpServers[0]
	cmd := exec.Command(s.Command, s.Args...)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr
	for _, env := range s.Env {
		cmd.Env = append(cmd.Env, env.Name+"="+env.Value)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "external-mission-test", Version: "1"}, nil)
	var err error
	a.tools, err = client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return libacp.NewSessionResponse{}, err
	}
	listed, err := a.tools.ListTools(ctx, nil)
	if err != nil || len(listed.Tools) != 4 {
		return libacp.NewSessionResponse{}, fmt.Errorf("mission tools not available: %v", err)
	}
	return libacp.NewSessionResponse{SessionID: "external-mission"}, nil
}

func (a *agent) Prompt(ctx context.Context, _ libacp.PromptRequest) (libacp.PromptResponse, error) {
	if _, err := os.ReadFile(os.Getenv("MISSION_TEST_FORBIDDEN_PATH")); !os.IsPermission(err) {
		return libacp.PromptResponse{}, fmt.Errorf("outside-workspace read must remain denied, got %v", err)
	}
	for _, call := range []mcp.CallToolParams{
		{Name: "mission_report", Arguments: map[string]any{"kind": "progress", "summary": "external agent reporting via MCP"}},
		{Name: "mission_ask_attention", Arguments: map[string]any{"summary": "May I finish?"}},
		{Name: "mission_plan", Arguments: map[string]any{"entries": []map[string]any{{"content": "Verify the bridge", "status": "completed", "priority": "high"}}}},
		{Name: "mission_finish", Arguments: map[string]any{"status": "landed", "reason": "external MCP tools worked"}},
	} {
		result, err := a.tools.CallTool(ctx, &call)
		if err != nil {
			return libacp.PromptResponse{}, err
		}
		if result.IsError {
			return libacp.PromptResponse{}, fmt.Errorf("%s failed: %v", call.Name, result.Content)
		}
	}
	return libacp.PromptResponse{StopReason: libacp.StopReasonEndTurn}, nil
}

func main() {
	a := &agent{}
	conn := libacp.NewAgentSideConnection(stdio{}, func(*libacp.AgentSideConnection) libacp.Agent { return a })
	if err := conn.Run(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

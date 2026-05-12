package acpsvc

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/taskengine"
)

func TestACPTerminalTools_Supports(t *testing.T) {
	term := NewACPTerminalTools(func() *Transport { return nil })
	names, err := term.Supports(context.Background())
	if err != nil {
		t.Fatalf("Supports: %v", err)
	}
	if len(names) != 1 || names[0] != ACPTerminalToolsName {
		t.Fatalf("got %v, want [%s]", names, ACPTerminalToolsName)
	}
}

func TestACPTerminalTools_GetTools_TerminalCapability(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{}) // no fs caps
	tr.clientCaps.Terminal = true
	term := NewACPTerminalTools(func() *Transport { return tr })

	tools, err := term.GetToolsForToolsByName(context.Background(), ACPTerminalToolsName)
	if err != nil {
		t.Fatalf("GetToolsForToolsByName: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Function.Name != "exec" {
		t.Errorf("expected exec, got %s", tools[0].Function.Name)
	}
}

func TestACPTerminalTools_GetTools_NoCaps(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{})
	// tr.clientCaps.Terminal is false by default
	term := NewACPTerminalTools(func() *Transport { return tr })

	_, err := term.GetToolsForToolsByName(context.Background(), ACPTerminalToolsName)
	if err == nil {
		t.Fatal("expected ErrToolsNotFound when terminal cap is false")
	}
}

func TestACPTerminalTools_GetTools_NilTransport(t *testing.T) {
	term := NewACPTerminalTools(func() *Transport { return nil })
	_, err := term.GetToolsForToolsByName(context.Background(), ACPTerminalToolsName)
	if err == nil {
		t.Fatal("expected error when transport is nil")
	}
}

func TestACPTerminalTools_Exec_NilTransport(t *testing.T) {
	term := NewACPTerminalTools(func() *Transport { return nil })
	_, _, err := term.Exec(context.Background(), time.Now(), map[string]any{"command": "ls"}, false, &taskengine.ToolsCall{
		Name:     ACPTerminalToolsName,
		ToolName: "exec",
	})
	if err == nil {
		t.Fatal("expected error when transport is nil")
	}
}

func TestACPTerminalTools_Exec_MissingCommand(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{})
	tr.clientCaps.Terminal = true
	term := NewACPTerminalTools(func() *Transport { return tr })
	_, _, err := term.Exec(context.Background(), time.Now(), map[string]any{}, false, &taskengine.ToolsCall{
		Name:     ACPTerminalToolsName,
		ToolName: "exec",
	})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestACPTerminalTools_Exec_UnknownTool(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{})
	tr.clientCaps.Terminal = true
	term := NewACPTerminalTools(func() *Transport { return tr })
	_, _, err := term.Exec(context.Background(), time.Now(), map[string]any{}, false, &taskengine.ToolsCall{
		Name:     ACPTerminalToolsName,
		ToolName: "run_background",
	})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

package acpsvc

import (
	"context"
	"testing"
	"time"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/taskengine"
)

// mockTransportForFS creates a Transport with pre-set client capabilities for
// testing ACPFSTools. No real connection is needed because the tests only call
// methods that read in-memory state.
func mockTransportForFS(caps libacp.FileSystemCapabilities) *Transport {
	t := &Transport{
		sessions:        make(map[libacp.SessionID]*sessionEntry),
		contenoxToACPID: make(map[string]libacp.SessionID),
	}
	t.clientCaps = libacp.ClientCapabilities{FS: caps}
	return t
}

func TestUnit_ACPFSTools_Supports(t *testing.T) {
	fs := NewACPFSTools(func() *Transport { return nil })
	names, err := fs.Supports(context.Background())
	if err != nil {
		t.Fatalf("Supports: %v", err)
	}
	if len(names) != 1 || names[0] != ACPFSToolsName {
		t.Fatalf("got %v, want [%s]", names, ACPFSToolsName)
	}
}

func TestUnit_ACPFSTools_GetTools_BothCapabilities(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{
		ReadTextFile:  true,
		WriteTextFile: true,
	})
	fs := NewACPFSTools(func() *Transport { return tr })

	tools, err := fs.GetToolsForToolsByName(context.Background(), ACPFSToolsName)
	if err != nil {
		t.Fatalf("GetToolsForToolsByName: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Function.Name] = true
	}
	if !names["read_file"] {
		t.Error("missing read_file tool")
	}
	if !names["write_file"] {
		t.Error("missing write_file tool")
	}
}

func TestUnit_ACPFSTools_GetTools_ReadOnly(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{
		ReadTextFile:  true,
		WriteTextFile: false,
	})
	fs := NewACPFSTools(func() *Transport { return tr })

	tools, err := fs.GetToolsForToolsByName(context.Background(), ACPFSToolsName)
	if err != nil {
		t.Fatalf("GetToolsForToolsByName: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0].Function.Name != "read_file" {
		t.Errorf("expected read_file, got %s", tools[0].Function.Name)
	}
}

func TestUnit_ACPFSTools_GetTools_NoCaps(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{})
	fs := NewACPFSTools(func() *Transport { return tr })

	_, err := fs.GetToolsForToolsByName(context.Background(), ACPFSToolsName)
	if err == nil {
		t.Fatal("expected ErrToolsNotFound when no fs caps")
	}
}

func TestUnit_ACPFSTools_GetTools_NilTransport(t *testing.T) {
	fs := NewACPFSTools(func() *Transport { return nil })
	_, err := fs.GetToolsForToolsByName(context.Background(), ACPFSToolsName)
	if err == nil {
		t.Fatal("expected error when transport is nil")
	}
}

func TestUnit_ACPFSTools_Exec_NilTransport(t *testing.T) {
	fs := NewACPFSTools(func() *Transport { return nil })
	_, _, err := fs.Exec(context.Background(), time.Now(), map[string]any{"path": "/test"}, false, &taskengine.ToolsCall{
		Name:     ACPFSToolsName,
		ToolName: "read_file",
	})
	if err == nil {
		t.Fatal("expected error when transport is nil")
	}
}

func TestUnit_ACPFSTools_Exec_UnknownTool(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{ReadTextFile: true})
	fs := NewACPFSTools(func() *Transport { return tr })
	_, _, err := fs.Exec(context.Background(), time.Now(), map[string]any{}, false, &taskengine.ToolsCall{
		Name:     ACPFSToolsName,
		ToolName: "delete_file",
	})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestUnit_ACPFSTools_Exec_MissingPath(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{ReadTextFile: true})
	fs := NewACPFSTools(func() *Transport { return tr })
	_, _, err := fs.Exec(context.Background(), time.Now(), map[string]any{}, false, &taskengine.ToolsCall{
		Name:     ACPFSToolsName,
		ToolName: "read_file",
	})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestUnit_ACPFSTools_Exec_WriteMissingPath(t *testing.T) {
	tr := mockTransportForFS(libacp.FileSystemCapabilities{WriteTextFile: true})
	fs := NewACPFSTools(func() *Transport { return tr })
	_, _, err := fs.Exec(context.Background(), time.Now(), map[string]any{"content": "hello"}, false, &taskengine.ToolsCall{
		Name:     ACPFSToolsName,
		ToolName: "write_file",
	})
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestToIntPtr(t *testing.T) {
	cases := []struct {
		input any
		want  *int
	}{
		{float64(10), intPtr(10)},
		{int(5), intPtr(5)},
		{int64(20), intPtr(20)},
		{"nope", nil},
		{nil, nil},
	}
	for _, c := range cases {
		got := toIntPtr(c.input)
		if c.want == nil && got != nil {
			t.Errorf("toIntPtr(%v) = %d, want nil", c.input, *got)
		} else if c.want != nil && (got == nil || *got != *c.want) {
			t.Errorf("toIntPtr(%v) = %v, want %d", c.input, got, *c.want)
		}
	}
}

func intPtr(n int) *int { return &n }

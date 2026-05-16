package acpsvc

import (
	"bytes"
	"context"
	"testing"

	"github.com/contenox/contenox/libacp"
	"github.com/contenox/contenox/runtime/localtools"
)

func TestUnit_ACPCommandRunner_FallsBackToOSWhenClientLacksTerminalCapability(t *testing.T) {
	t.Parallel()
	tr := mockTransportForFS(libacp.FileSystemCapabilities{})
	runner := NewACPCommandRunner(func() *Transport { return tr })

	var stdout, stderr bytes.Buffer
	exitCode, err := runner.Run(context.Background(),
		localtools.CommandSpec{Command: "printf", Args: []string{"hello"}},
		&stdout, &stderr)

	if err != nil {
		t.Fatalf("os fallback must run the command, got %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit 0 from os fallback, got %d", exitCode)
	}
	if stdout.String() != "hello" {
		t.Fatalf("expected os fallback stdout %q, got %q", "hello", stdout.String())
	}
}

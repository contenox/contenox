package modeldprobe

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/contenox/contenox/internal/modeldinstall"
)

func TestUnit_ManagedDaemonDiscovery(t *testing.T) {
	root := t.TempDir()
	installed := filepath.Join(root, "installed")
	if err := os.MkdirAll(installed, 0700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(installed, modeldinstall.LauncherName(runtime.GOOS))
	if err := os.WriteFile(launcher, []byte("launcher"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := modeldinstall.WriteCurrentPointer(root, installed); err != nil {
		t.Fatal(err)
	}
	detector := New(root)
	detector.binaryOverride = ""
	detector.lookPath = func(string) (string, error) { t.Fatal("managed install should precede PATH"); return "", nil }
	if got := detector.Detect().Binary; got != launcher {
		t.Fatalf("binary=%q want %q", got, launcher)
	}
	detector.binaryOverride = filepath.Join(root, "missing")
	if got := detector.Detect().Binary; got != "" {
		t.Fatalf("invalid explicit override fell through: %q", got)
	}
	args := detector.LauncherArgs("contenox-modeld.exe")
	if len(args) != 3 || args[0] != "serve" || args[1] != "--data-root" || args[2] != root {
		t.Fatalf("args=%v", args)
	}
}

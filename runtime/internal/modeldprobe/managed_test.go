package modeldprobe

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/contenox/runtime/runtime/internal/modeldinstall"
	"github.com/contenox/runtime/runtime/version"
)

// A Contenox-managed install (what `contenox setup` downloads) is located ahead
// of PATH, without CONTENOX_MODELD_BIN set.
func TestLocate_ManagedInstall(t *testing.T) {
	v := version.Get()
	if !modeldinstall.IsOfficialVersion(v) {
		t.Skipf("dev build (version %q) installs no managed package", v)
	}
	dataRoot := t.TempDir()
	managed := modeldinstall.ManagedLauncherPath(dataRoot, v, runtime.GOOS, runtime.GOARCH)

	d := &Detector{
		managedBinary: managed,
		statBinary:    func(p string) bool { return p == managed },
		lookPath:      func(string) (string, error) { return "/usr/bin/modeld", nil }, // must be ignored
	}
	if got := d.locate(); got != managed {
		t.Fatalf("locate = %q, want managed path %q", got, managed)
	}
}

// When the managed install is absent, discovery falls through to PATH.
func TestLocate_ManagedMissingFallsBackToPATH(t *testing.T) {
	managed := filepath.Join(t.TempDir(), "modeld", "v1.0.0", "linux-amd64", "modeld")
	d := &Detector{
		managedBinary: managed,
		statBinary:    func(string) bool { return false },
		lookPath:      func(string) (string, error) { return "/usr/bin/modeld", nil },
	}
	if got := d.locate(); got != "/usr/bin/modeld" {
		t.Fatalf("locate = %q, want PATH fallback /usr/bin/modeld", got)
	}
}

// New derives the managed path from the CLI version for an official release.
func TestNew_DerivesManagedBinary(t *testing.T) {
	v := version.Get()
	dataRoot := t.TempDir()
	d := New(dataRoot)
	if modeldinstall.IsOfficialVersion(v) {
		want := modeldinstall.ManagedLauncherPath(dataRoot, v, runtime.GOOS, runtime.GOARCH)
		if d.managedBinary != want {
			t.Fatalf("managedBinary = %q, want %q", d.managedBinary, want)
		}
	} else if d.managedBinary != "" {
		t.Fatalf("dev build derived a managed path %q, want empty", d.managedBinary)
	}
}

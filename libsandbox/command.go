package libsandbox

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/contenox/runtime/libtracker"
)

// Command assembles the confined command for the agent named by name (with
// args), pinned to the workspace and wrapped in the wall described by spec. It
// validates the spec, pins the working directory to spec.WorkspaceRoot, sets a
// scrubbed minimal environment (see scrubEnv) with HOME forced to spec.Home, and
// applies the platform's isolation before returning the ready-to-run *exec.Cmd.
//
// It does not start the process, and — deliberately — does not bind its lifetime
// to ctx: the returned command is handed to whatever runs and supervises it
// (e.g. libprocess or acpexec.Spawn), which owns start, stop, and teardown, just
// as the existing external-agent spawn builds a bare exec.Command and lets its
// runner own the lifetime. ctx scopes the assembly instead: it drives the
// ActivityTracker lifecycle and is available to the isolation seam for setup
// that may need cancellation or a deadline in a later slice.
//
// The env-scrub and Dir pinning happen on every platform — that credential-leak
// fix is portable and applied here and now. The deny-by-construction wall (fs,
// net, process) is applied by applyIsolation, which is a documented no-op in
// this slice and on non-Linux hosts; see the isolation_*.go seam. A minimal
// valid spec (a workspace, a home, no carve-outs) therefore assembles cleanly.
//
// Errors wrap ErrInvalidSpec (missing name/workspace/home) or ErrInvalidCarveout
// (a malformed hole), and are also reported to spec.Tracker.
func Command(ctx context.Context, spec Spec, name string, args ...string) (*exec.Cmd, error) {
	tracker := spec.Tracker
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	reportErr, reportChange, end := tracker.Start(ctx, "confine", "sandbox",
		"command", name, "workspace", spec.WorkspaceRoot)
	defer end()

	if strings.TrimSpace(name) == "" {
		err := fmt.Errorf("%w: name (the executable to confine) is required", ErrInvalidSpec)
		reportErr(err)
		return nil, err
	}
	if err := spec.validate(); err != nil {
		reportErr(err)
		return nil, err
	}

	cmd := exec.Command(name, args...)
	cmd.Dir = spec.WorkspaceRoot
	cmd.Env = scrubEnv(os.Environ(), spec.EnvAllow, spec.EnvSet, spec.Home)

	if err := applyIsolation(ctx, cmd, spec, tracker); err != nil {
		err = fmt.Errorf("libsandbox: apply isolation for %q: %w", name, err)
		reportErr(err)
		return nil, err
	}

	// The change carries the shape of the confinement, never any secret value:
	// counts and the pinned dir, not the scrubbed variables themselves.
	reportChange(name, map[string]any{
		"dir":  cmd.Dir,
		"args": len(args),
		"fs":   len(spec.FS),
		"net":  len(spec.Net),
		"env":  len(cmd.Env),
	})
	return cmd, nil
}

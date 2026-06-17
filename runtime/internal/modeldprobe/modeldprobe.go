// Package modeldprobe detects whether the modeld daemon (the separate CGO
// inference binary) is installed, running, or dead, so the runtime can fail
// honestly and the setup wizard can guide the user.
//
// It is pure Go and self-contained: it locates the binary and inspects the
// owner lease via liblease. It deliberately does not import modeld — the only
// shared facts are the lease file name and the endpoint metadata key, mirrored
// here as constants (see cmd/modeld and modeld/owner.EndpointMetaKey).
//
// Liveness via lease freshness is a proxy, not a health check: a wedged process
// can still hold a fresh lease. A real reachability ping rides on the IPC
// transport and is added when that exists. See
// docs/blueprints/modeld-provisioning-detection.md.
package modeldprobe

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/contenox/runtime/liblease"
)

// Convention shared with the daemon; kept local so runtime does not depend on
// the modeld package.
const (
	leaseFileName   = "modeld.lease" // mirrors cmd/modeld resolveLeasePath
	endpointMetaKey = "endpoint"     // mirrors modeld/owner.EndpointMetaKey
	binaryName      = "modeld"
	binaryEnv       = "CONTENOX_MODELD_BIN" // explicit override (VS Code/extension installs)
	dataRootEnv     = "CONTENOX_DATA_ROOT"
)

// State is the detected condition of the modeld daemon.
type State int

const (
	// StateNotInstalled means no modeld binary could be located.
	StateNotInstalled State = iota
	// StateNotRunning means the binary exists but no owner lease is present.
	StateNotRunning
	// StateStale means an owner lease exists but has expired: the daemon
	// likely crashed or was killed.
	StateStale
	// StateRunning means a live owner holds a fresh lease.
	StateRunning
)

func (s State) String() string {
	switch s {
	case StateNotInstalled:
		return "not-installed"
	case StateNotRunning:
		return "not-running"
	case StateStale:
		return "stale"
	case StateRunning:
		return "running"
	default:
		return "unknown"
	}
}

// Typed, actionable errors. Status.Err returns one of these (or nil) so callers
// can branch with errors.Is.
var (
	ErrNotInstalled = errors.New("modeld is not installed")
	ErrNotRunning   = errors.New("modeld is not running")
	ErrStale        = errors.New("modeld owner is stale (the daemon may have crashed)")
)

// Status is the result of a detection.
type Status struct {
	State    State
	Binary   string // resolved binary path, when located
	Endpoint string // advertised IPC endpoint, when running
	Instance string // owner instance UUID, when a lease is present
}

// Err maps the status to a typed error, or nil when the daemon is running. A nil
// error means install + liveness passed; it does NOT by itself mean inference
// works, because the wire transport is a separate concern.
func (s Status) Err() error {
	switch s.State {
	case StateNotInstalled:
		return ErrNotInstalled
	case StateNotRunning:
		return ErrNotRunning
	case StateStale:
		return ErrStale
	default:
		return nil
	}
}

// Detector resolves the modeld state. Construct it with New; the now and
// lookPath fields are injectable for tests.
type Detector struct {
	leasePath      string
	binaryOverride string
	lookPath       func(string) (string, error)
	statBinary     func(string) bool
	now            func() time.Time
}

// New returns a Detector for the given data root (where the owner lease lives).
// An empty dataRoot falls back to DefaultDataRoot.
func New(dataRoot string) *Detector {
	if dataRoot == "" {
		dataRoot = DefaultDataRoot()
	}
	return &Detector{
		leasePath:      filepath.Join(dataRoot, leaseFileName),
		binaryOverride: os.Getenv(binaryEnv),
		lookPath:       exec.LookPath,
		statBinary:     fileExists,
		now:            time.Now,
	}
}

// Detect resolves the current modeld state. A live lease takes precedence (the
// daemon is running regardless of whether we can locate the binary locally,
// e.g. when an extension started it from its own directory); binary presence
// then distinguishes stale/not-running from not-installed.
func (d *Detector) Detect() Status {
	rec, leaseErr := liblease.Inspect(d.leasePath)
	hasLease := leaseErr == nil
	live := hasLease && d.now().Before(rec.ExpiresAt())

	binary := d.locate()

	switch {
	case live:
		return Status{
			State:    StateRunning,
			Binary:   binary,
			Endpoint: rec.Meta[endpointMetaKey],
			Instance: rec.InstanceID,
		}
	case binary == "":
		return Status{State: StateNotInstalled}
	case hasLease: // present but expired
		return Status{State: StateStale, Binary: binary, Instance: rec.InstanceID}
	default:
		return Status{State: StateNotRunning, Binary: binary}
	}
}

// locate resolves the modeld binary: an explicit override first (required for
// installs that are not on PATH, like the VS Code extension dir), then PATH.
func (d *Detector) locate() string {
	if d.binaryOverride != "" {
		if d.statBinary(d.binaryOverride) {
			return d.binaryOverride
		}
		return ""
	}
	if p, err := d.lookPath(binaryName); err == nil {
		return p
	}
	return ""
}

// DefaultDataRoot resolves the contenox data root: $CONTENOX_DATA_ROOT, else
// ~/.contenox.
func DefaultDataRoot() string {
	if r := os.Getenv(dataRootEnv); r != "" {
		return r
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".contenox"
	}
	return filepath.Join(home, ".contenox")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

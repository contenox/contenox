package modeldinstall

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"slices"
)

// versionInfo mirrors the subset of `modeld version --json` we validate against.
// The full shape lives in cmd/modeld/version.go; only version + backends matter
// for capability validation (backend_info is ignored here).
type versionInfo struct {
	Version  string   `json:"version"`
	Backends []string `json:"backends"`
}

// probeBinary runs `<launcher> version --json` and parses the report. It does not
// start the daemon or claim the lease, so it is safe to run against a freshly
// extracted (or already installed) binary. A missing launcher is reported so the
// caller can decide to download.
func (c *client) probeBinary(ctx context.Context, launcher string) (versionInfo, error) {
	if !fileExists(launcher) {
		return versionInfo{}, fmt.Errorf("modeld launcher not found: %s", launcher)
	}
	cmd := exec.CommandContext(ctx, launcher, "version", "--json")
	out, err := cmd.Output()
	if err != nil {
		return versionInfo{}, fmt.Errorf("modeld version --json: %w", err)
	}
	var vi versionInfo
	if err := json.Unmarshal(out, &vi); err != nil {
		return versionInfo{}, fmt.Errorf("modeld version --json: parse: %w", err)
	}
	if vi.Version == "" {
		return versionInfo{}, fmt.Errorf("modeld version --json: empty version")
	}
	return vi, nil
}

// checkCapability validates a probed binary for the selected provider: the
// version must match the expected release (an official tag by the time we get
// here) and the compiled backends must include the provider's backend. This is
// compiled-package capability only — not live backend selection, which happens
// inside `modeld serve`.
func checkCapability(info versionInfo, wantVersion, provider string) error {
	if info.Version != wantVersion {
		return fmt.Errorf("modeld setup: version mismatch: binary %s, expected %s", info.Version, wantVersion)
	}
	if slices.Contains(info.Backends, provider) {
		return nil
	}
	return fmt.Errorf("%w: provider %q needs backend %q, binary has %v", ErrBackendMissing, provider, provider, info.Backends)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

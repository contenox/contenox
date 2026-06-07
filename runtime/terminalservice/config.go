package terminalservice

import (
	"fmt"
	"strings"
	"time"
)

// Config holds terminal service settings.
type Config struct {
	Enabled      bool
	AllowedRoot  string
	DefaultShell string
	// IdleTimeout is the maximum time a detached session is kept alive.
	// Zero disables idle reaping.
	IdleTimeout time.Duration
}

const DefaultIdleTimeout = 30 * time.Minute

// IsEnabled returns true for accepted truthy terminal feature flag values.
func IsEnabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	default:
		return false
	}
}

// ParseEnv builds a Config from raw env-backed string fields.
func ParseEnv(terminalEnabled, terminalAllowedRoot, terminalShell, terminalIdleTimeout string) (Config, error) {
	cfg := Config{Enabled: IsEnabled(terminalEnabled)}
	if !cfg.Enabled {
		return cfg, nil
	}
	cfg.AllowedRoot = strings.TrimSpace(terminalAllowedRoot)
	if cfg.AllowedRoot == "" {
		return Config{}, fmt.Errorf("terminalservice: terminal_enabled=true requires terminal_allowed_root")
	}
	cfg.DefaultShell = strings.TrimSpace(terminalShell)
	if cfg.DefaultShell == "" {
		cfg.DefaultShell = "/bin/bash"
	}
	idleStr := strings.TrimSpace(terminalIdleTimeout)
	if idleStr == "" {
		cfg.IdleTimeout = DefaultIdleTimeout
		return cfg, nil
	}
	d, err := time.ParseDuration(idleStr)
	if err != nil || d < 0 {
		return Config{}, fmt.Errorf("terminalservice: invalid terminal_idle_timeout %q", idleStr)
	}
	cfg.IdleTimeout = d
	return cfg, nil
}

package terminalservice

import (
	"fmt"
	"path/filepath"
	"strings"
)

var defaultAllowedShells = map[string]struct{}{
	"/bin/bash":     {},
	"/bin/sh":       {},
	"/usr/bin/bash": {},
	"/usr/bin/sh":   {},
	"/bin/zsh":      {},
	"/usr/bin/zsh":  {},
	"/bin/dash":     {},
	"/usr/bin/dash": {},
}

func ValidateShell(shell string) error {
	shell = filepath.Clean(shell)
	if !filepath.IsAbs(shell) {
		return fmt.Errorf("terminalservice: shell must be an absolute path")
	}
	if _, ok := defaultAllowedShells[shell]; !ok {
		return fmt.Errorf("terminalservice: shell %q is not allowed", shell)
	}
	return nil
}

// CwdUnderRoot ensures cwd is the same as allowedRoot or a subdirectory.
func CwdUnderRoot(allowedRoot, cwd string) error {
	_, err := ResolveCwdUnderRoot(allowedRoot, cwd)
	return err
}

// ResolveCwdUnderRoot returns a cleaned, real cwd after validating it is inside allowedRoot.
func ResolveCwdUnderRoot(allowedRoot, cwd string) (string, error) {
	root, err := filepath.Abs(allowedRoot)
	if err != nil {
		return "", fmt.Errorf("terminalservice: allowed root: %w", err)
	}
	root, err = filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", fmt.Errorf("terminalservice: allowed root: %w", err)
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("terminalservice: cwd: %w", err)
	}
	absCwd, err = filepath.EvalSymlinks(filepath.Clean(absCwd))
	if err != nil {
		return "", fmt.Errorf("terminalservice: cwd: %w", err)
	}
	rel, err := filepath.Rel(root, absCwd)
	if err != nil {
		return "", fmt.Errorf("terminalservice: cwd not under allowed root")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("terminalservice: cwd escapes allowed root")
	}
	return absCwd, nil
}

package contenoxcli

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// serveTokenFileName is the well-known file where `contenox serve` persists its
// bearer token so a repeat serve AND every programmatic client (contenox
// approvals / mission / fleet) share one credential without re-passing TOKEN.
//
// It lives in the GLOBAL ~/.contenox for two reasons:
//   - Discovery: a client does not know a serve's workspace-local `.contenox`
//     (ResolveContenoxDir may pick one), but every client can find ~/.contenox.
//   - Isolation: controlPlaneDirs ALWAYS denies ~/.contenox (see cli.go), so the
//     token file is inside the control-plane deny zone and no session, browse
//     root, or agent fs tool can read it — the same invariant that protects the
//     policy/config the token gates.
const serveTokenFileName = "serve-token.txt"

// serveTokenFilePath is ~/.contenox/serve-token.txt. It ensures ~/.contenox
// exists (globalContenoxDir MkdirAll's it, 0700).
func serveTokenFilePath() (string, error) {
	dir, err := globalContenoxDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, serveTokenFileName), nil
}

// serveTokenPathHint returns the token file path for user-facing messages,
// falling back to the literal path when the home dir can't be resolved.
func serveTokenPathHint() string {
	if p, err := serveTokenFilePath(); err == nil {
		return p
	}
	return filepath.Join("~", ".contenox", serveTokenFileName)
}

// readServeTokenFile returns the trimmed token persisted at serveTokenFilePath,
// or "" when the file is absent, empty, or unreadable.
func readServeTokenFile() string {
	path, err := serveTokenFilePath()
	if err != nil {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// writeServeTokenFile persists token at serveTokenFilePath with 0600 perms
// (owner-only), trailing newline for a friendly `cat`.
func writeServeTokenFile(token string) error {
	path, err := serveTokenFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0o600)
}

// generateServeToken returns a fresh 24-byte (48 hex char) random token.
func generateServeToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

package modeldinstall

import (
	"fmt"
	"path/filepath"
	"strings"
)

// errUnsafePath is returned for any archive entry that would write outside the
// destination. It is intentionally unexported and not a sentinel: unsafe archives
// are a hard failure, never a soft "fall back" case.
func unsafePathErr(name string) error {
	return fmt.Errorf("modeld setup: unsafe archive path %q", name)
}

// safeJoin resolves an archive entry name (forward-slash separated) against dest
// and guarantees the result stays within dest. It rejects empty paths, absolute
// paths, and `..` traversal.
func safeJoin(dest, name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", unsafePathErr(name)
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	if filepath.IsAbs(clean) {
		return "", unsafePathErr(name)
	}
	target := filepath.Join(dest, clean)
	rel, err := filepath.Rel(dest, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", unsafePathErr(name)
	}
	return target, nil
}

// safeLinkTarget validates a symlink: linkDir is the directory containing the
// link, linkname is the (possibly relative) target as stored in the archive. The
// resolved target must stay within dest, and absolute targets are rejected.
func safeLinkTarget(dest, linkDir, linkname string) error {
	if strings.TrimSpace(linkname) == "" {
		return unsafePathErr(linkname)
	}
	ln := filepath.FromSlash(linkname)
	if filepath.IsAbs(ln) {
		return unsafePathErr(linkname)
	}
	resolved := filepath.Join(linkDir, ln)
	rel, err := filepath.Rel(dest, resolved)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return unsafePathErr(linkname)
	}
	return nil
}

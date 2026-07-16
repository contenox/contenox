package vfs

import (
	"fmt"
	"path/filepath"
)

// Factory holds the serve-level allowlist of workspace roots — the set of
// directories a browser client may choose as a session's workspace. Choosing a
// root outside the allowlist is refused. The first root is the default, used
// when a client asks for the sentinel "/" (or nothing), which keeps existing
// clients that always send cwd:"/" working.
type Factory struct {
	// roots is the ordered, de-duplicated, symlink-resolved allowlist. roots[0]
	// is the default.
	roots []string
	// display maps a resolved root back to the path the operator configured, for
	// human-facing labels.
	display map[string]string
}

// NewFactory builds a Factory from an ordered list of roots. The first root is
// the default. Roots are cleaned, made absolute, symlink-resolved, and
// de-duplicated (preserving first-seen order). At least one root is required.
func NewFactory(roots ...string) (*Factory, error) {
	f := &Factory{display: map[string]string{}}
	seen := map[string]struct{}{}
	for _, r := range roots {
		if r == "" {
			continue
		}
		resolved, err := ResolveRoot(r)
		if err != nil {
			return nil, fmt.Errorf("vfs: workspace root %q: %w", r, err)
		}
		if _, dup := seen[resolved]; dup {
			continue
		}
		seen[resolved] = struct{}{}
		f.roots = append(f.roots, resolved)
		if _, ok := f.display[resolved]; !ok {
			f.display[resolved] = filepath.Clean(r)
		}
	}
	if len(f.roots) == 0 {
		return nil, fmt.Errorf("vfs: at least one workspace root is required")
	}
	return f, nil
}

// Roots returns the allowlisted roots in configured order (resolved absolute
// paths). The slice is a copy; callers may not mutate the Factory through it.
func (f *Factory) Roots() []string {
	out := make([]string, len(f.roots))
	copy(out, f.roots)
	return out
}

// Default returns the default root (the first configured), used for the "/" /
// empty sentinel.
func (f *Factory) Default() string {
	if len(f.roots) == 0 {
		return ""
	}
	return f.roots[0]
}

// Resolve maps a requested root to an allowlisted, resolved root. The sentinel
// "/" and the empty string both resolve to the default root — the compat story
// for clients (beam today) that always send cwd:"/". Any other value must
// resolve to a member of the allowlist; otherwise ErrNotAllowed is returned.
func (f *Factory) Resolve(root string) (string, error) {
	if root == "" || root == "/" {
		return f.Default(), nil
	}
	resolved, err := ResolveRoot(root)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotAllowed, err)
	}
	for _, r := range f.roots {
		if r == resolved {
			return r, nil
		}
	}
	return "", fmt.Errorf("%w: %s", ErrNotAllowed, root)
}

// Allows reports whether root resolves to an allowlisted root, returning the
// normalized root when it does.
func (f *Factory) Allows(root string) (string, bool) {
	resolved, err := f.Resolve(root)
	if err != nil {
		return "", false
	}
	return resolved, true
}

// Open returns a View rooted at root, which must be allowlisted (via Resolve
// semantics, so "/" opens the default root).
func (f *Factory) Open(root string) (*View, error) {
	resolved, err := f.Resolve(root)
	if err != nil {
		return nil, err
	}
	return newView(resolved)
}

// View is a root-bound convenience wrapper over Contain. It caches the
// symlink-resolved root so repeated Resolve calls avoid re-walking it.
type View struct {
	root     string
	realRoot string
}

// OpenView returns a View rooted at root, resolving its symlinks. Unlike
// Factory.Open it enforces no allowlist — use it for a single fixed root that is
// trusted by construction (e.g. a serve-owned browse root).
func OpenView(root string) (*View, error) {
	resolved, err := ResolveRoot(root)
	if err != nil {
		return nil, err
	}
	return newView(resolved)
}

// newView builds a View for an already-resolved root.
func newView(resolved string) (*View, error) {
	realRoot, err := ResolveRoot(resolved)
	if err != nil {
		return nil, err
	}
	return &View{root: resolved, realRoot: realRoot}, nil
}

// Root returns the resolved root of this view.
func (v *View) Root() string { return v.root }

// Resolve contains candidate within the view's root (see Contain).
func (v *View) Resolve(candidate string) (string, error) {
	return containWithin(v.realRoot, v.root, candidate)
}

// Contains reports whether an already-absolute path lies within the view root.
func (v *View) Contains(abs string) bool {
	absPath, err := filepath.Abs(abs)
	if err != nil {
		return false
	}
	return within(v.realRoot, filepath.Clean(absPath))
}

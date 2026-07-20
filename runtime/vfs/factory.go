package vfs

import (
	"errors"
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

// ErrCwdNotPermitted is the ONE sentinel every session-cwd refusal wraps (see
// ResolveSessionCwd). Callers translate it into their own transport error —
// apiframework.InvalidParameterValue for the REST surface,
// libacp.ErrInvalidParams for the ACP surface — via errors.Is, and take the
// user-facing text from Error(). The sentinel exists so the DECISION lives here
// while the WIRE SHAPE stays each transport's own concern.
var ErrCwdNotPermitted = errors.New("session cwd is not permitted")

// cwdError carries a refusal message verbatim while still matching
// ErrCwdNotPermitted under errors.Is. A plain fmt.Errorf("%w: ...") would splice
// the sentinel's text into the message every caller forwards to a user; this
// keeps the message exactly what the operator should read.
type cwdError struct{ msg string }

func (e *cwdError) Error() string { return e.msg }
func (e *cwdError) Is(target error) bool {
	return target == ErrCwdNotPermitted
}

func cwdRefusal(format string, args ...any) error {
	return &cwdError{msg: fmt.Sprintf(format, args...)}
}

// ResolveSessionCwd is the ONE decision procedure for "which concrete directory
// does this session run in". Every surface that opens or re-roots a session — the
// ACP session/new, session/load and session/resume paths, and the REST fleet
// dispatch — resolves through here so the workspace-root envelope cannot be
// enforced in one place and skipped in another. Its rules, in order:
//
//  1. A non-empty cwd MUST be absolute. A relative path is refused outright,
//     before any allowlist is consulted, because a relative cwd is meaningless
//     to a session that will run in some other process's working directory —
//     and, with no allowlist configured, would otherwise be adopted verbatim.
//     An empty cwd is not a path but the ABSENCE of one, and falls through to
//     rule 2.
//  2. The sentinel "/" and an empty cwd mean "unspecified". With an allowlist
//     configured they resolve to its default root (the compat story for clients
//     that always send cwd:"/"). With none configured, an empty cwd resolves to
//     fallback — the caller's own default root, "" for a caller that has none.
//  3. With an allowlist configured, any other value must resolve to an
//     allowlisted root; otherwise the request is refused.
//  4. With NO allowlist configured (f nil — the stdio ACP path), an absolute cwd
//     is returned unchanged: there is no server-side envelope to enforce and the
//     editor owns the filesystem.
//
// The nil-allowlist default is deliberately the CALLER's (fallback) rather than a
// value invented here: "unspecified" means different things to a transport that
// has no notion of a project root (ACP stdio, which passes "" and leaves the cwd
// unspecified for the editor to interpret) and to one that does (fleet dispatch,
// which passes its configured project root). One procedure, one parameter, no
// second implementation.
func ResolveSessionCwd(f *Factory, cwd, fallback string) (string, error) {
	if cwd != "" && !filepath.IsAbs(cwd) {
		return "", cwdRefusal("cwd must be an absolute path, got %q", cwd)
	}
	if f == nil {
		if cwd == "" {
			return fallback, nil
		}
		return cwd, nil
	}
	resolved, err := f.Resolve(cwd)
	if err != nil {
		return "", cwdRefusal("workspace directory %q is not permitted; choose one of the configured workspace roots", cwd)
	}
	return resolved, nil
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

package hitlservice

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/contenox/contenox/internal/services/vfs"
)

// PolicySource reads a named HITL policy document for a tenant; any error
// (including not-found) makes the evaluator fall back to the built-in default
// policy.
type PolicySource interface {
	ReadPolicy(ctx context.Context, tenantID, name string) ([]byte, error)
}

// filePolicySource looks a policy up through open handles, nearest first.
type filePolicySource struct{ roots []vfs.Files }

// NewFilesPolicySource returns a PolicySource that reads "<root>/<name>" from
// each handle in order, returning the first hit. It is the form a host holding
// the rendered policies in a store uses: the tenant is already the handle's,
// and the name is looked up beside the declarations that produced it.
func NewFilesPolicySource(roots ...vfs.Files) PolicySource {
	return &filePolicySource{roots: roots}
}

// NewFSPolicySource returns a PolicySource that looks up "<dir>/<name>" in
// each dir in order, returning the first hit; tenantID is ignored and empty
// directories are skipped.
func NewFSPolicySource(dirs ...string) PolicySource {
	roots := make([]vfs.Files, 0, len(dirs))
	for _, dir := range dirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		view, err := vfs.OpenPrivilegedView(dir)
		if err != nil {
			continue
		}
		roots = append(roots, view)
	}
	return &filePolicySource{roots: roots}
}

func (f *filePolicySource) ReadPolicy(ctx context.Context, _, name string) ([]byte, error) {
	file := policyFileName(name)
	var lastErr error = fs.ErrNotExist
	for _, root := range f.roots {
		if root == nil {
			continue
		}
		data, err := root.ReadFile(ctx, file)
		if err == nil {
			return data, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
		lastErr = err
	}
	return nil, fmt.Errorf("hitl policy %q not found: %w", file, lastErr)
}

// policyFileName is idempotent: a bare "strict" becomes "hitl-policy-strict.json";
// a value already ending in .json or carrying a path separator is left verbatim.
func policyFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.HasSuffix(name, ".json") || strings.ContainsAny(name, `/\`) {
		return name
	}
	return "hitl-policy-" + name + ".json"
}

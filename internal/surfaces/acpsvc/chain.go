package acpsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/agentdecl"
	"github.com/contenox/contenox/internal/services/vfs"
)

// SystemDirName is the subdirectory of a contenox directory holding the shipped
// chain files.
const SystemDirName = "system"

const (
	defaultChainFilename = "chain-agent-acp.json"
	chainPathEnv         = "CONTENOX_ACP_CHAIN_PATH"

	defaultFIMChainFilename = "chain-fim-default.json"
	fimChainPathEnv         = "CONTENOX_ACP_FIM_CHAIN_PATH"
)

type ChainRegistry struct {
	defaultChain *taskengine.TaskChainDefinition
	source       string
}

func LoadChainRegistry() (*ChainRegistry, error) {
	return LoadChainRegistryFrom(defaultChainFilename, chainPathEnv)
}

// LoadChainRegistryAt loads a chain from an open root: filename inside fs, with
// source naming it in errors. It is how a host that already holds a handle —
// an account's contenox tree in a store — loads a compiled chain without a
// filesystem path.
func LoadChainRegistryAt(ctx context.Context, fs vfs.Files, filename, source string) (*ChainRegistry, error) {
	if fs == nil {
		return nil, fmt.Errorf("acpsvc: chain %q: no root to read it from", filename)
	}
	if source == "" {
		source = filename
	}
	data, err := fs.ReadFile(ctx, filename)
	if err != nil {
		return nil, fmt.Errorf("acpsvc: chain file %q not found: %w", source, err)
	}
	chain, err := parseChain(data, source)
	if err != nil {
		return nil, err
	}
	return &ChainRegistry{defaultChain: chain, source: source}, nil
}

// ChainFileHandle resolves filename against an open contenox root the way
// ChainSearchPath resolves it against a directory: the operator's own copy
// first, then the compiled declarations, then the shipped system copy. It
// returns the handle over the containing directory and the file's name in it.
func ChainFileHandle(ctx context.Context, contenox vfs.Files, filename string) (vfs.Files, string, bool) {
	if contenox == nil {
		return nil, "", false
	}
	for _, dir := range []string{"", agentdecl.GeneratedDirName, SystemDirName} {
		root := contenox
		if dir != "" {
			sub, err := contenox.Sub(dir)
			if err != nil {
				continue
			}
			root = sub
		}
		if _, err := root.Stat(ctx, filename); err == nil {
			return root, filename, true
		}
	}
	return nil, "", false
}

// parseChain is the one reader both loaders share, so a chain refused for a
// missing id or an empty task list is refused identically from a directory and
// from a store.
func parseChain(data []byte, source string) (*taskengine.TaskChainDefinition, error) {
	var chain taskengine.TaskChainDefinition
	if err := json.Unmarshal(data, &chain); err != nil {
		return nil, fmt.Errorf("acpsvc: invalid chain JSON at %q: %w", source, err)
	}
	if chain.ID == "" {
		return nil, fmt.Errorf("acpsvc: chain at %q has empty ID", source)
	}
	if len(chain.Tasks) == 0 {
		return nil, fmt.Errorf("acpsvc: chain at %q has no tasks", source)
	}
	return &chain, nil
}

// LoadChainRegistryFrom loads the ACP chain for a specific profile: filename is
// the ~/.contenox/ file the chain is read from, envVar overrides that path. A
// missing file is a hard error.
func LoadChainRegistryFrom(filename, envVar string) (*ChainRegistry, error) {
	path := os.Getenv(envVar)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("acpsvc: cannot determine home directory and %s is not set: %w", envVar, err)
		}
		candidates := ChainSearchPath(filepath.Join(home, ".contenox"), filename)
		// Falls back to the last candidate so a total miss names the system copy.
		path = candidates[len(candidates)-1]
		for _, c := range candidates {
			view, vErr := vfs.OpenPrivilegedView(filepath.Dir(c))
			if vErr != nil {
				continue
			}
			if _, statErr := view.Stat(context.Background(), filepath.Base(c)); statErr == nil {
				path = c
				break
			}
		}
	}

	pathView, vErr := vfs.OpenPrivilegedView(filepath.Dir(path))
	if vErr != nil {
		return nil, fmt.Errorf("acpsvc: chain file %q: %w", path, vErr)
	}
	data, err := pathView.ReadFile(context.Background(), filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("acpsvc: chain file %q not found; populate it like any other contenox chain or set %s: %w", path, envVar, err)
	}
	chain, err := parseChain(data, path)
	if err != nil {
		return nil, err
	}
	return &ChainRegistry{defaultChain: chain, source: path}, nil
}

// ChainSearchPath is where LoadChainRegistryFrom looks for filename under
// contenoxDir when the env override is unset, nearest first: an operator copy at
// the top level, then the compiled declarations, then the shipped system copy.
func ChainSearchPath(contenoxDir, filename string) []string {
	return []string{
		filepath.Join(contenoxDir, filename),
		filepath.Join(contenoxDir, agentdecl.GeneratedDirName, filename),
		filepath.Join(contenoxDir, SystemDirName, filename),
	}
}

// ChainFileResolves reports whether any ChainSearchPath candidate exists, which
// is what separates a contenox directory that can serve filename from one a
// caller still has to populate.
func ChainFileResolves(contenoxDir, filename string) bool {
	for _, p := range ChainSearchPath(contenoxDir, filename) {
		view, vErr := vfs.OpenPrivilegedView(filepath.Dir(p))
		if vErr != nil {
			continue
		}
		if _, err := view.Stat(context.Background(), filepath.Base(p)); err == nil {
			return true
		}
	}
	return false
}

func (r *ChainRegistry) Default() *taskengine.TaskChainDefinition { return r.defaultChain }

func (r *ChainRegistry) Source() string { return r.source }

// LoadFIMChainRegistry loads the fill-in-the-middle chain for
// _contenox/autocomplete, mirroring LoadChainRegistry's file and env-var
// convention. A missing or invalid file is a hard error.
func LoadFIMChainRegistry() (*ChainRegistry, error) {
	return LoadChainRegistryFrom(defaultFIMChainFilename, fimChainPathEnv)
}

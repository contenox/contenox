package contenoxcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/surfaces/acpsvc"
	"github.com/contenox/contenox/libbus"

	"github.com/contenox/contenox/internal/services/agentdecl"
	"github.com/contenox/contenox/internal/services/agentregistryservice"
	"github.com/contenox/contenox/internal/services/chainagents"
	"github.com/contenox/contenox/libtracker"
)

// discoverChainAgents runs one chain-agent discovery pass over the workspace
// .contenox/ and ~/.contenox/. Best effort: a failed pass leaves the registry as
// it was, and the outcome is reported via tracker. A nil tracker degrades to Noop.
func discoverChainAgents(ctx context.Context, agents agentregistryservice.Service, contenoxDir string, tracker libtracker.ActivityTracker, deps DiscoverDeps) {
	discoverChainAgentsReporting(ctx, agents, contenoxDir, tracker, deps)
}

// DiscoverDeps carries what registering a declaration's own tool sources needs.
// Without a Store the pass registers nothing; without a Bus the rows are written
// while worker startup waits for a host that has one.
type DiscoverDeps struct {
	Store runtimetypes.Store
	Bus   libbus.Messenger
}

// discoverChainAgentsReporting is discoverChainAgents plus the sync results a
// caller may want to show a human.
func discoverChainAgentsReporting(ctx context.Context, agents agentregistryservice.Service, contenoxDir string, tracker libtracker.ActivityTracker, deps DiscoverDeps) []agentdecl.SyncResult {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	var notable []agentdecl.SyncResult
	roots := []string{contenoxDir}
	homeDir, homeErr := globalContenoxDir()
	if homeErr == nil {
		// system/ is scanned last, so an operator copy at the top level wins.
		roots = append(roots, homeDir, systemDir(homeDir))
	}

	generated, results := syncDeclaredAgents(ctx, contenoxDir, homeDir, tracker)
	if generated != "" {
		roots = append(roots, generated)
	}
	// Registered before discovery: the emitted chain names these toolsets.
	if deps.Store != nil {
		results = append(results, reconcileDeclaredTools(ctx, deps.Store, deps.Bus, results)...)
	}
	for _, r := range results {
		if r.Action == agentdecl.ActionRefused || r.Action == agentdecl.ActionIgnored || len(r.Unmapped) > 0 {
			notable = append(notable, r)
		}
	}

	reportErr, reportChange, end := tracker.Start(ctx, "discover", "chain_agents", "roots", roots)
	defer end()

	// DiscoverKept (not Discover) reports refused files and vanished agents through the tracker.
	res, err := chainagents.DiscoverKept(ctx, agents, tracker, nil, roots...)
	if err != nil {
		reportErr(err)
		return notable
	}
	if len(res.Created) > 0 || len(res.Updated) > 0 || len(res.Disabled) > 0 || len(res.Skipped) > 0 {

		reportChange(contenoxDir, map[string]any{
			"created":            res.Created,
			"updated":            res.Updated,
			"disabled":           res.Disabled,
			"skipped_name_taken": res.Skipped,
			"unchanged":          len(res.Unchanged),
		})
	}
	return notable
}

// ensureProfileChain makes chainFile resolvable under contenoxDir before a
// surface loads it. Only `contenox init` preseeds the declarations and only the
// fleet's discovery pass compiles them, and both run after the load, so a fresh
// contenox directory could never boot a surface. Preseed writes just the missing
// files and Sync is idempotent, so a populated directory is left as it is. A set
// chainEnv is honoured untouched: the operator named that exact file, and a
// missing one stays the hard error they asked for.
func ensureProfileChain(ctx context.Context, contenoxDir, chainFile, chainEnv string, tracker libtracker.ActivityTracker) error {
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	if strings.TrimSpace(os.Getenv(chainEnv)) != "" {
		return nil
	}
	if acpsvc.ChainFileResolves(contenoxDir, chainFile) {
		return nil
	}
	reportErr, reportChange, end := tracker.Start(ctx, "ensure", "profile_chain", "file", chainFile)
	defer end()

	contenoxRoot, err := declRoot(contenoxDir)
	if err != nil {
		reportErr(err)
		return fmt.Errorf("open %s: %w", contenoxDir, err)
	}
	seeded, err := agentdecl.Preseed(ctx, contenoxRoot)
	if err != nil {
		reportErr(err)
		return fmt.Errorf("seed agent declarations: %w", err)
	}
	homeDir, err := globalContenoxDir()
	if err != nil {
		reportErr(err)
		return err
	}
	syncDeclaredAgents(ctx, contenoxDir, homeDir, tracker)
	if !acpsvc.ChainFileResolves(contenoxDir, chainFile) {
		err := fmt.Errorf("seeded %s but no chain %q was compiled", contenoxDir, chainFile)
		reportErr(err)
		return err
	}
	reportChange(contenoxDir, map[string]any{"seeded": len(seeded.Created), "refreshed": len(seeded.Updated)})
	return nil
}

// printSyncProblems shows what a discovery pass could not act on, on stderr.
func printSyncProblems(w io.Writer, results []agentdecl.SyncResult) {
	for _, r := range results {
		switch r.Action {
		case agentdecl.ActionRefused:
			fmt.Fprintf(w, "refused  %s: %s\n", r.Source, r.Reason)
		case agentdecl.ActionIgnored:
			fmt.Fprintf(w, "ignored  %s: %s\n", r.Source, r.Reason)
		}
		for _, u := range r.Unmapped {
			fmt.Fprintf(w, "not carried  %s: %s — %s\n", r.Name, u.Field, u.Reason)
		}
	}
}

// syncDeclaredAgents transpiles every Markdown declaration into
// contenoxDir/generated and returns that directory, empty when there is nothing
// to discover.
func syncDeclaredAgents(ctx context.Context, contenoxDir, homeDir string, tracker libtracker.ActivityTracker) (string, []agentdecl.SyncResult) {
	reportErr, reportChange, end := tracker.Start(ctx, "sync", "declared_agents")
	defer end()

	homeRoot, homeRootErr := declRoot(homeDir)
	if homeRootErr != nil {
		homeRoot = agentdecl.Root{}
	}
	contenoxRoot, rootErr := declRoot(contenoxDir)
	if rootErr != nil {
		reportErr(rootErr)
		return "", nil
	}

	// Declarations are read workspace first, so a workspace's own file precedes
	// the same name in the home directory; agents.toml is read the other way
	// round, weakest first.
	sourceRoots := []agentdecl.Root{contenoxRoot}
	configRoots := []agentdecl.Root{contenoxRoot}
	if homeRootErr == nil {
		sourceRoots = append(sourceRoots, homeRoot)
		configRoots = []agentdecl.Root{homeRoot, contenoxRoot}
	}

	workspaceRoots := workspaceRootsForSync(contenoxDir)
	var workspaceRoot string
	if len(workspaceRoots) > 0 {
		workspaceRoot = workspaceRoots[0]
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		workspaceRoots = append(workspaceRoots, home)
	}

	sourceDirs := agentdecl.DiscoverSourceDirs(ctx, sourceRoots, rootsOf(workspaceRoots...))
	generatedPath := filepath.Join(contenoxDir, agentdecl.GeneratedDirName)
	if len(sourceDirs) == 0 {
		if _, err := os.Stat(generatedPath); err != nil {
			return "", nil
		}
	}

	cfg, err := agentdecl.Load(ctx, configRoots...)
	if err != nil {
		reportErr(err)
		return "", nil
	}

	generated, genErr := declRoot(generatedPath)
	if genErr != nil {
		reportErr(genErr)
		return "", nil
	}

	// Skills live beside the agents that use them, with the same nearest-wins
	// precedence, resolved relative to the project root.
	skills := agentdecl.DiscoverSkills(ctx, sourceRoots, workspaceRoot)

	results, err := agentdecl.Sync(ctx, sourceDirs, generated, cfg, agentdecl.WithSkills(skills))
	if err != nil {
		reportErr(err)
		return "", nil
	}

	changed := map[string]any{}
	for _, r := range results {
		switch r.Action {
		case agentdecl.ActionRefused:
			changed["refused:"+r.Source] = r.Reason
		case agentdecl.ActionIgnored:
			changed["ignored:"+r.Source] = r.Reason
		case agentdecl.ActionCreated, agentdecl.ActionUpdated:
			changed[string(r.Action)+":"+r.Name] = r.Source
		}
		for _, u := range r.Unmapped {
			changed["unmapped:"+r.Name+"."+u.Field] = u.Reason
		}
	}
	if len(changed) > 0 {
		reportChange(generatedPath, changed)
	}
	if _, err := os.Stat(generatedPath); err != nil {
		return "", results
	}
	return generatedPath, results
}

// declRoot opens a directory as a compilation root. An empty path is not a
// root, which is how a run without a home contenox directory stays a run.
func declRoot(dir string) (agentdecl.Root, error) {
	if strings.TrimSpace(dir) == "" {
		return agentdecl.Root{}, fmt.Errorf("no directory to read")
	}
	return agentdecl.LocalRoot(dir)
}

// rootsOf opens the directories that exist as compilation roots, keeping the
// order given. A directory that cannot be opened is dropped rather than failing
// the caller: discovery reads what is there.
func rootsOf(dirs ...string) []agentdecl.Root {
	out := make([]agentdecl.Root, 0, len(dirs))
	for _, dir := range dirs {
		if root, err := declRoot(dir); err == nil {
			out = append(out, root)
		}
	}
	return out
}

// workspaceRootsForSync is the project a contenox directory belongs to.
func workspaceRootsForSync(contenoxDir string) []string {
	parent := filepath.Dir(filepath.Clean(contenoxDir))
	if parent == "." || parent == string(filepath.Separator) {
		return nil
	}
	return []string{parent}
}

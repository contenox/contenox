package contenoxcli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"unicode"

	libbus "github.com/contenox/runtime/libbus"
	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/modelservice"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/transport"
	"github.com/spf13/cobra"
)

var modelCmd = &cobra.Command{
	Use:     "model",
	Aliases: []string{"models"},
	Short:   "Inspect LLM models from live backends and local disk.",
	Long: `Inspect models from LLM backends and local model storage.

'model list' queries registered backends in real time and shows models that can
be used now. For local llama/OpenVINO, that means modeld is running in the
matching backend mode and can describe/load the model. Inactive local modeld
backend registrations are hidden from this live list.

'model local' is the offline inventory of installed GGUF/OpenVINO artifacts on
disk. It does not require modeld and may include models that are not currently
loadable by the active daemon.

Examples:
  contenox model list
  contenox model local
  contenox model registry-list

Set the default model:
  contenox config set default-model    gemini-flash-latest
  contenox config set default-provider gemini`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			return fmt.Errorf("unknown subcommand %q\n\nTo set a default model:\n  contenox config set default-model <model>\n  contenox config set default-provider <provider>", args[0])
		}
		return cmd.Help()
	},
}

var modelListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List models currently loadable from live backends.",
	Long: `Query each registered backend in real time and show models that can be used now.

For cloud/Ollama/vLLM providers this is the provider-advertised live catalog.
For local llama/OpenVINO this is the modeld runtime view: only the backend mode
currently served by modeld is shown, and only models modeld can describe/load
are listed. Use 'contenox model local' to inspect installed local artifacts even
when modeld is stopped or serving the other local backend.

Shows model name, backend, and effective capabilities observed at runtime plus
manual overrides (chat, embed, prompt, think, context length).

Examples:
  contenox model list`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, _, err := openBackendDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()
		return printLiveModels(ctx, db, cmd.OutOrStdout(), cmd.ErrOrStderr())
	},
}

var modelLocalCmd = &cobra.Command{
	Use:     "local",
	Aliases: []string{"installed", "library"},
	Short:   "List installed local model artifacts.",
	Long: `List local llama/OpenVINO model artifacts on disk.

This is an offline inventory surface: it scans local model directories and does
not require modeld to be running or the model to be loaded. When modeld is
reachable, the currently active slot is marked in the STATUS column.

Examples:
  contenox model local
  contenox model installed`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, _, err := openBackendDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()
		return printLocalModelInventory(ctx, db, cmd.OutOrStdout())
	},
}

// printLiveModels runs one backend reconciliation cycle and prints what each
// backend is actually serving right now.
func printLiveModels(ctx context.Context, db libdb.DBManager, out, errW io.Writer) error {
	bus := libbus.NewSQLite(db.WithoutTransaction())
	defer bus.Close()

	// Read the preferred model from config so we can mark it.
	store := runtimetypes.New(db.WithoutTransaction())
	preferredModel, err := getConfigKV(ctx, store, "default-model")
	if err != nil {
		return fmt.Errorf("failed to get preferred model: %w", err)
	}

	state, err := runtimestate.New(ctx, db, bus, runtimestate.WithSkipDeleteUndeclaredModels(), runtimestate.WithAutoDiscoverModels())
	if err != nil {
		return fmt.Errorf("failed to initialize runtime state: %w", err)
	}

	// A single cycle contacts every backend and populates PulledModels.
	if err := state.RunBackendCycle(ctx); err != nil {
		// Non-fatal: partial results are still useful.
		fmt.Fprintf(errW, "warning: backend cycle error: %v\n", err)
	}

	rt := state.Get(ctx)
	if len(rt) == 0 {
		fmt.Fprintln(out, "No backends registered. Run: contenox backend add <name> --type <type>")
		return nil
	}

	// Stable sort by backend name.
	type entry struct {
		backendName string
		backendType string
		backendErr  string
		pulled      []string
		canChat     map[string]bool
		canEmbed    map[string]bool
		canPrompt   map[string]bool
		canThink    map[string]bool
		ctx         map[string]int
	}
	var entries []entry
	for _, bs := range rt {
		e := entry{
			backendName: bs.Name,
			backendType: modelrepo.CanonicalBackendType(bs.Backend.Type),
			backendErr:  bs.Error,
			canChat:     map[string]bool{},
			canEmbed:    map[string]bool{},
			canPrompt:   map[string]bool{},
			canThink:    map[string]bool{},
			ctx:         map[string]int{},
		}
		for _, pm := range bs.PulledModels {
			e.pulled = append(e.pulled, pm.Model)
			e.canChat[pm.Model] = pm.CanChat
			e.canEmbed[pm.Model] = pm.CanEmbed
			e.canPrompt[pm.Model] = pm.CanPrompt
			e.canThink[pm.Model] = pm.CanThink
			e.ctx[pm.Model] = pm.ContextLength
		}
		// Some providers only report model names; when the backend is healthy,
		// keep those visible even if no detailed PulledModels entries were built.
		if len(e.pulled) == 0 && bs.Error == "" && len(bs.Models) > 0 {
			e.pulled = append(e.pulled, bs.Models...)
		}
		sort.Strings(e.pulled)
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].backendName < entries[j].backendName })

	any := false
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BACKEND\tMODEL\tCHAT\tEMBED\tPROMPT\tTHINK\tCTX")
	activeModeldBackend := modeldconn.Backend()
	for _, e := range entries {
		if hideInactiveLocalBackend(e.backendType, activeModeldBackend, e.backendErr, e.pulled) {
			continue
		}
		if e.backendErr != "" {
			errMsg := e.backendErr
			if len(errMsg) > 80 {
				errMsg = errMsg[:80] + "..."
			}
			fmt.Fprintf(w, "%s\t(unreachable: %s)\t\t\t\t\t\n", e.backendName, errMsg)
			continue
		}
		if len(e.pulled) == 0 {
			fmt.Fprintf(w, "%s\t(no models)\t\t\t\t\t\n", e.backendName)
			continue
		}
		for _, m := range e.pulled {
			any = true
			displayName := displayModelName(m)
			if preferredModel != "" && m == preferredModel {
				displayName += " *"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\n",
				e.backendName, displayName,
				boolMark(e.canChat[m]),
				boolMark(e.canEmbed[m]),
				boolMark(e.canPrompt[m]),
				boolMark(e.canThink[m]),
				e.ctx[m],
			)
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if !any {
		fmt.Fprintln(out, "\nNo loadable models found on any live backend. For installed local artifacts, run: contenox model local")
	}
	if preferredModel != "" {
		fmt.Fprintln(out, "\n* = default model (contenox config set default-model <name>)")
	}
	return nil
}

func hideInactiveLocalBackend(backendType, activeModeldBackend, backendErr string, pulled []string) bool {
	typ := modelrepo.CanonicalBackendType(backendType)
	if typ != "llama" && typ != "openvino" {
		return false
	}
	if backendErr != "" || len(pulled) > 0 {
		return false
	}
	return activeModeldBackend == "" || activeModeldBackend != typ
}

type localModelInventoryEntry struct {
	BackendName string
	Type        string
	Model       string
	Path        string
	Status      string
}

type localModelScanRoot struct {
	backendName string
	typ         string
	root        string
}

func printLocalModelInventory(ctx context.Context, db libdb.DBManager, out io.Writer) error {
	entries, err := localModelInventory(ctx, db)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		fmt.Fprintln(out, "No local model artifacts found.")
		return nil
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BACKEND\tTYPE\tMODEL\tSTATUS\tPATH")
	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", e.BackendName, e.Type, e.Model, e.Status, e.Path)
	}
	return w.Flush()
}

func localModelInventory(ctx context.Context, db libdb.DBManager) ([]localModelInventoryEntry, error) {
	backends, err := backendservice.New(db).List(ctx, nil, 1000)
	if err != nil {
		return nil, fmt.Errorf("list backends: %w", err)
	}

	var roots []localModelScanRoot
	for _, b := range backends {
		typ := modelrepo.CanonicalBackendType(b.Type)
		if typ != "llama" && typ != "openvino" {
			continue
		}
		if strings.TrimSpace(b.BaseURL) == "" {
			continue
		}
		roots = append(roots, localModelScanRoot{backendName: b.Name, typ: typ, root: b.BaseURL})
	}
	for _, r := range defaultLocalModelRoots() {
		roots = append(roots, r)
	}

	status, _ := modeldconn.Status(ctx)
	var out []localModelInventoryEntry
	seen := map[string]bool{}
	for _, root := range roots {
		found, err := scanLocalModelRoot(root.backendName, root.typ, root.root, status)
		if err != nil {
			continue
		}
		for _, e := range found {
			key := e.Type + "\x00" + e.Path
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		if out[i].BackendName != out[j].BackendName {
			return out[i].BackendName < out[j].BackendName
		}
		return out[i].Model < out[j].Model
	})
	return out, nil
}

func defaultLocalModelRoots() []localModelScanRoot {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil
	}
	base := filepath.Join(home, ".contenox", "models")
	return []localModelScanRoot{
		{backendName: "(default)", typ: "llama", root: filepath.Join(base, "llama")},
		{backendName: "(default)", typ: "openvino", root: filepath.Join(base, "openvino")},
		{backendName: "(legacy)", typ: "llama", root: base},
	}
}

func scanLocalModelRoot(backendName, typ, root string, status transport.DaemonStatus) ([]localModelInventoryEntry, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []localModelInventoryEntry
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		dir := filepath.Join(root, name)
		path, ok := localModelArtifactPath(typ, dir)
		if !ok {
			continue
		}
		out = append(out, localModelInventoryEntry{
			BackendName: backendName,
			Type:        typ,
			Model:       name,
			Path:        path,
			Status:      localModelStatus(typ, path, status),
		})
	}
	return out, nil
}

func localModelArtifactPath(typ, dir string) (string, bool) {
	switch typ {
	case "llama":
		path := filepath.Join(dir, "model.gguf")
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	case "openvino":
		if _, err := os.Stat(filepath.Join(dir, "openvino_model.xml")); err == nil {
			return dir, true
		}
	}
	return "", false
}

func localModelStatus(typ, path string, status transport.DaemonStatus) string {
	if status.Active == nil {
		return "installed"
	}
	if status.Active.Type == typ && status.Active.Path == path {
		if status.State == "" {
			return "active"
		}
		return "active:" + string(status.State)
	}
	return "installed"
}

func boolMark(b bool) string {
	if b {
		return "✓"
	}
	return "-"
}

func displayModelName(model string) string {
	return strings.TrimPrefix(strings.TrimSpace(model), "models/")
}

// parseContextSize converts a human-friendly token-count string to an int.
// Accepted suffixes (case-insensitive): k (×1 000), m (×1 000 000).
// A bare integer is returned as-is.  Examples:
//
//	"12k" → 12000
//	"128K" → 128000
//	"1m"  → 1000000
//	"8192" → 8192
//	"0"   → 0  (API-authoritative)
func parseContextSize(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("context size must not be empty")
	}
	last := rune(s[len(s)-1])
	var multiplier int64 = 1
	numPart := s
	if unicode.IsLetter(last) {
		numPart = s[:len(s)-1]
		switch unicode.ToLower(last) {
		case 'k':
			multiplier = 1_000
		case 'm':
			multiplier = 1_000_000
		default:
			return 0, fmt.Errorf("unknown suffix %q: use k (thousands) or m (millions)", string(last))
		}
	}
	n, err := strconv.ParseInt(numPart, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid context size %q: %w", s, err)
	}
	if n < 0 {
		return 0, fmt.Errorf("context size must be ≥ 0, got %d", n)
	}
	return int(n * multiplier), nil
}

var modelSetContextCmd = &cobra.Command{
	Use:   "set-context <model-name>",
	Short: "Set a local context override for a model.",
	Long: `Override the locally stored context window for a model already known to the local runtime state.

Accepts a bare integer or a k/m shorthand (case-insensitive):
  k  – thousands   (12k  = 12 000)
  m  – millions    (1m   = 1 000 000)

Examples:
  contenox model set-context gpt-5-mini           --context 128k
  contenox model set-context gemini-3.1-pro-preview --context 1m
  contenox model set-context qwen2.5:7b             --context 32k`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		db, _, err := openBackendDB(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctxRaw, _ := cmd.Flags().GetString("context")
		ctxLen, err := parseContextSize(ctxRaw)
		if err != nil {
			return fmt.Errorf("--context: %w", err)
		}
		modelName := args[0]
		store := runtimetypes.New(db.WithoutTransaction())
		m, err := store.GetModelByName(ctx, modelName)
		if err != nil {
			return fmt.Errorf("model %q has no local override row yet: %w", modelName, err)
		}
		m.ContextLength = ctxLen
		if err := modelservice.New(db, "").Update(ctx, m); err != nil {
			return fmt.Errorf("failed to update model: %w", err)
		}
		if ctxLen == 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Model %q context cleared (API is authoritative).\n", modelName)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "Model %q context set to %d.\n", modelName, ctxLen)
		}
		return nil
	},
}

func init() {
	modelSetContextCmd.Flags().String("context", "", "Context window size: bare int or shorthand (12k, 128k, 1m).")
	_ = modelSetContextCmd.MarkFlagRequired("context")
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelLocalCmd)
	modelCmd.AddCommand(modelSetContextCmd)
}

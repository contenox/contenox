package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/contenox/contenox/internal/modeld/modelstore"
	"github.com/contenox/contenox/internal/modeldinstall"
	"github.com/contenox/contenox/internal/models/modeldprobe"
	"github.com/contenox/contenox/internal/models/modelhardware"
	"github.com/contenox/contenox/internal/models/modelregistry"
	"github.com/contenox/contenox/internal/models/modelrepo"
	native "github.com/contenox/contenox/internal/models/modelrepo/modeld"
	"github.com/contenox/contenox/internal/models/modelrepo/modeldconn"
	"github.com/contenox/contenox/internal/services/clikv"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/transport"
	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/cobra"
)

var autoCmd = &cobra.Command{
	Use:   "auto [us|eu|china|gus]",
	Short: "Choose a local model for this machine, install it, and open the TUI.",
	Long: `Select a current curated, permissively licensed model that is curated as able
to run a multi-step tool loop, holding at least 32768 tokens of usable context
in this machine's memory. Capability gates the choice and context qualifies it:
a larger window on a model that cannot use it is not a capability. Selection
uses available memory, the model's native context ceiling, and the KV cache type
the worker is configured with (CONTENOX_LLAMA_KV_CACHE_TYPE; q8_0 halves what a
window costs in device memory). Cold/planner capacity does not count.
Origin restricts the model developer's region. Downloads use Hugging Face by
default (HF_ENDPOINT overrides it; HF_TOKEN optionally authenticates).

The worker and model are installed, a harmless tool call is verified, and only
then are native defaults saved. Subsequent runs reuse the configured eligible
model; --refresh selects again. Setup uses native inference.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runAuto,
}

func init() {
	autoCmd.Flags().String("origin", "", "Model developer origin: us, eu, china or gus.")
	autoCmd.Flags().Bool("dry-run", false, "Show the hardware and selection without installing or changing defaults.")
	autoCmd.Flags().Bool("no-tui", false, "Install and verify, then exit instead of opening the TUI.")
	autoCmd.Flags().Bool("refresh", false, "Select from the current catalog instead of reusing the configured model.")
	autoCmd.Flags().String("worker-base-url", modeldinstall.DefaultBaseURL, "Native worker release index base URL.")
	reservedSubcommands["auto"] = true
	rootCmd.AddCommand(autoCmd)
}

func runAuto(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	origin, _ := cmd.Flags().GetString("origin")
	if len(args) != 0 {
		if origin != "" && origin != args[0] {
			return fmt.Errorf("choose either positional origin or --origin")
		}
		origin = args[0]
	}
	origin = strings.ToLower(strings.TrimSpace(origin))
	if !slices.Contains([]string{"", "us", "eu", "china", "gus"}, origin) {
		return fmt.Errorf("unknown origin %q: use us, eu, china or gus", origin)
	}
	dry, _ := cmd.Flags().GetBool("dry-run")
	noTUI, _ := cmd.Flags().GetBool("no-tui")
	refresh, _ := cmd.Flags().GetBool("refresh")
	if !dry && !noTUI {
		if err := requireBeamTerminal(); err != nil {
			return fmt.Errorf("auto opens the TUI; use --no-tui for unattended setup: %w", err)
		}
	}
	hardware := modelhardware.Detect(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	backend := hardware.PreferredBackend
	if override := strings.TrimSpace(os.Getenv("CONTENOX_MODELD_BACKEND")); override != "" {
		backend = override
	}
	probe := modeldprobe.New(modeldprobe.DefaultDataRoot()).Probe(ctx)
	if probe.State == modeldprobe.StateRunning {
		backend = probe.Backend
	}
	var selected modelregistry.ModelDescriptor
	reused := false
	if !refresh {
		selected, reused = previousAutoModel(ctx, cmd, origin, backend, hardware.MemoryBytes)
	}
	if !reused {
		var err error
		selected, err = modelregistry.SelectAutoType(origin, backend, hardware.MemoryBytes, workerKVCacheType())
		if err != nil {
			return fmt.Errorf("%s: %w", hardware.Summary, err)
		}
	}
	if dry {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Hardware            modelhardware.Hardware        `json:"hardware"`
			Model               modelregistry.ModelDescriptor `json:"model"`
			Reused              bool                          `json:"reused"`
			EstimatedHotContext int                           `json:"estimated_hot_context"`
		}{hardware, selected, reused, selected.AutoHotContextType(hardware.MemoryBytes, workerKVCacheType())})
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, hardware.Summary)
	fmt.Fprintf(out, "Selected %s (%s, %s, %s); download %s.\n", selected.Label(), selected.BackendType(), selected.Origin, selected.License, modelstore.FormatSize(selected.SizeBytes+selected.MMProjSizeBytes))
	kvType := workerKVCacheType()
	fmt.Fprintf(out, "Estimated hot context: %d tokens at %s KV; %d is the floor for agentic work.\n",
		selected.AutoHotContextType(hardware.MemoryBytes, kvType), kvLabelForDisplay(kvType), modelregistry.MinAgenticHotContext)
	if kvType == "" {
		fmt.Fprintln(out, "  Set CONTENOX_LLAMA_KV_CACHE_TYPE=q8_0 before starting the worker to halve what the window costs in device memory.")
	}
	root := modeldprobe.DefaultDataRoot()
	fmt.Fprintf(out, "Model store: %s\n", modelstore.Dir(root, ""))
	if reused {
		fmt.Fprintln(out, "Reusing configured model; --refresh selects from the current catalog.")
	}
	if err := checkAutoDisk(root, selected); err != nil {
		return err
	}
	if err := ensureAutoWorker(ctx, cmd, selected.BackendType()); err != nil {
		return err
	}
	if err := os.Setenv("CONTENOX_MODELD_BACKEND", selected.BackendType()); err != nil {
		return err
	}
	if err := modelstore.EnsureModelAvailable(ctx, modelregistry.New(nil), selected.Name, modelstore.AutoPullOptions{DataRoot: root, ProgressOut: out}); err != nil {
		return err
	}
	fmt.Fprintln(out, "Verifying native inference and tool calling...")
	hotContext, err := verifyAutoModel(ctx, out, selected, selected.AutoHotContextType(hardware.MemoryBytes, workerKVCacheType()))
	if err != nil {
		return fmt.Errorf("native verification failed; defaults were not changed: %w", err)
	}
	if err := saveAutoDefaults(ctx, cmd, selected); err != nil {
		return err
	}
	if err := modeldprobe.SetBackendPreference(root, selected.BackendType()); err != nil {
		return err
	}
	if err := RunGlobalInit(ctx, out); err != nil {
		return err
	}
	fmt.Fprintf(out, "Ready: %s; worker reports %d hot context tokens. Run contenox to start working.\n", selected.Name, hotContext)
	if noTUI {
		return nil
	}
	for _, setting := range []struct{ key, value string }{
		{envDefaultProvider, "modeld"}, {envDefaultModel, selected.Name},
		{envDefaultAltProvider, "modeld"}, {envDefaultAltModel, selected.Name},
	} {
		if err := os.Setenv(setting.key, setting.value); err != nil {
			return err
		}
	}
	if err := beamCmd.Flags().Set("new", "true"); err != nil {
		return err
	}
	beamCmd.SetContext(ctx)
	return beamCmd.RunE(beamCmd, nil)
}

// workerKVCacheType is the cache the modeld this machine will use is configured
// with, read from the same variable modeld reads. Selecting against anything
// else would size a window the worker cannot allocate: at f16 a 32K window
// costs more than a 6 GB device has free on the models that fit it, and half
// that at q8_0.
func workerKVCacheType() string {
	return strings.TrimSpace(os.Getenv("CONTENOX_LLAMA_KV_CACHE_TYPE"))
}

func kvLabelForDisplay(kvType string) string {
	if kvType == "" {
		return "f16"
	}
	return kvType
}

func previousAutoModel(ctx context.Context, cmd *cobra.Command, origin, backend string, budget int64) (modelregistry.ModelDescriptor, bool) {
	path, err := resolveDBPath(cmd)
	if err != nil {
		return modelregistry.ModelDescriptor{}, false
	}
	if _, err := os.Stat(path); err != nil {
		return modelregistry.ModelDescriptor{}, false
	}
	db, err := openOptionalDB(ctx, path)
	if err != nil || db == nil {
		return modelregistry.ModelDescriptor{}, false
	}
	defer db.Close()
	store := runtimetypes.New(db.WithoutTransaction())
	provider, _ := getConfigKV(ctx, store, "default-provider")
	name, _ := getConfigKV(ctx, store, "default-model")
	if provider != "modeld" {
		return modelregistry.ModelDescriptor{}, false
	}
	model, err := modelregistry.New(nil).Resolve(ctx, name)
	if err != nil || !model.AutoEligible || !model.SupportsAgenticTools() || model.BackendType() != backend || (origin != "" && model.Origin != origin) {
		return modelregistry.ModelDescriptor{}, false
	}
	if model.AutoHotContextType(budget, workerKVCacheType()) < modelregistry.MinAgenticHotContext {
		status, err := modeldconn.Status(ctx)
		if err != nil || status.Active == nil || status.Active.ModelName != model.Name {
			return modelregistry.ModelDescriptor{}, false
		}
	}
	return *model, true
}

func ensureAutoWorker(ctx context.Context, cmd *cobra.Command, backend string) error {
	detector := modeldprobe.New(modeldprobe.DefaultDataRoot())
	status := detector.Detect()
	if status.Binary != "" {
		info, err := modeldinstall.ProbeBinary(ctx, status.Binary)
		if err == nil && slices.Contains(info.Backends, backend) && transport.Supported(info.Protocol) {
			return nil
		}
		if os.Getenv("CONTENOX_MODELD_BIN") != "" {
			return fmt.Errorf("explicit CONTENOX_MODELD_BIN cannot serve %s: %v", backend, err)
		}
	}
	base, _ := cmd.Flags().GetString("worker-base-url")
	_, err := modeldinstall.EnsureInstalled(ctx, backend, modeldinstall.Options{DataRoot: modeldprobe.DefaultDataRoot(), BaseURL: base, Progress: cmd.OutOrStdout()})
	return err
}

func checkAutoDisk(root string, model modelregistry.ModelDescriptor) error {
	if _, err := modelstore.Resolve(modelstore.Dir(root, ""), model.Name, model.BackendType(), ""); err == nil {
		return nil
	}
	path := root
	for {
		if _, err := os.Stat(path); err == nil {
			break
		}
		parent := filepath.Dir(path)
		if parent == path {
			return fmt.Errorf("cannot find filesystem for %s", root)
		}
		path = parent
	}
	usage, err := disk.Usage(path)
	if err != nil {
		return fmt.Errorf("check download storage: %w", err)
	}
	required := model.SizeBytes + model.MMProjSizeBytes + 512*1024*1024
	if required <= 0 || uint64(required) > usage.Free {
		return fmt.Errorf("insufficient storage: need %s, available %s", modelstore.FormatSize(required), modelstore.FormatSize(int64(usage.Free)))
	}
	return nil
}

// verifyAutoModel runs one harmless tool call and reports the hot context the
// opened session really has. planned is what selection estimated with the KV
// cache type the worker was expected to run: the worker's own type is the last
// word, so a session that comes back below the floor is refused here rather than
// saved as a model that cannot do the work it was picked for.
func verifyAutoModel(ctx context.Context, out io.Writer, model modelregistry.ModelDescriptor, planned int) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if err := modeldconn.EnsureDaemon(ctx); err != nil {
		return 0, err
	}
	info, err := modeldconn.Describe(ctx, modeldconn.ModelRef{Name: model.Name, Type: model.BackendType()}, transport.Config{})
	if err != nil {
		return 0, err
	}
	if info.EffectiveContext < modelregistry.MinAgenticHotContext {
		return 0, fmt.Errorf("worker reports %d context tokens for %s; agentic work needs %d. "+
			"If the selection was planned at a quantized KV cache, start the worker with the same "+
			"CONTENOX_LLAMA_KV_CACHE_TYPE=%s it was planned with",
			info.EffectiveContext, model.Name, modelregistry.MinAgenticHotContext, kvLabelForDisplay(workerKVCacheType()))
	}
	provider := native.NewModeldProvider(model.Name, []string{"local"}, modelrepo.CapabilityConfig{CanChat: true, CanStream: true, ContextLength: info.EffectiveContext}, nil)
	chat, err := provider.GetChatConnection(ctx, "local")
	if err != nil {
		return 0, err
	}
	// The probe tool carries the shape a real toolset has — a required parameter and
	// a non-string one — because llama.cpp's generated grammar for a model-native
	// template is a strict sequence over a tool's declared parameters. A schema with
	// no required parameters (the shape this check used to send) cannot fail that way,
	// so it certified models whose real tool calls never parsed.
	probeTool := modelrepo.Tool{Type: "function", Function: &modelrepo.FunctionTool{
		Name:        "contenox_ready",
		Description: "Confirm this local installation is ready. This is a harmless setup check.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"status":  map[string]any{"type": "string", "description": "Reported readiness, \"ok\" when the installation works."},
				"verbose": map[string]any{"type": "boolean", "description": "Ask for a verbose readiness report."},
			},
			"required": []string{"status"},
		},
	}}
	result, err := chat.Chat(ctx, []modelrepo.Message{{Role: "user", Content: "Call the contenox_ready tool now with a status of ok. Do not answer in prose."}}, modelrepo.WithMaxTokens(1024), modelrepo.WithThink("off"), modelrepo.WithTemperature(0), modelrepo.WithTool(probeTool))
	if err != nil {
		return 0, err
	}
	for _, call := range result.ToolCalls {
		if call.Function.Name != "contenox_ready" {
			continue
		}
		session, err := modeldconn.OpenSession(ctx, modeldconn.ModelRef{Name: model.Name, Type: model.BackendType()}, transport.Config{})
		if err != nil {
			return 0, err
		}
		report := session.ExplainContext()
		session.Close()
		hot := report.HotContextTokens
		if hot <= 0 || hot > report.NumCtx {
			hot = report.NumCtx
		}
		if hot < modelregistry.MinAgenticHotContext {
			return 0, fmt.Errorf("opened session reports %d hot context tokens for %s; agentic work needs %d, "+
				"and selection estimated %d with KV cache type %s",
				hot, model.Name, modelregistry.MinAgenticHotContext, planned, kvLabelForDisplay(workerKVCacheType()))
		}
		if planned > 0 && hot < planned/2 {
			fmt.Fprintf(out, "  note: the session opened with %d hot tokens, well under the %d estimated for KV cache type %s\n",
				hot, planned, kvLabelForDisplay(workerKVCacheType()))
		}
		return hot, nil
	}

	return 0, fmt.Errorf("model did not produce the expected native tool call")
}

// saveAutoDefaults records the selection auto made: which provider and model
// this machine should use. It deliberately records no context window. The
// default-token-limit mask exists for providers that cannot report a model's
// capacity, and the modeld worker reports its own — a measured number written
// here freezes one reading of free device memory, so a later session that sees
// slightly less VRAM fails a turn against a window the operator never chose.
func saveAutoDefaults(ctx context.Context, cmd *cobra.Command, model modelregistry.ModelDescriptor) error {
	db, svc, err := openBackendDB(cmd)
	if err != nil {
		return err
	}
	defer db.Close()
	backends, err := svc.List(ctx, nil, 100)
	if err != nil {
		return err
	}
	found := false
	for _, backend := range backends {
		if backend.Type == "modeld" && backend.BaseURL == "local" {
			found = true
			break
		}
	}
	if !found {
		if err := svc.Create(ctx, &runtimetypes.Backend{ID: uuid.NewString(), Name: "native-local", Type: "modeld", BaseURL: "local"}); err != nil {
			return err
		}
	}
	store := runtimetypes.New(db.WithoutTransaction())
	for _, setting := range []struct{ key, value string }{
		{"default-provider", "modeld"}, {"default-model", model.Name},
		{"default-alt-provider", "modeld"}, {"default-alt-model", model.Name},
	} {
		if err := clikv.WriteConfig(ctx, store, "", setting.key, setting.value); err != nil {
			return err
		}
	}
	return nil
}

package contenoxcli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/internal/modeldinstall"
	"github.com/contenox/runtime/runtime/internal/modeldprobe"
	"github.com/contenox/runtime/runtime/internal/setupcheck"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/transport"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive wizard to configure your LLM provider and model.",
	Long: `Run the setup wizard to pick an LLM provider (Ollama, OpenAI, OpenRouter,
Gemini, or local llama/OpenVINO through modeld), enter credentials, and set defaults.
This is the same wizard that runs inside IDE terminals via ACP.

Examples:
  contenox setup`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSetup(cmd, cmd.OutOrStdout())
	},
}

type setupProvider struct {
	key          string
	label        string
	defaultModel string
	envKey       string
	needsAPIKey  bool
}

var setupProviders = []setupProvider{
	{key: "ollama", label: "Ollama (local daemon)", defaultModel: "qwen2.5:7b", needsAPIKey: false},
	{key: "openai", label: "OpenAI", defaultModel: "gpt-5-mini", envKey: "OPENAI_API_KEY", needsAPIKey: true},
	{key: "openrouter", label: "OpenRouter (300+ models, one API key — deepseek, qwen, llama, gemini, gpt and more)", defaultModel: "deepseek/deepseek-chat-v3-5", envKey: "OPENROUTER_API_KEY", needsAPIKey: true},
	{key: "gemini", label: "Google Gemini", defaultModel: "gemini-flash-latest", envKey: "GEMINI_API_KEY", needsAPIKey: true},
	{key: "llama", label: "Llama.cpp GGUF (local modeld)", defaultModel: "", needsAPIKey: false},
	{key: "openvino", label: "OpenVINO IR (local modeld)", defaultModel: "", needsAPIKey: false},
}

func runSetup(cmd *cobra.Command, out io.Writer) error {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Welcome to Contenox!")
	fmt.Fprintln(out, "")

	if err := RunGlobalInit(out); err != nil {
		return fmt.Errorf("global init: %w", err)
	}

	// Show current configuration status.
	alreadyConfigured := false
	if dbPath, gpErr := globalDBPath(); gpErr == nil {
		if db, openErr := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath); openErr == nil {
			store := runtimetypes.New(db.WithoutTransaction())
			ctx := libtracker.WithNewRequestID(context.Background())
			curProvider := clikv.Read(ctx, store, "default-provider")
			curModel := clikv.Read(ctx, store, "default-model")
			svc := backendservice.New(db)
			backends, _ := svc.List(ctx, nil, 100)
			db.Close()

			if curProvider != "" || curModel != "" || len(backends) > 0 {
				alreadyConfigured = true
				fmt.Fprintln(out, "")
				fmt.Fprintln(out, "  Current configuration:")
				if curProvider != "" {
					fmt.Fprintf(out, "    Provider: %s\n", curProvider)
				}
				if curModel != "" {
					fmt.Fprintf(out, "    Model:    %s\n", curModel)
				}
				if len(backends) > 0 {
					fmt.Fprintf(out, "    Backends: %d registered", len(backends))
					var names []string
					for _, b := range backends {
						names = append(names, b.Name)
					}
					fmt.Fprintf(out, " (%s)\n", strings.Join(names, ", "))
				}
			}
		}
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Choose your LLM provider:")
	fmt.Fprintln(out, "")
	for i, p := range setupProviders {
		fmt.Fprintf(out, "    %d. %s\n", i+1, p.label)
	}
	if alreadyConfigured {
		fmt.Fprintln(out, "    q. Keep current configuration")
	}
	fmt.Fprintln(out, "")

	scanner := bufio.NewScanner(os.Stdin)
	chosen := promptChoiceOrQuit(out, scanner, "  Provider", len(setupProviders), alreadyConfigured)
	if chosen < 0 {
		fmt.Fprintln(out, "  ✓ Keeping current configuration.")
		fmt.Fprintln(out, "")
		return nil
	}
	sp := setupProviders[chosen]

	var apiKey string
	if sp.needsAPIKey {
		apiKey = os.Getenv(sp.envKey)
		if apiKey != "" {
			fmt.Fprintf(out, "  ✓ Found %s in environment.\n\n", sp.envKey)
		} else {
			// Check if a key is already stored in the database.
			if dbPath, gpErr := globalDBPath(); gpErr == nil {
				if db, openErr := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath); openErr == nil {
					store := runtimetypes.New(db.WithoutTransaction())
					var cfg runtimestate.ProviderConfig
					kvKey := runtimestate.ProviderKeyPrefix + sp.key
					if err := store.GetKV(libtracker.WithNewRequestID(context.Background()), kvKey, &cfg); err == nil && cfg.APIKey != "" {
						apiKey = cfg.APIKey
						fmt.Fprintf(out, "  ✓ %s API key already stored.\n\n", sp.label)
					}
					db.Close()
				}
			}
		}
		if apiKey == "" {
			fmt.Fprintf(out, "  Enter your %s API key (or set %s):\n", sp.label, sp.envKey)
			apiKey = promptSecret(out, scanner, "  API key")
			if apiKey == "" {
				return fmt.Errorf("API key is required for %s", sp.label)
			}
			fmt.Fprintln(out, "")
		}
	}

	model := sp.defaultModel
	switch sp.key {
	case "ollama":
		model = promptOllamaModel(out, scanner, model)
	case "llama", "openvino":
		setupLocalModeld(out, sp.key)
	default:
		model = promptLine(out, scanner, fmt.Sprintf("  Model [%s]", model), model)
	}

	dbPath, err := globalDBPath()
	if err != nil {
		return fmt.Errorf("resolve db: %w", err)
	}
	ctx := libtracker.WithNewRequestID(context.Background())
	db, err := OpenDBAt(ctx, dbPath)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	if !isLocalModeldProvider(sp.key) {
		if err := registerSetupBackend(ctx, db, sp.key, apiKey); err != nil {
			return err
		}
	}

	store := runtimetypes.New(db.WithoutTransaction())
	_ = clikv.WriteConfig(ctx, store, "global", "default-provider", sp.key)
	if model != "" {
		_ = clikv.WriteConfig(ctx, store, "global", "default-model", model)
	}

	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "  ✓ Provider: %s\n", sp.key)
	if model != "" {
		fmt.Fprintf(out, "  ✓ Model:    %s\n", model)
	}

	reportSetupReadiness(ctx, cmd, db, out, sp.key, model)
	return nil
}

func isLocalModeldProvider(provider string) bool {
	switch provider {
	case "llama", "openvino":
		return true
	default:
		return false
	}
}

// setupLocalModeld resolves a protocol-compatible prebuilt modeld package for
// this platform and installs it. On any soft failure (no index, no compatible
// package, unsupported platform, network error) it falls back to the source-build
// instructions; a checksum mismatch is reported as a hard failure without
// falling back. Backend selection is never forced here — modeld picks its live
// backend at `serve` time.
func setupLocalModeld(out io.Writer, provider string) {
	runLocalModeldSetup(out, provider, modeldinstall.Options{
		ClientVersion: strings.TrimSpace(CLIVersion()),
		DataRoot:      modeldprobe.DefaultDataRoot(),
		Progress:      out,
	})
}

// runLocalModeldSetup is the testable core of setupLocalModeld: it takes the
// install options (so tests can point at a fake server + temp data root) and
// drives the prebuilt-check / install / fallback UX.
func runLocalModeldSetup(out io.Writer, provider string, opts modeldinstall.Options) {
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	platform := goos + "-" + goarch
	if opts.Progress == nil {
		opts.Progress = out
	}
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "  Local modeld provider selected: %s\n", provider)
	fmt.Fprintf(out, "  Resolving a compatible modeld build (protocol %d, %s)...\n", transport.ProtocolVersion, platform)

	res, err := modeldinstall.EnsureInstalled(context.Background(), provider, opts)
	if err != nil {
		printModeldInstallFallback(out, provider, platform, err)
		return
	}

	if res.AlreadyInstalled {
		fmt.Fprintf(out, "  Using installed modeld at %s\n", res.LauncherPath)
	}
	fmt.Fprintf(out, "  Validated modeld %s (protocol %d) with compiled backends: %s\n", res.Version, res.Protocol, strings.Join(res.Backends, ", "))
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Start modeld:")
	fmt.Fprintf(out, "    %s serve\n", res.LauncherPath)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Install a local model:")
	if provider == "openvino" {
		printOpenVINOModelChoices(out)
	} else {
		printLlamaModelChoices(out)
	}
	fmt.Fprintln(out, "")
	printAutocompleteModeldTip(out, provider)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  The first pulled model becomes default-model automatically.")
}

// printModeldInstallFallback explains why the prebuilt path did not apply and,
// except for a checksum mismatch, prints the source-build instructions.
func printModeldInstallFallback(out io.Writer, provider, platform string, err error) {
	switch {
	case errors.Is(err, modeldinstall.ErrChecksumMismatch):
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  Downloaded modeld package failed checksum verification.")
		fmt.Fprintln(out, "  The package was not installed.")
		return
	case errors.Is(err, modeldinstall.ErrNoIndex):
		fmt.Fprintln(out, "\n  Could not reach the modeld release index. Config is saved; rerun `contenox setup` later.")
	case errors.Is(err, modeldinstall.ErrNoCompatibleArtifact):
		fmt.Fprintf(out, "\n  No prebuilt modeld build is compatible with this contenox (protocol %d, %s).\n", transport.ProtocolVersion, platform)
	case errors.Is(err, modeldinstall.ErrArtifactUnavailable):
		fmt.Fprintln(out, "\n  The selected prebuilt modeld artifact is not available from the release store.")
	case errors.Is(err, modeldinstall.ErrUnsupportedPlatform):
		fmt.Fprintf(out, "\n  No prebuilt modeld package format exists for %s.\n", platform)
	case errors.Is(err, modeldinstall.ErrProtocolMismatch):
		fmt.Fprintf(out, "\n  The installed modeld speaks an unsupported transport protocol for this contenox (supported: %d..%d).\n", transport.MinProtocol, transport.ProtocolVersion)
	case errors.Is(err, modeldinstall.ErrBackendMissing):
		fmt.Fprintf(out, "\n  The prebuilt modeld package does not include the %s backend.\n", provider)
	default:
		fmt.Fprintf(out, "\n  Could not check prebuilt modeld builds: %v\n", err)
		fmt.Fprintln(out, "  Config is saved. You can rerun `contenox setup` later.")
	}
	fmt.Fprintln(out, "  Use the source-build path for now:")
	printLocalModeldSourceBuildSteps(out, provider)
}

func printLocalModeldSourceBuildSteps(out io.Writer, backend string) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  In another terminal:")
	fmt.Fprintln(out, "")
	fmt.Fprintf(out, "    git clone --branch %s --depth 1 https://github.com/contenox/runtime.git contenox-runtime\n", modeldSourceBuildRef())
	fmt.Fprintln(out, "    cd contenox-runtime")
	if backend == "openvino" {
		fmt.Fprintln(out, "    make deps-modeld")
		fmt.Fprintln(out, "    CONTENOX_MODELD_BACKEND=openvino make run-modeld")
	} else {
		fmt.Fprintln(out, "    CONTENOX_MODELD_BACKEND=llama make run-modeld")
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Keep modeld running, then return here and install a local model:")
	if backend == "openvino" {
		printOpenVINOModelChoices(out)
	} else {
		printLlamaModelChoices(out)
	}
	fmt.Fprintln(out, "")
	printAutocompleteModeldTip(out, backend)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "    contenox model local           # installed local artifacts")
	fmt.Fprintln(out, "    contenox model list            # loadable by the live daemon")
	fmt.Fprintln(out, "    contenox doctor")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  'model local' shows installed files. 'model list' only shows models")
	fmt.Fprintln(out, "  that the running modeld can describe/load.")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Build guide:")
	fmt.Fprintln(out, "    https://github.com/contenox/runtime/blob/main/docs/modeld-source-build.md")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  The first pulled model becomes default-model automatically.")
}

func printAutocompleteModeldTip(out io.Writer, backend string) {
	fmt.Fprintln(out, "  Optional VS Code autocomplete model:")
	fmt.Fprintln(out, "    Autocomplete is separate from chat, so you can keep chat on a hosted")
	fmt.Fprintln(out, "    model and use a separate coder model for ghost text.")
	if backend == "openvino" {
		fmt.Fprintln(out, "    contenox model pull qwen2.5-coder-1.5b-ov")
		fmt.Fprintln(out, "    contenox config set default-autocomplete-provider openvino")
		fmt.Fprintln(out, "    contenox config set default-autocomplete-model qwen2.5-coder-1.5b-ov")
	} else {
		fmt.Fprintln(out, "    contenox model pull qwen3-coder-30b-a3b")
		fmt.Fprintln(out, "    contenox config set default-autocomplete-provider llama")
		fmt.Fprintln(out, "    contenox config set default-autocomplete-model qwen3-coder-30b-a3b")
	}
}

func printLlamaModelChoices(out io.Writer) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "       VRAM     Model               Q4 size   Notes")
	fmt.Fprintln(out, "       ~2 GB    granite-3.2-2b      ~1.5 GB")
	fmt.Fprintln(out, "       ~3 GB    qwen3-4b            ~3 GB")
	fmt.Fprintln(out, "       ~3 GB    phi-4-mini          ~2.5 GB")
	fmt.Fprintln(out, "       ~5 GB    gemma4-e4b          ~5 GB     native tool format")
	fmt.Fprintln(out, "       ~5 GB    qwen3-8b            ~5 GB")
	fmt.Fprintln(out, "       ~5 GB    deepseek-r1-0528-qwen3-8b")
	fmt.Fprintln(out, "       ~8 GB    gemma4-12b          ~8 GB")
	fmt.Fprintln(out, "       ~12 GB   gpt-oss-20b         ~12 GB")
	fmt.Fprintln(out, "       ~19 GB   qwen3-coder-30b-a3b ~19 GB")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "       contenox model registry-list   # full list with sizes")
	fmt.Fprintln(out, "       contenox model pull qwen3-8b")
}

func printOpenVINOModelChoices(out io.Writer) {
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "       Model                    Size      Notes")
	fmt.Fprintln(out, "       qwen2.5-coder-0.5b-ov    ~350 MB   fastest smoke test")
	fmt.Fprintln(out, "       qwen2.5-coder-1.5b-ov    ~900 MB   small coding model")
	fmt.Fprintln(out, "       qwen3-4b-ov              ~2.3 GB")
	fmt.Fprintln(out, "       qwen3-8b-ov              ~4.9 GB")
	fmt.Fprintln(out, "       phi-4-mini-ov            ~2.4 GB")
	fmt.Fprintln(out, "       gemma4-e4b-ov            ~6.5 GB")
	fmt.Fprintln(out, "       gpt-oss-20b-ov           ~12.6 GB")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "       contenox model registry-list   # full list with sizes")
	fmt.Fprintln(out, "       contenox model pull qwen2.5-coder-0.5b-ov")
}

func modeldSourceBuildRef() string {
	v := strings.TrimSpace(CLIVersion())
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "main"
}

// reportSetupReadiness runs the same read-only reachability check as
// `contenox doctor` (a backend sync, never a model completion) and prints an
// honest verdict instead of unconditionally claiming success. Config is already
// saved; this only reports — it never second-guesses the operator or mutates
// anything, so it is safe even against ERP/audited tool backends.
func reportSetupReadiness(ctx context.Context, cmd *cobra.Command, db libdb.DBManager, out io.Writer, provider, model string) {
	contenoxDir, _ := ResolveContenoxDir(cmd)
	opts, err := buildRunOpts(cmd, db, contenoxDir)
	if err != nil {
		fmt.Fprintf(out, "  ⚠  Could not verify setup: %v\n", err)
		fmt.Fprintln(out, "     Config is saved. Run `contenox doctor` to check connectivity.")
		fmt.Fprintln(out, "")
		return
	}
	opts.EffectiveDefaultProvider = provider
	if model != "" {
		opts.EffectiveDefaultModel = model
	}

	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Checking reachability (no test prompt is sent)...")
	res, err := ComputeReadiness(ctx, db, opts)
	if err != nil {
		fmt.Fprintf(out, "  ⚠  Could not verify setup: %v\n", err)
		fmt.Fprintln(out, "     Config is saved. Run `contenox doctor` to check connectivity.")
		fmt.Fprintln(out, "")
		return
	}
	if res.Ready() {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  ✓ Setup ready — provider reachable and a chat model is available.")
		fmt.Fprintln(out, "  Close this tab and start chatting!")
		fmt.Fprintln(out, "")
		return
	}
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Config saved, but the agent is not ready yet:")
	PrintSetupIssues(out, res)
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Fix the above (or run `contenox doctor`), then you're set.")
	fmt.Fprintln(out, "")
}

func registerSetupBackend(ctx context.Context, db libdb.DBManager, providerType, apiKey string) error {
	svc := backendservice.New(db)

	backendURL := ""
	switch providerType {
	case "ollama":
		if base, ok := setupcheck.ProbeLocalOllamaAPI(ctx); ok {
			backendURL = base
		} else {
			backendURL = "http://127.0.0.1:11434"
		}
	case "openai":
		backendURL = "https://api.openai.com/v1"
	case "openrouter":
		backendURL = "https://openrouter.ai/api/v1"
	case "gemini":
		backendURL = "https://generativelanguage.googleapis.com"
	}

	existing, _ := svc.List(ctx, nil, 100)
	for _, b := range existing {
		if strings.EqualFold(b.Type, providerType) {
			return nil
		}
	}

	backend := &runtimetypes.Backend{
		ID:      uuid.NewString(),
		Name:    providerType,
		BaseURL: backendURL,
		Type:    providerType,
	}
	if err := svc.Create(ctx, backend); err != nil {
		return fmt.Errorf("register %s backend: %w", providerType, err)
	}

	if apiKey != "" {
		store := runtimetypes.New(db.WithoutTransaction())
		key := runtimestate.ProviderKeyPrefix + providerType
		cfg := runtimestate.ProviderConfig{APIKey: apiKey, Type: providerType}
		data, _ := json.Marshal(cfg)
		_ = store.SetKV(ctx, key, data)
	}
	return nil
}

func promptOllamaModel(out io.Writer, scanner *bufio.Scanner, defaultModel string) string {
	ctx := context.Background()
	if base, ok := setupcheck.ProbeLocalOllamaAPI(ctx); ok {
		fmt.Fprintf(out, "  ✓ Ollama is running at %s\n\n", base)
	} else {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  ⚠ Ollama is not reachable. Make sure 'ollama serve' is running,")
		fmt.Fprintln(out, "    then pull a model: ollama pull qwen2.5:7b")
		fmt.Fprintln(out, "")
	}
	return promptLine(out, scanner, fmt.Sprintf("  Model [%s]", defaultModel), defaultModel)
}

func promptChoiceOrQuit(out io.Writer, scanner *bufio.Scanner, label string, max int, allowQuit bool) int {
	for {
		if allowQuit {
			fmt.Fprintf(out, "%s (1-%d, q to quit): ", label, max)
		} else {
			fmt.Fprintf(out, "%s (1-%d): ", label, max)
		}
		if !scanner.Scan() {
			return 0
		}
		text := strings.TrimSpace(scanner.Text())
		if allowQuit && strings.EqualFold(text, "q") {
			return -1
		}
		n, err := strconv.Atoi(text)
		if err == nil && n >= 1 && n <= max {
			return n - 1
		}
		if allowQuit {
			fmt.Fprintf(out, "  Please enter a number between 1 and %d, or 'q' to keep current config.\n", max)
		} else {
			fmt.Fprintf(out, "  Please enter a number between 1 and %d.\n", max)
		}
	}
}

func promptLine(out io.Writer, scanner *bufio.Scanner, label, defaultVal string) string {
	fmt.Fprintf(out, "%s: ", label)
	if !scanner.Scan() {
		return defaultVal
	}
	v := strings.TrimSpace(scanner.Text())
	if v == "" {
		return defaultVal
	}
	return v
}

func promptSecret(out io.Writer, scanner *bufio.Scanner, label string) string {
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		fmt.Fprintf(out, "%s: ", label)
		bytes, err := term.ReadPassword(fd)
		fmt.Fprintln(out)
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(bytes))
	}
	return promptLine(out, scanner, label, "")
}

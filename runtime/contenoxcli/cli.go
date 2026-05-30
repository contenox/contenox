// cli.go holds the contenox CLI entrypoint (Main), default constants, flags, and merge logic.
package contenoxcli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/internal/clikv"
	"github.com/contenox/agent/runtime/runtimetypes"
	"github.com/contenox/agent/runtime/version"
	"github.com/spf13/cobra"
)

// Version is an optional link-time override via
// -ldflags "-X github.com/contenox/agent/contenoxcli.Version=…"
// (e.g. distro packagers). When empty, CLIVersion uses runtime/version/version.txt.
var Version string

// CLIVersion returns the effective CLI version string (embedded file or link override).
func CLIVersion() string {
	return cliVersion()
}

func cliVersion() string {
	if v := strings.TrimSpace(Version); v != "" {
		return v
	}
	return version.Get()
}

const DefaultWorkspaceID = "00000000-0000-0000-0000-000000000002"

const (
	defaultOllama  = "http://127.0.0.1:11434"
	defaultModel   = "qwen2.5:7b"
	defaultContext = 0
	defaultTimeout = 5 * time.Minute
)

// reservedSubcommands are first-arg names that must not be treated as run input (Cobra or our subcommands).
var reservedSubcommands = map[string]bool{"init": true, "chat": true, "help": true, "completion": true, "session": true, "run": true, "tools": true, "mcp": true, "backend": true, "config": true, "model": true, "models": true, "doctor": true, "version": true, "state": true, "acp": true, "acpx": true, "setup": true}

// Main runs the contenox CLI: init subcommand or run (default) with optional positional input.
func Main() {
	args := os.Args[1:]
	// Only inject "run" when no reserved subcommand was given (so "contenox completion" and "contenox help" work).
	// Scan past leading flags (e.g. --db /path) to find the first non-flag argument.
	// Also skip injection when args contains only --help/-h so the root command shows its own help.
	onlyHelp := len(args) == 0
	if !onlyHelp {
		allRootFlags := true
		for _, a := range args {
			if a != "--help" && a != "-h" && a != "--version" && a != "-v" {
				allRootFlags = false
				break
			}
		}
		onlyHelp = allRootFlags
	}
	switch {
	case containsExperimentalACPFlag(args) && !firstNonFlagIsReserved(args):
		rootCmd.SetArgs(append([]string{"acp"}, args...))
	case !onlyHelp && !firstNonFlagIsReserved(args):
		rootCmd.SetArgs(append([]string{"run"}, args...))
	}
	if err := rootCmd.Execute(); err != nil {
		recordStartupFailure(err)
		fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
		os.Exit(1)
	}
}

func containsExperimentalACPFlag(args []string) bool {
	return slices.Contains(args, "--experimental-acp")
}

func recordStartupFailure(execErr error) {
	defer func() { _ = recover() }()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	dir := filepath.Join(home, ".contenox")
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		return
	}
	f, openErr := os.OpenFile(filepath.Join(dir, "telemetry.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if openErr != nil {
		return
	}
	defer f.Close()
	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo}))
	tr := libtracker.NewLogActivityTracker(logger)
	reportErr, _, end := tr.Start(context.Background(), "exec", "cli",
		"argv", strings.Join(os.Args[1:], " "),
		"version", CLIVersion(),
	)
	reportErr(execErr)
	end()
}

// firstNonFlagIsReserved scans args, skipping flags and their values, and returns
// true if the first positional argument is a reserved subcommand name.
func firstNonFlagIsReserved(args []string) bool {
	// Boolean flags that do NOT consume the next token as their value.
	// Without this list, `contenox --trace chat` would mistake "chat" for the
	// value of --trace and then forward it to the chat command as text input.
	boolFlags := map[string]bool{
		"--shell": true, "--trace": true, "--steps": true, "--raw": true,
		"--think": true, "--no-delete-models": true, "--editor": true,
		"-e": true, "-h": true, "--help": true, "-v": true, "--version": true,
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			// Explicit end of flags; next arg would be positional.
			if i+1 < len(args) {
				return reservedSubcommands[args[i+1]]
			}
			return false
		}
		if strings.HasPrefix(a, "--") {
			// Long flag: boolean flags and flag=value forms don't consume next token.
			if strings.Contains(a, "=") || boolFlags[a] {
				continue
			}
			i++ // this flag consumes the next token as its value
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			// Short flag: skip (simplified: assume it consumes next token if no value attached).
			if len(a) == 2 {
				i++ // skip value
			}
			continue
		}
		// First non-flag argument found.
		return reservedSubcommands[a]
	}
	return false
}

var rootCmd = &cobra.Command{
	Use:   "contenox",
	Short: "AI agent CLI: execute tasks using your LLM of choice.",
	Long: `Contenox is a local AI agent CLI that executes tasks on your machine using
filesystem and shell tools — driven by your LLM of choice.
No daemon, no cloud required. State is stored in SQLite.

  Quickstart:
    contenox setup                         # interactive wizard — pick provider, model, API key
    contenox init                          # scaffold .contenox/ with default chains
    contenox "list files in my home dir"   # one-shot natural language → shell

  Or register an LLM backend manually:
    # Fully embedded (no external server, no network, no API key):
    #   llama.cpp inference is compiled into the contenox binary.
    contenox backend add embedded --type local --url <path-to-gguf-or-hf-url>
    contenox config set default-provider local
    contenox config set default-model <model-name>

    # Local Ollama daemon
    ollama serve && ollama pull qwen2.5:7b
    contenox backend add ollama --type ollama
    contenox config set default-provider ollama
    contenox config set default-model qwen2.5:7b

    # Hosted Ollama Cloud
    contenox backend add ollama-cloud --type ollama --url https://ollama.com/api --api-key-env OLLAMA_API_KEY
    contenox config set default-provider ollama

    # Google Gemini (no GPU required)
    contenox backend add gemini --type gemini --api-key-env GEMINI_API_KEY
    contenox config set default-model  gemini-flash-latest
    contenox config set default-provider gemini

    # OpenAI
    contenox backend add openai --type openai --api-key-env OPENAI_API_KEY
    contenox config set default-model    gpt-4o-mini
    contenox config set default-provider openai

  Scope note:
    Backends and config are GLOBAL (stored in ~/.contenox/local.db).
    Chain files (.contenox/) are LOCAL to each project directory — like .git/.
    Run 'contenox init' once per project to create the local chain files.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Run a stateful chat session (default when no subcommand is given).",
	Long: `Send a message to the active chat session and get a response.
Input is passed as positional args, --input, or piped via stdin.

  contenox "what can you do?"
  echo "summarise README.md" | contenox
  contenox chat --shell "list files in the current dir"

Sessions persist conversation history across invocations (stored in SQLite).
Each session remembers previous messages so the model has context.
The first run auto-creates a "default" session. Manage sessions with:

  contenox session list              list active-scope sessions (* = active)
  contenox session list --all        list every session across the whole DB
  contenox session new <name>        create a new named session (becomes active)
  contenox session switch <name>     switch to a different session
  contenox session show [name|id]    print a session (active, by name, or by id)
  contenox session delete <name>     delete a session and all its messages
  contenox session workspaces        list workspaces and namespaces (whole DB)
  contenox session fork --summary    compact older history into a summary and continue
                                     in a new session (useful when context fills up)

Giving the model tools (file system and shell access):

  --local-exec-allowed-dir <dir>     allow local_fs tools inside <dir>
  --shell                            enable local_shell (command policy is defined in the chain)

Human-in-the-loop is on by default. The agent pauses for terminal approval before
write_file, sed, and local_shell calls. The active policy is defined in
~/.contenox/hitl-policy-default.json (override per workspace via
.contenox/hitl-policy-*.json or via 'contenox config set hitl-policy-name').

  --auto                             autonomous mode: disable approval prompts
                                     entirely. Use only in trusted environments
                                     or for non-interactive scripts.

Examples:
  # Chat with file system access to the current project:
  contenox chat --local-exec-allowed-dir . "summarise the README"

  # Shell access (policy comes from the chain's tools_policies; default chains allow common dev tools):
  contenox chat --shell "suggest a commit message from git diff"

  # Autonomous shell run — no approvals, runs everything (USE WITH CARE):
  contenox chat --shell --local-exec-allowed-dir . --auto "refactor main.go to use slog"

  # Trim context: only send last 10 messages from session history to the model:
  contenox chat --trim 10 "let's continue where we left off"

  # Show last 6 turns of the conversation after the reply:
  contenox chat --last 6 "hello"`,
	Args: cobra.ArbitraryArgs,
	RunE: runChat,
}

var initCmd = &cobra.Command{
	Use:   "init [provider]",
	Short: "Scaffold .contenox/ with default chain files.",
	Long: `Create the .contenox/ directory and populate it with default chain files.

This writes default-chain.json and default-run-chain.json.

After init, register a backend, make sure the runtime can see a model, then set your defaults:

  # Local Ollama:
  contenox backend add local --type ollama
  contenox config set default-model qwen2.5:7b

  # Hosted Ollama Cloud:
  contenox backend add ollama-cloud --type ollama --url https://ollama.com/api --api-key-env OLLAMA_API_KEY
  contenox config set default-model gpt-oss:20b

  # OpenAI:
  contenox backend add openai --type openai --api-key-env OPENAI_API_KEY
  contenox config set default-model gpt-5-mini

  # Google Gemini:
  contenox backend add gemini --type gemini --api-key-env GEMINI_API_KEY
  contenox config set default-model gemini-3.1-pro-preview

Use --force to overwrite existing files.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInitCmd,
}

// versionCmd prints the same line as `contenox --version` so `contenox version`
// is not mistaken for chat input (the default run command).
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the contenox CLI version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("%s version %s\n", cmd.Root().Name(), cmd.Root().Version)
	},
}

func init() {
	v := cliVersion()
	rootCmd.Version = v
	rootCmd.Short = fmt.Sprintf("AI agent CLI v%s: execute tasks using your LLM of choice.", v)
	// Cobra prints Long for --help when set; include version so it matches runtime/version/version.txt.
	rootCmd.Long = fmt.Sprintf("Version: %s\n\n%s", v, rootCmd.Long)

	// Run flags on root so "contenox --input x" and "contenox hi" both work.
	f := rootCmd.PersistentFlags()
	f.String("db", "", "SQLite database path (default: .contenox/local.db)")
	f.String("data-dir", "", "Override the .contenox data directory path")
	f.String("ollama", defaultOllama, "Ollama base URL")
	f.String("model", defaultModel, "Model name (task/chat/embed)")
	f.String("provider", "", "Provider type override (ollama, openai, vllm, gemini). Overrides config default_provider.")
	f.String("alt-model", "", "Alt model name (chains referencing {{var:alt_model}}). Overrides config default-alt-model.")
	f.String("alt-provider", "", "Alt provider type (chains referencing {{var:alt_provider}}). Overrides config default-alt-provider.")
	f.Int("context", defaultContext, "Context length")
	f.Bool("no-delete-models", true, "Legacy compatibility flag; OSS runtime model deletion is disabled.")
	f.String("chain", "", "Path to a task chain JSON file. Chains define the LLM workflow: which model, which tools, how to branch. Falls back to default_chain in config, then .contenox/default-chain.json")
	f.String("input", "", "Input for the chain (default: positional args or stdin if piped)")
	f.Bool("shell", false, "Enable the local_shell tools (use only in trusted environments)")
	f.String("local-exec-allowed-dir", "", "If set, local_shell may only run scripts/binaries under this directory")
	f.Duration("timeout", defaultTimeout, "Maximum execution time (e.g., 5m, 1h)")
	f.Bool("trace", false, "Stream task-step events to stderr in real time")

	f.Bool("steps", false, "Print execution steps after the result")
	f.Bool("raw", false, "Print full output (e.g. entire chat JSON)")
	f.Bool("think", false, "Print model reasoning trace to stderr (for thinking models)")
	f.BoolP("editor", "e", false, "Open $EDITOR (or $VISUAL, fallback nano) to compose the prompt; piped stdin is preloaded as reference")

	rootCmd.AddCommand(initCmd, chatCmd, sessionCmd, runCmd, toolsCmd, doctorCmd, versionCmd)
	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(backendCmd)
	rootCmd.AddCommand(configCmd)
	rootCmd.AddCommand(modelCmd)
	rootCmd.AddCommand(stateCmd)
	rootCmd.AddCommand(acpCmd)
	rootCmd.AddCommand(acpxCmd)
	rootCmd.AddCommand(setupCmd)

	rootCmd.InitDefaultHelpCmd() // so "contenox help" is handled by Cobra, not passed as run input
	initCmd.Flags().BoolP("force", "f", false, "Overwrite existing files")

	// Chat-specific local flags (not exposed globally).
	chatCmd.Flags().Int("trim", 0, "Only send the last N messages from session history to the model (0 = send all)")
	chatCmd.Flags().Int("last", 0, "Print last N user/assistant turns after the reply (0 = only print new reply)")
	chatCmd.Flags().Bool("auto", false, "Autonomous mode: disable HITL approval prompts. Default is HITL on; tools route through the active hitl-policy. Use --auto only in trusted/scripted contexts.")

}

// setupTelemetryLogging checks if the user has enabled file logging.
// If enabled, it sets up slog to write to both os.Stderr and ~/.contenox/telemetry.log.
// Returns a cleanup function to close the file.
func setupTelemetryLogging(ctx context.Context, store runtimetypes.Store, contenoxDir string) (func(), error) {
	enabledStr := clikv.Read(ctx, store, "telemetry-enabled")
	if enabledStr != "true" {
		return func() {}, nil
	}

	logPath := filepath.Join(contenoxDir, "telemetry.log")
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return func() {}, fmt.Errorf("failed to open telemetry log: %w", err)
	}

	mw := io.MultiWriter(os.Stderr, f)
	slog.SetDefault(slog.New(slog.NewTextHandler(mw, &slog.HandlerOptions{Level: slog.LevelInfo})))
	return func() { f.Close() }, nil
}

// ResolveContenoxDir finds the closest .contenox directory by walking up from the
// current working directory. If cmd is non-nil and --data-dir is set, that value
// is returned directly. Otherwise it walks up from cwd; if it hits the root
// without finding one, it returns the .contenox directory in the current working
// directory as a fallback.
func ResolveContenoxDir(cmd *cobra.Command) (string, error) {
	if cmd != nil {
		dataDir, _ := cmd.Root().PersistentFlags().GetString("data-dir")
		if dataDir != "" {
			return filepath.Abs(dataDir)
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	dir := cwd
	for {
		candidate := filepath.Join(dir, ".contenox")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			// Require workspace.id to be present — a .contenox/ without it is
			// not a valid workspace (e.g. a backup or pre-init directory).
			// Keep walking up so callers get a proper workspace or the cwd fallback.
			if _, werr := os.Stat(filepath.Join(candidate, "workspace.id")); werr == nil {
				return candidate, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Hit root without finding it. Fallback to cwd/.contenox
			return filepath.Join(cwd, ".contenox"), nil
		}
		dir = parent
	}
}

func ResolveWorkspaceID(contenoxDir string) string {
	data, err := os.ReadFile(filepath.Join(contenoxDir, "workspace.id"))
	if err != nil {
		return DefaultWorkspaceID
	}
	id := strings.TrimSpace(string(data))
	if id == "" {
		return DefaultWorkspaceID
	}
	return id
}

func runInitCmd(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	provider := ""
	if len(args) > 0 {
		provider = args[0]
	}
	contenoxDir, err := ResolveContenoxDir(cmd)
	if err != nil {
		return fmt.Errorf("failed to resolve .contenox dir: %w", err)
	}
	return RunInit(cmd.OutOrStdout(), cmd.ErrOrStderr(), force, provider, contenoxDir)
}

func runChat(cmd *cobra.Command, args []string) error {
	flags := cmd.Root().Flags()
	useEditor, _ := flags.GetBool("editor")

	if len(args) == 1 && args[0] == "help" && !flags.Changed("input") && !useEditor {
		_ = cmd.Help()
		return nil
	}

	// No subcommand, no input, no editor, and no piped stdin: show help and exit 0.
	if len(args) == 0 && !flags.Changed("input") && !useEditor {
		if stat, err := os.Stdin.Stat(); err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
			_ = cmd.Usage()
			return nil
		}
	}

	contenoxDir, err := ResolveContenoxDir(cmd)
	if err != nil {
		return fmt.Errorf("failed to resolve .contenox dir: %w", err)
	}

	// Resolve DB path (needed for KV reads below).
	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return err
	}
	dbCtx := libtracker.WithNewRequestID(context.Background())
	db, err := OpenDBAt(dbCtx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	closeLogs, err := setupTelemetryLogging(dbCtx, runtimetypes.New(db.WithoutTransaction()), contenoxDir)
	if err != nil {
		slog.Warn("Failed to setup telemetry logging", "error", err)
	}
	defer closeLogs()

	store := runtimetypes.New(db.WithoutTransaction())

	changed := func(name string) bool { return flags.Changed(name) }

	// Resolve model: flag > SQLite KV > hardcoded default.
	effectiveModel, _ := flags.GetString("model")
	if !changed("model") || effectiveModel == defaultModel {
		if kv, _ := getConfigKV(dbCtx, store, "default-model"); kv != "" {
			effectiveModel = kv
		}
	}

	effectiveDefaultProvider := ""
	if kv, _ := getConfigKV(dbCtx, store, "default-provider"); kv != "" {
		effectiveDefaultProvider = kv
	}
	if changed("provider") {
		effectiveDefaultProvider, _ = flags.GetString("provider")
	}

	effectiveAltModel := ""
	if kv, _ := getConfigKV(dbCtx, store, "default-alt-model"); kv != "" {
		effectiveAltModel = kv
	}
	if changed("alt-model") {
		effectiveAltModel, _ = flags.GetString("alt-model")
	}

	effectiveAltProvider := ""
	if kv, _ := getConfigKV(dbCtx, store, "default-alt-provider"); kv != "" {
		effectiveAltProvider = kv
	}
	if changed("alt-provider") {
		effectiveAltProvider, _ = flags.GetString("alt-provider")
	}

	effectiveContext, _ := flags.GetInt("context")
	effectiveNoDeleteModels, _ := flags.GetBool("no-delete-models")

	effectiveChain, _ := flags.GetString("chain")
	if effectiveChain == "" && !changed("chain") {
		if kv, _ := getConfigKV(dbCtx, store, "default-chain"); kv != "" {
			effectiveChain = kv
			if !filepath.IsAbs(effectiveChain) {
				if resolved, rerr := lookupSystemFile(contenoxDir, effectiveChain); rerr == nil {
					effectiveChain = resolved
				} else {
					effectiveChain = filepath.Join(contenoxDir, effectiveChain)
				}
			}
		}
	}
	if effectiveChain == "" && !changed("chain") {
		if resolved, rerr := lookupSystemFile(contenoxDir, "default-chain.json"); rerr == nil {
			effectiveChain = resolved
		}
	}
	if effectiveChain == "" {
		fmt.Fprintln(os.Stderr, "No default chain found in .contenox/ (workspace) or ~/.contenox/.")
		fmt.Fprintln(os.Stderr, "Run 'contenox init' to scaffold one, or pass --chain explicitly.")
		return errChainRequired
	}

	effectiveEnableLocalExec, _ := flags.GetBool("shell")
	effectiveLocalExecAllowedDir, _ := flags.GetString("local-exec-allowed-dir")

	effectiveTracing, _ := flags.GetBool("trace")
	effectiveSteps, _ := flags.GetBool("steps")
	effectiveRaw, _ := flags.GetBool("raw")

	var inputValue string
	var inputPassed bool
	if useEditor {
		var seed []byte
		if data, ok, err := readStdinIfAvailable(maxCLIStdinBytes); err != nil {
			return err
		} else if ok {
			seed = []byte(data)
		}
		prompt, err := captureFromEditor(seed, effectiveModel)
		if err != nil {
			if errors.Is(err, errEmptyPrompt) {
				fmt.Fprintln(cmd.ErrOrStderr(), "aborted due to empty prompt")
				return errPromptAborted
			}
			return err
		}
		inputValue = prompt
		inputPassed = true
	} else if changed("input") {
		inputValue, _ = flags.GetString("input")
		inputPassed = true
	} else if len(args) > 0 {
		inputValue = strings.Join(args, " ")
	}

	timeout, _ := flags.GetDuration("timeout")
	timeoutCtx, timeoutCancel := context.WithTimeout(libtracker.WithNewRequestID(context.Background()), timeout)
	defer timeoutCancel()

	// Use signal.NotifyContext so cleanup is automatic when the cmd returns;
	// avoids leaking a goroutine blocked forever on <-sigCh.
	ctx, stop := signal.NotifyContext(timeoutCtx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	effectiveThink, _ := flags.GetBool("think")
	autoMode, _ := cmd.Flags().GetBool("auto")
	effectiveHITL := !autoMode
	historyTrim, _ := cmd.Flags().GetInt("trim")
	lastN, _ := cmd.Flags().GetInt("last")

	opts := chatOpts{
		EffectiveDB:                  dbPath,
		EffectiveChain:               effectiveChain,
		EffectiveDefaultModel:        effectiveModel,
		EffectiveDefaultProvider:     effectiveDefaultProvider,
		EffectiveAltDefaultModel:     effectiveAltModel,
		EffectiveAltDefaultProvider:  effectiveAltProvider,
		EffectiveContext:             effectiveContext,
		EffectiveNoDeleteModels:      effectiveNoDeleteModels,
		EffectiveEnableLocalExec:     effectiveEnableLocalExec,
		EffectiveLocalExecAllowedDir: effectiveLocalExecAllowedDir,
		EffectiveTracing:             effectiveTracing,
		EffectiveSteps:               effectiveSteps,
		EffectiveHITL:                effectiveHITL,
		EffectiveRaw:                 effectiveRaw,
		EffectiveThink:               effectiveThink,
		HistoryTrim:                  historyTrim,
		LastN:                        lastN,
		InputValue:                   inputValue,
		InputFlagPassed:              inputPassed,
		ContenoxDir:                  contenoxDir,
	}
	return execChat(ctx, db, opts, cmd.OutOrStdout(), cmd.ErrOrStderr())
}

// Sentinel errors so RunE can return and main can os.Exit(1).
var (
	errChainRequired = &exitError{1}
	errPromptAborted = &exitError{1}
)

type exitError struct{ code int }

func (e *exitError) Error() string { return "exit" }

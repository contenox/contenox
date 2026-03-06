// cli.go holds the contenox CLI entrypoint (Main), default constants, flags, and merge logic.
package contenoxcli

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/contenox/contenox/libtracker"
	"github.com/spf13/cobra"
)

// Version is set at build time via -ldflags "-X github.com/contenox/contenox/internal/contenoxcli.Version=vX.Y.Z".
// Falls back to "dev" when building without the flag (e.g. go run).
var Version = "dev"

const localTenantID = "00000000-0000-0000-0000-000000000001"

const (
	defaultOllama  = "http://127.0.0.1:11434"
	defaultModel   = "qwen2.5:7b"
	defaultContext = 2048
	defaultTimeout = 5 * time.Minute
)

// reservedSubcommands are first-arg names that must not be treated as run input (Cobra or our subcommands).
var reservedSubcommands = map[string]bool{"init": true, "chat": true, "help": true, "completion": true, "session": true, "plan": true, "run": true, "hook": true}

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
	if !onlyHelp && !firstNonFlagIsReserved(args) {
		rootCmd.SetArgs(append([]string{"chat"}, args...))
	}
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// firstNonFlagIsReserved scans args, skipping flags and their values, and returns
// true if the first positional argument is a reserved subcommand name.
func firstNonFlagIsReserved(args []string) bool {
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
			// Long flag: if it has no '=' it consumes the next token as its value.
			if !strings.Contains(a, "=") {
				i++ // skip value
			}
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
	Short: "AI agent CLI: plan and execute tasks using your LLM of choice.",
	Long: `Contenox is a local AI agent CLI that plans and executes multi-step tasks on your
machine using filesystem and shell tools — driven by your LLM of choice.
No daemon, no cloud required. State is stored in SQLite.

  Quickstart:
    contenox init                         # scaffold .contenox/ with config + chain
    contenox list files in my home dir    # one-shot natural language → shell
    contenox plan new "some multi-step goal"  # create an autonomous plan
    contenox plan next --auto             # execute until done

  LLM providers (edit .contenox/config.yaml after 'contenox init'):
    Local (Ollama):  ollama serve && ollama pull qwen2.5:7b
    OpenAI:          set OPENAI_API_KEY (auto-declared, no config edit needed)
    Gemini:          set GEMINI_API_KEY (auto-declared, no config edit needed)

  For contenox plan, the model MUST support tool calling.
  Run 'contenox init' and open .contenox/config.yaml for full provider examples.`,
	SilenceUsage: true,
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Run a stateful chat session (default when no subcommand is given).",
	Long:  `Run a stateful task chain with chat history. You can pass input as positional args (e.g. contenox hi) or via --input.`,
	Args:  cobra.ArbitraryArgs,
	RunE:  runChat,
}

var initCmd = &cobra.Command{
	Use:   "init [provider]",
	Short: "Scaffold .contenox/ (config and default chain).",
	Long: `Create .contenox/config.yaml and .contenox/default-chain.json.

Optional provider argument sets the default LLM backend in config.yaml:
  ollama   Local model via Ollama (default)
  openai   OpenAI — set OPENAI_API_KEY
  gemini   Google Gemini — set GEMINI_API_KEY

Use --force to overwrite existing files.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runInitCmd,
}

func init() {
	// Wire the ldflags-injected version into cobra's built-in --version/-v flag.
	// Must be done here (not in the struct literal) so the ldflags value is used.
	rootCmd.Version = Version

	// Run flags on root so "contenox --input x" and "contenox hi" both work.
	f := rootCmd.PersistentFlags()
	f.String("db", "", "SQLite database path (default: .contenox/local.db)")
	f.String("ollama", defaultOllama, "Ollama base URL")
	f.String("model", defaultModel, "Model name (task/chat/embed)")
	f.Int("context", defaultContext, "Context length")
	f.Bool("no-delete-models", true, "Do not delete Ollama models that are not declared (default true for contenox)")
	f.String("chain", "", "Path to a task chain JSON file. Chains define the LLM workflow: which model, which hooks, how to branch. Falls back to default_chain in config, then .contenox/default-chain.json")
	f.String("input", "", "Input for the chain (default: positional args or stdin if piped)")
	f.Bool("shell", false, "Enable the local_shell hook (use only in trusted environments)")
	f.String("local-exec-allowed-dir", "", "If set, local_shell may only run scripts/binaries under this directory")
	f.String("local-exec-allowed-commands", "", "Comma-separated list of allowed executable paths/names for local_shell")
	f.String("local-exec-denied-commands", "", "Comma-separated list of denied executable basenames/paths for local_shell")
	f.Duration("timeout", defaultTimeout, "Maximum execution time (e.g., 5m, 1h)")
	f.Bool("trace", false, "Enable operation telemetry on stderr")

	f.Bool("steps", false, "Print execution steps after the result")
	f.Bool("raw", false, "Print full output (e.g. entire chat JSON)")
	f.Bool("think", false, "Print model reasoning trace to stderr (for thinking models)")

	rootCmd.AddCommand(initCmd, chatCmd, sessionCmd, planCmd, runCmd, hookCmd)

	rootCmd.InitDefaultHelpCmd() // so "contenox help" is handled by Cobra, not passed as run input
	initCmd.Flags().BoolP("force", "f", false, "Overwrite existing files")
}

func runInitCmd(cmd *cobra.Command, args []string) error {
	force, _ := cmd.Flags().GetBool("force")
	provider := ""
	if len(args) > 0 {
		provider = args[0]
	}
	RunInit(force, provider)
	return nil
}

func runChat(cmd *cobra.Command, args []string) error {
	// No subcommand, no input, and no piped stdin: show help and exit 0.
	flags := cmd.Root().Flags()
	if len(args) == 0 && !flags.Changed("input") {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			_ = cmd.Root().Usage()
			return nil
		}
	}

	cfg, configPath, err := loadLocalConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		return err
	}

	var contenoxDir string
	if configPath != "" {
		contenoxDir = filepath.Dir(configPath)
	} else {
		cwd, _ := os.Getwd()
		contenoxDir = filepath.Join(cwd, ".contenox")
	}

	flags = cmd.Root().Flags()
	changed := func(name string) bool { return flags.Changed(name) }

	effectiveDB, _ := flags.GetString("db")
	if effectiveDB == "" && !changed("db") && cfg.DB != "" {
		effectiveDB = cfg.DB
	}
	if effectiveDB == "" {
		effectiveDB = filepath.Join(contenoxDir, "local.db")
	}

	effectiveOllama, _ := flags.GetString("ollama")
	if effectiveOllama == defaultOllama && !changed("ollama") && cfg.Ollama != "" {
		effectiveOllama = cfg.Ollama
	}

	effectiveModel, _ := flags.GetString("model")
	if effectiveModel == defaultModel && !changed("model") && cfg.Model != "" {
		effectiveModel = cfg.Model
	}

	effectiveContext, _ := flags.GetInt("context")
	if effectiveContext == defaultContext && !changed("context") && cfg.Context != nil {
		effectiveContext = *cfg.Context
	}

	effectiveNoDeleteModels, _ := flags.GetBool("no-delete-models")
	if effectiveNoDeleteModels && !changed("no-delete-models") && cfg.NoDeleteModels != nil {
		effectiveNoDeleteModels = *cfg.NoDeleteModels
	}

	effectiveChain, _ := flags.GetString("chain")
	if effectiveChain == "" && !changed("chain") && cfg.DefaultChain != "" {
		effectiveChain = filepath.Join(contenoxDir, cfg.DefaultChain)
	}
	if effectiveChain == "" && !changed("chain") {
		wellKnown := filepath.Join(contenoxDir, "default-chain.json")
		if _, err := os.Stat(wellKnown); err == nil {
			effectiveChain = wellKnown
		}
	}
	if effectiveChain == "" {
		slog.Error("No chain file specified", "hint", "use --chain <path>, or set default_chain in .contenox/config.yaml, or add .contenox/default-chain.json")
		return errChainRequired
	}

	effectiveEnableLocalExec, _ := flags.GetBool("shell")
	if !effectiveEnableLocalExec && !changed("shell") && cfg.EnableLocalExec != nil {
		effectiveEnableLocalExec = *cfg.EnableLocalExec
	}

	effectiveLocalExecAllowedDir, _ := flags.GetString("local-exec-allowed-dir")
	if effectiveLocalExecAllowedDir == "" && !changed("local-exec-allowed-dir") && cfg.LocalExecAllowedDir != "" {
		effectiveLocalExecAllowedDir = cfg.LocalExecAllowedDir
	}

	effectiveLocalExecAllowedCommands, _ := flags.GetString("local-exec-allowed-commands")
	if effectiveLocalExecAllowedCommands == "" && !changed("local-exec-allowed-commands") && cfg.LocalExecAllowedCommands != "" {
		effectiveLocalExecAllowedCommands = cfg.LocalExecAllowedCommands
	}

	var effectiveLocalExecDeniedCommands []string
	if changed("local-exec-denied-commands") {
		denied, _ := flags.GetString("local-exec-denied-commands")
		effectiveLocalExecDeniedCommands = splitAndTrim(denied, ",")
	} else if len(cfg.LocalExecDeniedCommands) > 0 {
		effectiveLocalExecDeniedCommands = cfg.LocalExecDeniedCommands
	}

	effectiveTracing, _ := flags.GetBool("trace")
	if !effectiveTracing && !changed("trace") && cfg.Tracing != nil {

		effectiveTracing = *cfg.Tracing
	}

	effectiveSteps, _ := flags.GetBool("steps")
	if !effectiveSteps && !changed("steps") && cfg.Steps != nil {
		effectiveSteps = *cfg.Steps
	}

	effectiveRaw, _ := flags.GetBool("raw")
	if !effectiveRaw && !changed("raw") && cfg.Raw != nil {
		effectiveRaw = *cfg.Raw
	}

	resolvedBackends, effectiveDefaultProvider, effectiveDefaultModel := resolveEffectiveBackends(cfg, effectiveOllama, effectiveModel)
	if changed("model") {
		effectiveDefaultModel = effectiveModel
	}

	if effectiveEnableLocalExec && effectiveLocalExecAllowedDir != "" && effectiveLocalExecAllowedCommands != "" {
		allowedDir, err := filepath.Abs(effectiveLocalExecAllowedDir)
		if err != nil {
			slog.Error("Invalid allowed directory", "error", err)
			return err
		}
		commands := splitAndTrim(effectiveLocalExecAllowedCommands, ",")
		for _, c := range commands {
			if filepath.IsAbs(c) && !strings.HasPrefix(c, allowedDir) {
				slog.Error("Command path not inside allowed directory", "command", c, "allowed_dir", allowedDir)
				return errInvalidConfig
			}
		}
	}

	// Input: --input overrides positional args; else positionals joined; else empty (run() will try stdin).
	var inputValue string
	var inputPassed bool
	if changed("input") {
		inputValue, _ = flags.GetString("input")
		inputPassed = true
	} else if len(args) > 0 {
		inputValue = strings.Join(args, " ")
		inputPassed = true
	}

	timeout, _ := flags.GetDuration("timeout")
	ctx, cancel := context.WithTimeout(libtracker.WithNewRequestID(context.Background()), timeout)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		slog.Warn("Received interrupt, shutting down...")
		cancel()
	}()

	opts := chatOpts{
		EffectiveDB:                       effectiveDB,
		EffectiveChain:                    effectiveChain,
		EffectiveDefaultModel:             effectiveDefaultModel,
		EffectiveDefaultProvider:          effectiveDefaultProvider,
		EffectiveContext:                  effectiveContext,
		EffectiveNoDeleteModels:           effectiveNoDeleteModels,
		EffectiveEnableLocalExec:          effectiveEnableLocalExec,
		EffectiveLocalExecAllowedDir:      effectiveLocalExecAllowedDir,
		EffectiveLocalExecAllowedCommands: effectiveLocalExecAllowedCommands,
		EffectiveLocalExecDeniedCommands:  effectiveLocalExecDeniedCommands,
		EffectiveTracing:                  effectiveTracing,
		EffectiveSteps:                    effectiveSteps,
		EffectiveRaw:                      effectiveRaw,
		EffectiveThink:                    func() bool { v, _ := flags.GetBool("think"); return v }(),
		InputValue:                        inputValue,
		InputFlagPassed:                   inputPassed,
		Cfg:                               cfg,
		ResolvedBackends:                  resolvedBackends,
		ContenoxDir:                       contenoxDir,
	}
	execChat(ctx, opts)
	return nil
}

// Sentinel errors so RunE can return and main can os.Exit(1).
var (
	errChainRequired = &exitError{1}
	errInvalidConfig = &exitError{1}
)

type exitError struct{ code int }

func (e *exitError) Error() string { return "exit" }

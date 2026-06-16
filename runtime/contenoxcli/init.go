// init.go implements the contenox init subcommand (scaffold .contenox/).
package contenoxcli

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/internal/setupcheck"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/runtimestate"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
)

//go:embed chain-contenox.json
var initChain string

//go:embed chain-run.json
var initRunChain string

//go:embed chain-compact.json
var initCompactChain string

//go:embed chain-acp.json
var initACPChain string

//go:embed chain-acpx.json
var initACPXChain string

//go:embed chain-fim.json
var initFIMChain string

// blessedChainHashes is a map of file basenames to a list of known-good SHA256
// checksums from previous versions. When the --update flag is used, if a file's
// current checksum matches one of these, it is safe to overwrite it.
var blessedChainHashes = map[string][]string{}

// headlessACPChainFilename is the on-disk name the acpx (OpenClaw / untrusted
// driver) profile loads its chain from, parallel to default-acp-chain.json for
// the acp profile.
const headlessACPChainFilename = "headless-acp-chain.json"

// seedHeadlessACPChainIfMissing writes the embedded acpx chain to contenoxDir
// only when absent. It never overwrites a user-edited file, and a failure here
// leaves the file absent so LoadChainRegistryFrom still fails closed rather
// than the acpx profile silently running a different chain.
func seedHeadlessACPChainIfMissing(contenoxDir string) error {
	dst := filepath.Join(contenoxDir, headlessACPChainFilename)
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(contenoxDir, 0750); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(initACPXChain), 0644)
}

// seedACPChainIfMissing writes the default-acp-chain.json preset when it is
// absent, so the `acp` profile is self-sufficient on a fresh install the same
// way `acpx` is via seedHeadlessACPChainIfMissing. Without this, a clean
// environment that never ran `contenox init`/`--setup` hard-errors at launch
// in LoadChainRegistryFrom (the registry validator runs in exactly such an
// isolated HOME), so the ACP transport never starts and `initialize` is never
// answered.
func seedACPChainIfMissing(contenoxDir string) error {
	dst := filepath.Join(contenoxDir, "default-acp-chain.json")
	if _, err := os.Stat(dst); err == nil {
		return nil
	}
	if err := os.MkdirAll(contenoxDir, 0750); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(initACPChain), 0644)
}

// providerConfig holds the provider-specific values used during init.
type providerConfig struct {
	name         string
	defaultModel string
	envKey       string
}

var providerConfigs = map[string]providerConfig{
	"ollama": {
		name:         "Ollama (local)",
		defaultModel: defaultModel,
		envKey:       "",
	},
	"gemini": {
		name:         "Google Gemini",
		defaultModel: "gemini-3.1-pro-preview",
		envKey:       "GEMINI_API_KEY",
	},
	"openai": {
		name:         "OpenAI",
		defaultModel: "gpt-5-mini",
		envKey:       "OPENAI_API_KEY",
	},
	"anthropic": {
		name:         "Anthropic (direct)",
		defaultModel: "claude-sonnet-4-5",
		envKey:       "ANTHROPIC_API_KEY",
	},
	"mistral": {
		name:         "Mistral (direct)",
		defaultModel: "mistral-large-latest",
		envKey:       "MISTRAL_API_KEY",
	},
	"openrouter": {
		name:         "OpenRouter",
		defaultModel: "deepseek/deepseek-chat-v3-5",
		envKey:       "OPENROUTER_API_KEY",
	},
	"bedrock": {
		name:         "AWS Bedrock",
		defaultModel: "anthropic.claude-3-5-sonnet-20241022-v2:0",
		envKey:       "", // ambient AWS credential chain
	},
	"llama": {
		name:         "Llama.cpp (GGUF)",
		defaultModel: "",
		envKey:       "",
	},
	"vertex-google": {
		name:         "Google Vertex AI (Gemini)",
		defaultModel: "gemini-flash-latest",
		envKey:       "",
	},
}

// ensureLlamaBackend creates the implicit llama backend if none exists.
// The llama backend is always-present infrastructure pointed at ~/.contenox/models/;
// the user only needs to populate it via `contenox model pull`. Idempotent.
func ensureLlamaBackend(out io.Writer) error {
	if hasBackendOfType("llama") || hasBackendOfType("local") {
		return nil
	}
	homeDir, err := globalContenoxDir()
	if err != nil {
		return err
	}
	modelsDir := filepath.Join(homeDir, "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		return fmt.Errorf("create models dir: %w", err)
	}
	dbPath, err := globalDBPath()
	if err != nil {
		return err
	}
	ctx := libtracker.WithNewRequestID(context.Background())
	db, err := OpenDBAt(ctx, dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	svc := backendservice.New(db)
	if err := svc.Create(ctx, &runtimetypes.Backend{
		ID:      uuid.NewString(),
		Name:    "llama",
		BaseURL: modelsDir,
		Type:    "llama",
	}); err != nil {
		return fmt.Errorf("create llama backend: %w", err)
	}
	fmt.Fprintf(out, "  Registered llama backend -> %s\n", modelsDir)
	return nil
}

// hasBackendOfType returns true when the local DB already contains at least one
// backend whose Type matches the given provider string.
func hasBackendOfType(providerType string) bool {
	dbPath, err := globalDBPath()
	if err != nil {
		return false
	}
	db, err := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath)
	if err != nil {
		return false
	}
	defer db.Close()
	svc := backendservice.New(db)
	backends, err := svc.List(libtracker.WithNewRequestID(context.Background()), nil, 100)
	if err != nil {
		return false
	}
	for _, b := range backends {
		if strings.EqualFold(b.Type, providerType) {
			return true
		}
	}
	return false
}

// RunGlobalInit ensures ~/.contenox/ has chain files, HITL policies, and a llama backend.
// Unlike RunInit it does NOT create a workspace-scoped .contenox/ directory.
func RunGlobalInit(out io.Writer) error {
	homeDir, err := globalContenoxDir()
	if err != nil {
		return fmt.Errorf("could not resolve ~/.contenox: %w", err)
	}
	if err := os.MkdirAll(homeDir, 0o750); err != nil {
		return fmt.Errorf("create ~/.contenox: %w", err)
	}
	writeFile := func(path, content string) error {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", path, err)
		}
		fmt.Fprintf(out, "  Created %s\n", path)
		return nil
	}
	if err := writeFile(filepath.Join(homeDir, "default-chain.json"), initChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, "default-run-chain.json"), initRunChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, "chain-compact.json"), initCompactChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, "default-acp-chain.json"), initACPChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, headlessACPChainFilename), initACPXChain); err != nil {
		return err
	}
	if err := writeEmbeddedHITLPolicies(homeDir, false); err != nil {
		return err
	}
	if err := ensureLlamaBackend(out); err != nil {
		fmt.Fprintf(out, "  warning: could not register llama backend: %v\n", err)
	}
	return nil
}

// RunInit scaffolds .contenox/ with default chain files.
// provider is "" (defaults to the already-configured provider or "llama"), "ollama", "gemini", "openai", or "llama".
// contenoxDir is the target data directory (e.g. from --data-dir or the default .contenox/).
func RunInit(out, errOut io.Writer, force, update bool, provider string, contenoxDir string) error {
	provider = modelrepo.CanonicalBackendType(provider)
	if provider == "" {
		// Default to the provider already configured in the database so that
		// re-running init doesn't show irrelevant setup steps.
		if dbPath, gpErr := globalDBPath(); gpErr == nil {
			if db, openErr := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath); openErr == nil {
				store := runtimetypes.New(db.WithoutTransaction())
				if cur, err := getConfigKV(libtracker.WithNewRequestID(context.Background()), store, "default-provider"); err == nil && cur != "" {
					cur = modelrepo.CanonicalBackendType(cur)
					if _, known := providerConfigs[cur]; known {
						provider = cur
					}
				}
				db.Close()
			}
		}
		if provider == "" {
			provider = "llama"
		}
	}

	pc, ok := providerConfigs[provider]
	if !ok {
		return fmt.Errorf("unknown provider %q — valid options: ollama, openai, openrouter, gemini, anthropic, mistral, bedrock, llama, vertex-google", provider)
	}
	if err := os.MkdirAll(contenoxDir, 0750); err != nil {
		return fmt.Errorf("failed to create .contenox directory: %w", err)
	}
	wsPath := filepath.Join(contenoxDir, "workspace.id")
	if _, err := os.Stat(wsPath); os.IsNotExist(err) {
		_ = os.WriteFile(wsPath, []byte(uuid.NewString()), 0o644)
	}
	writeFile := func(path, content string) error {
		// If the file doesn't exist, we always write it.
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", path, err)
			}
			fmt.Fprintf(out, "  Created %s\n", path)
			return nil
		}

		// If we're forcing, we always overwrite.
		if force {
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", path, err)
			}
			fmt.Fprintf(out, "  Overwrote %s (--force)\n", path)
			return nil
		}

		// If --update is passed, we check the checksum and overwrite if it's a known-good, unmodified file.
		if update {
			basename := filepath.Base(path)
			if knownHashes, ok := blessedChainHashes[basename]; ok {
				data, err := os.ReadFile(path)
				if err != nil {
					return fmt.Errorf("failed to read %s for update check: %w", path, err)
				}
				hash := sha256.Sum256(data)
				currentHash := hex.EncodeToString(hash[:])

				for _, knownHash := range knownHashes {
					if currentHash == knownHash {
						// Checksum matches, safe to overwrite.
						if err := os.WriteFile(path, []byte(content), 0644); err != nil {
							return fmt.Errorf("failed to write %s: %w", path, err)
						}
						fmt.Fprintf(out, "  Updated %s\n", path)
						return nil
					}
				}
			}
			// If we're here, the file was either not in the blessed list or the checksum didn't match.
			// We don't overwrite it, but we also don't print a scary "already exists" message.
			fmt.Fprintf(out, "  Skipped %s (has been modified)\n", path)
			return nil
		}

		// Default case: file exists, no --force, no --update. Do nothing.
		fmt.Fprintf(out, "  %s already exists (use --force to overwrite or --update to refresh)\n", path)
		return nil
	}

	homeDir, hdErr := globalContenoxDir()
	if hdErr != nil {
		return fmt.Errorf("could not resolve ~/.contenox: %w", hdErr)
	}
	if err := writeFile(filepath.Join(homeDir, "default-chain.json"), initChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, "default-run-chain.json"), initRunChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, "chain-compact.json"), initCompactChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, "default-acp-chain.json"), initACPChain); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(homeDir, headlessACPChainFilename), initACPXChain); err != nil {
		return err
	}
	if err := writeEmbeddedHITLPolicies(homeDir, force); err != nil {
		return err
	}

	// The llama backend is always-present infrastructure. Create it if missing
	// so the user only ever needs to pull a model — never wire up a backend.
	if err := ensureLlamaBackend(out); err != nil {
		fmt.Fprintf(errOut, "  warning: could not register llama backend: %v\n", err)
	}

	// Make llama the default provider when nothing else is configured.
	// User-set values are not overwritten.
	if dbPath, gpErr := globalDBPath(); gpErr == nil {
		if db, openErr := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath); openErr == nil {
			ctx := libtracker.WithNewRequestID(context.Background())
			store := runtimetypes.New(db.WithoutTransaction())
			if cur, _ := getConfigKV(ctx, store, "default-provider"); cur == "" && provider == "llama" {
				workspaceID := ResolveWorkspaceID(contenoxDir)
				_ = clikv.WriteConfig(ctx, store, workspaceID, "default-provider", "llama")
			}
			db.Close()
		}
	}

	fmt.Fprintln(out, "Done.")
	fmt.Fprintln(out, "")

	// Surface the currently configured model so users immediately know
	// if they have a stale entry from a previous install.
	if dbPath, gpErr := globalDBPath(); gpErr == nil {
		if db, openErr := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath); openErr == nil {
			store := runtimetypes.New(db.WithoutTransaction())
			ctx := libtracker.WithNewRequestID(context.Background())
			curModel, err := getConfigKV(ctx, store, "default-model")
			if err != nil {
				return err
			}
			curProvider, err := getConfigKV(ctx, store, "default-provider")
			if err != nil {
				return err
			}
			db.Close()
			if curModel != "" || curProvider != "" {
				fmt.Fprintln(out, "Current config (from ~/.contenox/local.db):")
				if curProvider != "" {
					fmt.Fprintf(out, "  default-provider = %s\n", curProvider)
				}
				if curModel != "" {
					fmt.Fprintf(out, "  default-model    = %s\n", curModel)
				}
				fmt.Fprintln(out, "  To change: contenox config set default-model <model>")
				fmt.Fprintln(out, "")
			}
		}
	}

	// Resolve API key status (env or KV store) — used both for the status line and to
	// suppress the "register backend" step when the backend is already configured.
	var envVal string
	var kvHasKey bool
	if pc.envKey != "" {
		envVal = os.Getenv(pc.envKey)
		if envVal == "" {
			if dbPath, gpErr := globalDBPath(); gpErr == nil {
				if db, openErr := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath); openErr == nil {
					store := runtimetypes.New(db.WithoutTransaction())
					var cfg runtimestate.ProviderConfig
					kvKey := runtimestate.ProviderKeyPrefix + strings.ToLower(provider)
					if err := store.GetKV(libtracker.WithNewRequestID(context.Background()), kvKey, &cfg); err == nil && cfg.APIKey != "" {
						kvHasKey = true
					}
					db.Close()
				}
			}
		}
		switch {
		case envVal != "":
			fmt.Fprintf(out, "✓  %s API key detected (%s).\n\n", pc.name, pc.envKey)
		case kvHasKey:
			fmt.Fprintf(out, "✓  %s API key stored in local.db (set %s to use a different key).\n\n", pc.name, pc.envKey)
		default:
			fmt.Fprintf(out, "⚠️  %s API key not found in environment.\n", pc.name)
			fmt.Fprintf(out, "   Set it before running contenox:\n\n")
			fmt.Fprintf(out, "     export %s=your-key-here\n\n", pc.envKey)
		}
	}
	backendReady := kvHasKey || envVal != ""

	fmt.Fprintln(out, "Next steps:")
	fmt.Fprintln(out, "")
	chatStep := 3
	switch provider {
	case "vertex-google":
		fmt.Fprintln(out, "  1. Authenticate with Google Cloud:")
		fmt.Fprintln(out, "       export GOOGLE_CLOUD_PROJECT=my-project-id")
		fmt.Fprintln(out, "       gcloud auth application-default login --project $GOOGLE_CLOUD_PROJECT")
		fmt.Fprintln(out, "")
		fmt.Fprintf(out, "  2. Register the %s backend:\n", pc.name)
		fmt.Fprintf(out, "       contenox backend add %s --type %s \\\n", provider, provider)
		fmt.Fprintln(out, `         --url "https://us-central1-aiplatform.googleapis.com/v1/projects/$GOOGLE_CLOUD_PROJECT/locations/us-central1"`)
		fmt.Fprintln(out, "       contenox doctor")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  3. Set defaults:")
		fmt.Fprintf(out, "       contenox config set default-provider %s\n", provider)
		fmt.Fprintf(out, "       contenox config set default-model %s\n", pc.defaultModel)
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  Get started with Vertex AI: https://cloud.google.com/vertex-ai/generative-ai/docs/start/quickstarts")
		fmt.Fprintln(out, "")
		chatStep = 4
	case "llama":
		fmt.Fprintln(out, "  1. Pull a model (choose by available VRAM):")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "       VRAM     Model              Q4 size   Notes")
		fmt.Fprintln(out, "       ~2 GB    granite-3.2-2b     ~1-2 GB   good tool use")
		fmt.Fprintln(out, "       ~3 GB    qwen3-4b           ~3 GB")
		fmt.Fprintln(out, "       ~3 GB    gemma4-e2b         ~3.2 GB   (BF16: 9.6 GB, SFP8: 4.6 GB, Q4: 3.2 GB)")
		fmt.Fprintln(out, "       ~5 GB    gemma4-e4b         ~5 GB     (BF16: 15 GB, SFP8: 7.5 GB, Q4: 5 GB)")
		fmt.Fprintln(out, "       ~17 GB   gemma4-31b         ~17 GB    (BF16: 58.3 GB, SFP8: 30.4 GB)")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "       contenox model registry-list   # full list with sizes")
		fmt.Fprintln(out, "       contenox model pull granite-3.2-2b")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  The llama backend is already registered and set as default.")
		fmt.Fprintln(out, "  The first model you pull becomes the default-model automatically.")
		fmt.Fprintln(out, "")
		chatStep = 2
	case "openrouter":
		backendRegistered := hasBackendOfType("openrouter")
		envVal := os.Getenv(pc.envKey)
		var kvHasKey bool
		if envVal == "" {
			if dbPath, gpErr := globalDBPath(); gpErr == nil {
				if db, openErr := OpenDBAt(libtracker.WithNewRequestID(context.Background()), dbPath); openErr == nil {
					store := runtimetypes.New(db.WithoutTransaction())
					var cfg runtimestate.ProviderConfig
					kvKey := runtimestate.ProviderKeyPrefix + "openrouter"
					if err := store.GetKV(libtracker.WithNewRequestID(context.Background()), kvKey, &cfg); err == nil && cfg.APIKey != "" {
						kvHasKey = true
					}
					db.Close()
				}
			}
		}
		keyReady := envVal != "" || kvHasKey
		switch {
		case envVal != "":
			fmt.Fprintf(out, "  ✓ %s detected in environment.\n\n", pc.envKey)
		case kvHasKey:
			fmt.Fprintf(out, "  ✓ OpenRouter API key stored in local.db (set %s to use a different key).\n\n", pc.envKey)
		default:
			fmt.Fprintln(out, "  OpenRouter gives you access to 300+ models — DeepSeek, Qwen, Llama, Mistral,")
			fmt.Fprintln(out, "  Gemini, Claude, GPT and many more — through a single API key. It accepts")
			fmt.Fprintln(out, "  payment methods that are not always available on individual provider sites,")
			fmt.Fprintln(out, "  and lets you switch models without managing multiple accounts.")
			fmt.Fprintln(out, "")
			fmt.Fprintf(out, "  ⚠  %s not set.\n", pc.envKey)
			fmt.Fprintln(out, "  Get your free API key at: https://openrouter.ai/settings/keys")
			fmt.Fprintln(out, "  Then:")
			fmt.Fprintf(out, "       export %s=your-key-here\n\n", pc.envKey)
		}
		step := 1
		if !keyReady {
			fmt.Fprintf(out, "  1. Get an API key at https://openrouter.ai/settings/keys, then:\n")
			fmt.Fprintf(out, "       export %s=your-key-here\n\n", pc.envKey)
			step = 2
		}
		if !backendRegistered {
			fmt.Fprintf(out, "  %d. Register the OpenRouter backend and set defaults:\n", step)
			fmt.Fprintf(out, "       contenox backend add openrouter --type openrouter --api-key-env %s\n", pc.envKey)
			fmt.Fprintf(out, "       contenox config set default-provider openrouter\n")
			fmt.Fprintf(out, "       contenox config set default-model %s\n", pc.defaultModel)
			fmt.Fprintln(out, "       contenox doctor")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "  Some cost-effective starting models on OpenRouter:")
			fmt.Fprintln(out, "       deepseek/deepseek-chat-v3-5   (excellent quality, very low cost)")
			fmt.Fprintln(out, "       google/gemini-2.0-flash-001   (fast, cheap)")
			fmt.Fprintln(out, "       qwen/qwen3-235b-a22b           (strong reasoning)")
			fmt.Fprintln(out, "       meta-llama/llama-3.3-70b-instruct")
			fmt.Fprintln(out, "")
			fmt.Fprintln(out, "  Browse all models (with pricing): https://openrouter.ai/models")
			fmt.Fprintln(out, "")
			chatStep = step + 1
		} else {
			chatStep = step
		}

	case "ollama":
		if base, ok := setupcheck.ProbeLocalOllamaAPI(context.Background()); ok {
			fmt.Fprintf(out, "  Local Ollama is already reachable at %s. Skip steps 1-2 on this machine if install, ollama serve, and ollama pull (e.g. qwen2.5:7b) are already done.\n\n", base)
		}
		fmt.Fprintln(out, "  1. Install Ollama (if not already):")
		fmt.Fprintln(out, "       curl -fsSL https://ollama.com/install.sh | sh")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  2. Run the Ollama server (leave it running), then pull a model in another terminal:")
		fmt.Fprintln(out, "       ollama serve")
		fmt.Fprintln(out, "       ollama pull qwen2.5:7b")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  3. Register the local API and set defaults (URLs match contenox backend add defaults):")
		fmt.Fprintln(out, "       contenox backend add ollama --type ollama")
		fmt.Fprintln(out, "       contenox config set default-provider ollama")
		fmt.Fprintln(out, "       contenox config set default-model qwen2.5:7b")
		fmt.Fprintln(out, "       contenox doctor")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  Optional: use hosted Ollama Cloud instead of a local server:")
		fmt.Fprintln(out, "       export OLLAMA_API_KEY=your-key-here")
		fmt.Fprintln(out, "       contenox backend add ollama-cloud --type ollama --url https://ollama.com/api --api-key-env OLLAMA_API_KEY")
		fmt.Fprintln(out, "  Get an Ollama API key for direct cloud access: https://ollama.com/settings/keys")
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "  Optional: run fully local GGUF models via the llama provider:")
		fmt.Fprintln(out, "       contenox init llama")
		fmt.Fprintln(out, "")
		chatStep = 4
	default:
		backendRegistered := hasBackendOfType(provider)
		registerStep := 1
		if !backendReady {
			fmt.Fprintf(out, "  1. Set your %s API key:\n", pc.name)
			fmt.Fprintf(out, "       export %s=your-key-here\n", pc.envKey)
			switch provider {
			case "gemini":
				fmt.Fprintln(out, "  Get a free Gemini API key: https://aistudio.google.com/apikey")
			case "openai":
				fmt.Fprintln(out, "  Get an OpenAI API key: https://platform.openai.com/api-keys")
			}
			fmt.Fprintln(out, "")
			registerStep = 2
		}
		if !backendRegistered {
			fmt.Fprintf(out, "  %d. Register the %s backend and set defaults:\n", registerStep, pc.name)
			fmt.Fprintf(out, "       contenox backend add %s --type %s --api-key-env %s\n", provider, provider, pc.envKey)
			fmt.Fprintf(out, "       contenox config set default-provider %s\n", provider)
			fmt.Fprintf(out, "       contenox config set default-model %s\n", pc.defaultModel)
			fmt.Fprintln(out, "       contenox doctor")
			fmt.Fprintln(out, "")
			chatStep = registerStep + 1
		} else {
			chatStep = registerStep
		}
	}
	fmt.Fprintf(out, "  %d. Chat with your model:\n", chatStep)
	fmt.Fprintln(out, "       contenox hey, what can you do?")
	fmt.Fprintln(out, "       echo 'fix the typos in README.md' | contenox")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  To enable shell and filesystem tools pass --shell to any command, e.g.:")
	fmt.Fprintln(out, "       contenox --shell \"run the tests\"")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "  Run 'contenox --help' for full usage.")
	return nil
}

// init.go implements the contenox init subcommand (scaffold .contenox/).
package contenoxcli

import (
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

//go:embed chain-contenox.json
var initChain string

//go:embed chain-run.json
var initRunChain string

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
}

// RunInit scaffolds .contenox/ with default chain files.
// provider is "" (default = ollama), "ollama", "gemini", or "openai".
func RunInit(force bool, provider string) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		provider = "ollama"
	}

	pc, ok := providerConfigs[provider]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown provider %q. Valid options: ollama, gemini, openai\n", provider)
		os.Exit(1)
	}

	cwd, err := os.Getwd()
	if err != nil {
		slog.Error("Cannot get current directory", "error", err)
		os.Exit(1)
	}
	contenoxDir := filepath.Join(cwd, ".contenox")
	if err := os.MkdirAll(contenoxDir, 0750); err != nil {
		slog.Error("Failed to create .contenox directory", "error", err)
		os.Exit(1)
	}
	chainPath := filepath.Join(contenoxDir, "default-chain.json")
	runChainPath := filepath.Join(contenoxDir, "default-run-chain.json")
	writeFile := func(path, content string) bool {
		if !force {
			if _, err := os.Stat(path); err == nil {
				fmt.Printf("  %s already exists (use --force to overwrite)\n", path)
				return false
			}
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			slog.Error("Failed to write file", "path", path, "error", err)
			os.Exit(1)
		}
		fmt.Printf("  Created %s\n", path)
		return true
	}

	writeFile(chainPath, initChain)
	writeFile(runChainPath, initRunChain)

	fmt.Println("Done.")
	fmt.Println("")

	// If a cloud provider is selected, check for the API key and instruct if missing.
	if pc.envKey != "" {
		if os.Getenv(pc.envKey) == "" {
			fmt.Printf("⚠️  %s API key not found in environment.\n", pc.name)
			fmt.Printf("   Set it before running contenox:\n\n")
			fmt.Printf("     export %s=your-key-here\n\n", pc.envKey)
		} else {
			fmt.Printf("✓  %s API key detected (%s).\n\n", pc.name, pc.envKey)
		}
	}

	fmt.Println("Next steps:")
	fmt.Println("")
	if provider == "ollama" {
		fmt.Println("  1. Start Ollama and pull the default model:")
		fmt.Println("       ollama serve && ollama pull qwen2.5:7b")
		fmt.Println("")
	} else {
		fmt.Printf("  1. Register the %s backend:\n", pc.name)
		fmt.Printf("       contenox backend add %s --type %s --api-key-env %s\n", provider, provider, pc.envKey)
		fmt.Printf("       contenox config set default-model %s\n", pc.defaultModel)
		fmt.Println("")
	}
	fmt.Printf("  %d. Chat with your model:\n", map[bool]int{true: 1, false: 2}[provider != "ollama"])
	fmt.Println("       contenox hey, what can you do?")
	fmt.Println("       echo 'fix the typos in README.md' | contenox")
	fmt.Println("")
	fmt.Println("  Plan and execute a multi-step task:")
	fmt.Println("       contenox plan new \"create a TODOS.md from all TODO comments in the codebase\"")
	fmt.Println("       contenox plan next --auto")
	fmt.Println("")
	fmt.Println("  To enable shell and filesystem tools pass --shell to any command, e.g.:")
	fmt.Println("       contenox --shell --local-exec-allowed-commands git,go \"run the tests\"")
	fmt.Println("")
	fmt.Println("  Run 'contenox --help' for full usage.")
}

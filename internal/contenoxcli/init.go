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

//go:embed config.yaml
var initConfig string

//go:embed chain-contenox.json
var initChain string

// providerConfig holds the provider-specific values used to generate config.yaml.
type providerConfig struct {
	name         string
	defaultModel string
	envKey       string
	configBlock  string // the backends: + default_provider: + default_model: block
}

var providerConfigs = map[string]providerConfig{
	"ollama": {
		name:         "Ollama (local)",
		defaultModel: defaultModel,
		envKey:       "",
		configBlock: `backends:
  - name: local
    type: ollama
    base_url: http://127.0.0.1:11434
default_provider: local
default_model: qwen2.5:7b
context: 32768
`,
	},
	"gemini": {
		name:         "Google Gemini",
		defaultModel: "gemini-3.1-flash-lite-preview",
		envKey:       "GEMINI_API_KEY",
		configBlock: `backends:
  - name: gemini
    type: gemini
    api_key_from_env: GEMINI_API_KEY
default_provider: gemini
default_model: gemini-3.1-flash-lite-preview
context: 32768
`,
	},
	"openai": {
		name:         "OpenAI",
		defaultModel: "gpt-4.1-mini",
		envKey:       "OPENAI_API_KEY",
		configBlock: `backends:
  - name: openai
    type: openai
    base_url: https://api.openai.com/v1
    api_key_from_env: OPENAI_API_KEY
default_provider: openai
default_model: gpt-4.1-mini
context: 32768
`,
	},
}

// makeProviderConfig generates a minimal, focused config.yaml for the given provider.
// It injects the provider block into the embedded template, replacing the default Ollama block.
func makeProviderConfig(pc providerConfig) string {
	// Replace the existing backends/default_provider/default_model/context section
	// with the provider-specific block, keeping comments below intact.
	lines := strings.Split(initConfig, "\n")
	var before, after []string
	inBlock := false
	done := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !done && !inBlock && trimmed == "backends:" {
			inBlock = true
			continue
		}
		if inBlock {
			// Skip until we hit the first comment-only section after the block
			if strings.HasPrefix(trimmed, "#") || trimmed == "" {
				inBlock = false
				done = true
				after = append(after, line)
			}
			continue
		}
		if !done {
			before = append(before, line)
		} else {
			after = append(after, line)
		}
	}
	return strings.Join(before, "\n") + pc.configBlock + "\n" + strings.Join(after, "\n")
}

// RunInit scaffolds .contenox/ (config and default chain).
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
	configPath := filepath.Join(contenoxDir, "config.yaml")
	chainPath := filepath.Join(contenoxDir, "default-chain.json")
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

	configContent := initConfig
	if provider != "ollama" {
		configContent = makeProviderConfig(pc)
	}
	writeFile(configPath, configContent)
	writeFile(chainPath, initChain)

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
	}
	fmt.Printf("  %d. Chat with your model:\n", map[bool]int{true: 1, false: 2}[provider != "ollama"])
	fmt.Println("       contenox hey, what can you do?")
	fmt.Println("       echo 'fix the typos in README.md' | contenox")
	fmt.Println("")
	fmt.Println("  Plan and execute a multi-step task:")
	fmt.Println("       contenox plan new \"create a TODOS.md from all TODO comments in the codebase\"")
	fmt.Println("       contenox plan next --auto")
	fmt.Println("")
	fmt.Println("  To enable shell and filesystem tools (local_shell / local_fs), set")
	fmt.Println("  enable_local_shell: true in .contenox/config.yaml and configure the allow list.")
	fmt.Println("")
	fmt.Println("  Run 'contenox --help' for full usage.")
}

package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/contenox/contenox/internal/kernel/reasoning"
	"github.com/contenox/contenox/internal/services/clikv"
	"github.com/contenox/contenox/internal/services/settings"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/libtracker"
	"github.com/spf13/cobra"
)

// validConfigKeys and the descriptions 'config list' prints live in
// config_registry.go, beside the stores that settings this command cannot read.

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage named defaults; explain their scope and inheritance.",
	Long: `Store and retrieve persistent CLI defaults backed by SQLite.

Use 'contenox config list' for common defaults, '--all' for advanced defaults,
and '--describe' for full descriptions and the stores this command cannot read.

` + configKeyHelp() + `
Every policy key above names an envelope, not what is in one. The envelope
itself is a [envelopes.<name>] section in agents.toml, transpiled on every run.

` + toolGrantGrammar + `

` + askWaitGrammar + `

Validate what these keys point at with 'contenox vet'.`,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a persistent config value.",
	Long: `Set a default for future launches. Names use dotted namespaces and snake_case
fields; units are part of numeric names. Legacy names remain aliases of the same row.

Examples:
  contenox config set inference.context.window_tokens 131072
  contenox config set inference.generation.max_output_tokens 8192
  contenox config set inference.reasoning.effort high
  contenox config set inference.autocomplete.model qwen2.5-coder:7b
  contenox config set execution.permissions.policy hitl-policy-strict.json

Use --scope global to set the fallback for a workspace-scoped key. Other keys
are global. Session pickers and slash commands do not save defaults.
Use 'config get <key> --explain' to inspect inheritance and constraints.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		key, value := settings.StorageKey(args[0]), strings.TrimSpace(args[1])
		if value != "" {
			switch key {
			case "telemetry-enabled", "update-check", "opt-in-beta", "oracle-approves-tool-calls":
				v, err := strconv.ParseBool(value)
				if err != nil {
					return fmt.Errorf("%s requires true or false", settings.Canonical(key))
				}
				value = strconv.FormatBool(v)
			case "fleet-max-parallel":
				v, err := strconv.Atoi(value)
				if err != nil || v < 0 {
					return fmt.Errorf("%s requires a non-negative count; 0 disables the limit", settings.Canonical(key))
				}
				value = strconv.Itoa(v)
			}
		}
		if _, ok := validConfigKeys[key]; !ok {
			return fmt.Errorf("unknown key %q — valid keys: %s", key, validConfigKeyList())
		}
		if value != "" {
			if key == "default-max-tokens" {
				normalized, err := normalizeMaxTokensConfig(value)
				if err != nil {
					return err
				}
				value = normalized
			}
			if key == "default-token-limit" {
				normalized, err := normalizeTokenLimitConfig(value)
				if err != nil {
					return err
				}
				value = normalized
			}
			if key == "default-think" {
				normalized, err := reasoning.Normalize(value)
				if err != nil {
					return err
				}
				value = normalized
			}
			if normalized, err := normalizeLogConfig(key, value); err != nil {
				return err
			} else if normalized != "" {
				value = normalized
			}
			if normalized, err := normalizeApprovalCeiling(key, value); err != nil {
				return err
			} else if normalized != "" {
				value = normalized
			}
		}
		db, store, workspaceID, err := openConfigDBWithWorkspace(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := libtracker.WithNewRequestID(context.Background())
		requestedScope, _ := cmd.Flags().GetString("scope")
		if requestedScope != "" && requestedScope != "global" && requestedScope != "workspace" {
			return fmt.Errorf("scope must be global or workspace")
		}
		if requestedScope == "workspace" && !clikv.IsWorkspaceScoped(key) {
			return fmt.Errorf("%s is global; use a session control for a temporary override", settings.Canonical(key))
		}
		if requestedScope == "global" {
			workspaceID = ""
		}
		if err := clikv.WriteConfig(ctx, store, workspaceID, key, value); err != nil {
			return fmt.Errorf("failed to set %q: %w", key, err)
		}
		stored, scope := clikv.ReadConfig(ctx, store, workspaceID, key)
		writeScope := "global"
		if workspaceID != "" && clikv.IsWorkspaceScoped(key) {
			writeScope = "workspace"
		}
		if value == "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Reset %s override (%s).\n", settings.Canonical(key), writeScope)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "%s = %s (%s stored default).\n", settings.Canonical(key), value, writeScope)
		}
		resolved := settings.Resolve(key, stored, scope, settings.Fallback(key), nil)
		fmt.Fprintf(cmd.OutOrStdout(), "Inherited value: %q (%s).\n", resolved.Value, resolved.Source)
		fmt.Fprintln(cmd.OutOrStdout(), "Active sessions keep their choices. Start a new Contenox invocation for inference defaults; restart the surface for startup settings.")
		if settings.IsCommon(key) {
			fmt.Fprintln(cmd.OutOrStdout(), "Agent limits and model capacity still apply; inspect the running session with /settings.")
		}
		return nil
	},
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a persistent config value.",
	Long: `Read a stored default. --explain shows the inherited value, source,
meaning and constraints; --json returns the same information as structured data.
This command does not inspect a running session. Use /settings inside the session
for its current choices and effective context budget.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := settings.StorageKey(args[0])
		if _, ok := validConfigKeys[key]; !ok {
			return fmt.Errorf("unknown key %q", key)
		}
		db, store, workspaceID, err := openConfigDBWithWorkspace(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := libtracker.WithNewRequestID(context.Background())
		val, scope := clikv.ReadConfig(ctx, store, workspaceID, key)
		spec := validConfigKeys[key]
		value := settings.Resolve(key, val, scope, settings.Fallback(key), nil)
		asJSON, _ := cmd.Flags().GetBool("json")
		if asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
				settings.Value
				Description configKeySpec `json:"description"`
			}{value, spec})
		}
		explain, _ := cmd.Flags().GetBool("explain")
		if !explain {
			fmt.Fprintf(cmd.OutOrStdout(), "%s  (%s stored default)\n", val, scope)
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n  Stored: %q (%s)\n  Inherited: %q (%s)\n  Meaning: %s\n  Effect: %s\n  Unset: %s\n  Applies: future launches; active sessions use /settings\n  Legacy alias: %s\n", spec.Key, val, scope, value.Value, value.Source, spec.Meaning, spec.Changes, spec.Unset, key)
		if spec.Env != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "  Environment: %s (invocation only)\n", spec.Env)
		}
		if key == "default-token-limit" || key == "default-max-tokens" {
			fmt.Fprintln(cmd.OutOrStdout(), "  Constraints: agent chain and selected model. Inspect /settings for the running chain and effective budget. Reset an inherited default with config reset; existing agents.toml overrides are preserved.")
		}
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List common defaults and their inherited values.",
	Long: `List common defaults, their stored values and inherited values before session,
agent and model constraints. --all includes advanced settings; --describe adds
complete descriptions and the locations of settings held outside this store.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		db, store, workspaceID, err := openConfigDBWithWorkspace(cmd)
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := libtracker.WithNewRequestID(context.Background())
		rows := make([]configListRow, 0, len(validConfigKeys))
		all, _ := cmd.Flags().GetBool("all")
		describe, _ := cmd.Flags().GetBool("describe")
		for _, key := range validConfigKeyNames() {
			if !all && !describe && !settings.IsCommon(key) {
				continue
			}
			val, scope := clikv.ReadConfig(ctx, store, workspaceID, key)
			rows = append(rows, configListRow{
				Key:   key,
				Value: val,
				Scope: scope,
				Env:   envOverrideFor(validConfigKeys[key]),
			})
		}
		if describe {
			return renderConfigList(cmd.OutOrStdout(), rows)
		}
		return renderConfigSummary(cmd.OutOrStdout(), rows)
	},
}

func normalizeMaxTokensConfig(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "auto" {
		return "0", nil
	}
	if value == "" {
		return "", nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return "", fmt.Errorf("default-max-tokens must be a non-negative integer, got %q", value)
	}
	if n < 0 {
		return "", fmt.Errorf("default-max-tokens must be non-negative, got %d", n)
	}
	return strconv.Itoa(n), nil
}

func normalizeTokenLimitConfig(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "auto" {
		return "0", nil
	}
	if value == "" {
		return "", nil
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return "", fmt.Errorf("default-token-limit must be a non-negative integer, got %q", value)
	}
	if n < 0 {
		return "", fmt.Errorf("default-token-limit must be non-negative, got %d", n)
	}
	return strconv.Itoa(n), nil
}

func validConfigKeyList() string {
	keys := validConfigKeyNames()
	for i, key := range keys {
		keys[i] = settings.Canonical(key)
	}
	return strings.Join(keys, ", ")
}

func validConfigKeyNames() []string {
	keys := make([]string, 0, len(validConfigKeys))
	for key := range validConfigKeys {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int { return strings.Compare(settings.Canonical(a), settings.Canonical(b)) })
	return keys
}

func getConfigKV(ctx context.Context, store runtimetypes.Store, key string) (string, error) {
	stored := ""
	if store != nil {
		stored = clikv.Read(ctx, store, key)
	}
	return settings.Resolve(key, stored, "global", settings.Fallback(key), nil).Value, nil
}

func openConfigDB(cmd *cobra.Command) (libdb.DBManager, runtimetypes.Store, error) {
	dbPath, err := resolveDBPath(cmd)
	if err != nil {
		return nil, nil, err
	}
	ctx := libtracker.WithNewRequestID(context.Background())
	db, err := OpenDBAt(ctx, dbPath)
	if err != nil {
		return nil, nil, err
	}
	return db, runtimetypes.New(db.WithoutTransaction()), nil
}

func openConfigDBWithWorkspace(cmd *cobra.Command) (libdb.DBManager, runtimetypes.Store, string, error) {
	db, store, err := openConfigDB(cmd)
	if err != nil {
		return nil, nil, "", err
	}
	contenoxDir, _ := ResolveContenoxDir(cmd)
	workspaceID := ResolveWorkspaceID(contenoxDir)
	return db, store, workspaceID, nil
}

var configResetCmd = &cobra.Command{
	Use: "reset <key>", Short: "Remove a stored override and inherit its fallback.", Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error { return configSetCmd.RunE(cmd, []string{args[0], ""}) },
}

func completeConfigKeys(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) != 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := []string{}
	for _, spec := range settings.All() {
		out = append(out, spec.Key+"\t"+spec.Meaning)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

func init() {
	configSetCmd.Flags().String("scope", "", "Storage scope: global or workspace (where supported)")
	configResetCmd.Flags().String("scope", "", "Storage scope to reset: global or workspace (where supported)")
	configGetCmd.Flags().Bool("explain", false, "Explain the inherited default, provenance and constraints")
	configGetCmd.Flags().Bool("json", false, "Emit the default, provenance and description as JSON")
	configListCmd.Flags().Bool("all", false, "Include advanced saved defaults")
	configListCmd.Flags().Bool("describe", false, "Include full descriptions and advanced configuration locations")
	configSetCmd.ValidArgsFunction = completeConfigKeys
	configGetCmd.ValidArgsFunction = completeConfigKeys
	configResetCmd.ValidArgsFunction = completeConfigKeys
	configCmd.AddCommand(configResetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configListCmd)
}

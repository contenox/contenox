package contenoxcli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/contenox/contenox/internal/services/settings"
)

type configKeySpec = settings.Spec

var validConfigKeys = func() map[string]configKeySpec {
	out := make(map[string]configKeySpec)
	for _, spec := range settings.All() {
		out[settings.StorageKey(spec.Key)] = spec
	}
	return out
}()

// configKeyHelp renders the supported-key block of the command help from the
// registry, so help and 'config list' cannot disagree about which keys exist.
func configKeyHelp() string {
	var sb strings.Builder
	sb.WriteString("Supported keys:\n")
	for _, key := range validConfigKeyNames() {
		fmt.Fprintf(&sb, "  %-30s %s\n", settings.Canonical(key), validConfigKeys[key].Meaning)
	}
	return sb.String()
}

// configListRow is one rendered line of the table: what is stored, where it is
// scoped, and whether an environment variable is overriding it right now.
type configListRow struct {
	Key   string
	Value string
	Scope string
	Env   string
}

// configListElsewhere names the settings that exist but cannot appear in this
// table, because they live in another store. A table of one store that reads as
// a table of all of them is how a context budget of 128000 became invisible
// while every listed value was accounted for.
const configListElsewhere = `
Settings that are not in this table, and where they live:

  agents.toml, per agent and per tree — shipped defaults are compiled into this
  binary; ~/.contenox/agents.toml and ./.contenox/agents.toml override them, an
  [agents.<name>] section overrides those, and the declaration itself wins last:
      [chain]           token_limit, max_tokens, think, main_rounds,
                        recovery_rounds, shift, retry_on_failure, retry_policy
      [routing]         model, provider, alt_model, alt_provider, router_model,
                        router_provider, pin_model
      [tools_policies]  per-toolset allowlists and limits
      [policy]          postures, always_deny, compute and attention caps
      [envelopes]       named HITL envelopes and the rules inside them
      [naming] [tools] [models]

  session options — set per session by the client, held in memory, never
  persisted. Agent limits and model capacity still constrain them:
      token-limit, think, model, provider, policy

  per model — context window and output cap, listed by 'contenox model list'.
  A backend that reports neither leaves them unknown.

  derived — the context budget is min(chain token_limit, session token-limit);
  the effective output cap is then reserved as output headroom. Stored defaults
  are inputs to this calculation; this table is not a running session snapshot.
`

// envOverrideFor reports the environment override that is in force for a key, so
// a stored value that is being ignored cannot look like a value in use.
func envOverrideFor(spec configKeySpec) string {
	if spec.Env == "" {
		return ""
	}
	value := strings.TrimSpace(os.Getenv(spec.Env))
	if value == "" {
		return ""
	}
	return spec.Env + "=" + value
}

// renderConfigList prints the stored table, then every key in full: what it
// means, what changing it changes, who reads it, and what is in force when it is
// unset — followed by the stores this command cannot read.
func renderConfigList(w io.Writer, rows []configListRow) error {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "KEY\tVALUE\tSCOPE\tENV OVERRIDE")
	for _, row := range rows {
		value, scope, env := row.Value, row.Scope, row.Env
		if value == "" {
			value = "—"
		}
		if scope == "" {
			scope = "—"
		}
		if env == "" {
			env = "—"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", settings.Canonical(row.Key), value, scope, env)
	}
	if err := table.Flush(); err != nil {
		return err
	}

	fmt.Fprint(w, "\nWhat each key means, what changing it changes, and who reads it:\n\n")
	for _, row := range rows {
		spec, ok := validConfigKeys[row.Key]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "%s (alias: %s)\n", settings.Canonical(row.Key), row.Key)
		fmt.Fprintf(w, "    is       %s\n", spec.Meaning)
		fmt.Fprintf(w, "    changes  %s\n", spec.Changes)
		fmt.Fprintf(w, "    read by  %s\n", spec.ReadBy)
		if spec.Env != "" {
			fmt.Fprintf(w, "    env      %s overrides the stored value for one invocation and is not persisted\n", spec.Env)
		}
		if spec.Unset != "" {
			fmt.Fprintf(w, "    unset    %s\n", spec.Unset)
		}
		fmt.Fprintln(w)
	}

	fmt.Fprint(w, configListElsewhere)
	return nil
}

func renderConfigSummary(w io.Writer, rows []configListRow) error {
	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(table, "SETTING\tSTORED\tINHERITED\tSOURCE")
	for _, row := range rows {
		value := settings.Resolve(row.Key, row.Value, row.Scope, settings.Fallback(row.Key), nil)
		stored, inherited := row.Value, value.Value
		if stored == "" {
			stored = "—"
		}
		if inherited == "" {
			inherited = "surface default"
		}
		if row.Key == "default-model" || row.Key == "default-provider" {
			if value.Value == "" {
				inherited = "not configured"
			}
		}
		if value.Value == "0" && row.Key == "default-token-limit" {
			inherited = "auto (model capacity)"
		}
		if value.Value == "0" && row.Key == "default-max-tokens" {
			inherited = "backend default"
		}
		fmt.Fprintf(table, "%s\t%s\t%s\t%s\n", value.Key, stored, inherited, value.Source)
	}
	if err := table.Flush(); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "\nDefaults for new launches. Agent and model limits may narrow them.\nUse config get <setting> --explain; use /settings in a session for effective values.\nUse config list --all for advanced defaults; --describe explains every setting and its owner.")
	return err
}

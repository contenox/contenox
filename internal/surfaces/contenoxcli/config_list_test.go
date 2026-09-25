package contenoxcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/contenox/contenox/internal/services/settings"
)

// TestConfigRegistryDescribesEveryKey is the guard that keeps 'config list'
// honest as keys are added: a key that cannot say what it means, what changing
// it changes, and who reads it is a knob nobody can reason about, which is how
// the surface grew one nobody could explain.
func TestConfigRegistryDescribesEveryKey(t *testing.T) {
	if len(validConfigKeys) == 0 {
		t.Fatal("the config registry is empty")
	}
	for key, spec := range validConfigKeys {
		if strings.TrimSpace(spec.Meaning) == "" {
			t.Errorf("%s: no meaning", key)
		}
		if strings.TrimSpace(spec.Changes) == "" {
			t.Errorf("%s: does not say what changing it changes", key)
		}
		if strings.TrimSpace(spec.ReadBy) == "" {
			t.Errorf("%s: does not name a reader, so nothing tells an operator whether it is used", key)
		}
		if spec.Env != "" && (spec.Env != strings.ToUpper(spec.Env) || strings.Contains(spec.Env, " ")) {
			t.Errorf("%s: env override %q is not an environment variable name", key, spec.Env)
		}
	}
}

// TestConfigListRendersEveryKeyInFull pins that the listing carries the meaning,
// the blast radius and the reader of every key, not just its stored value.
func TestConfigListRendersEveryKeyInFull(t *testing.T) {
	rows := make([]configListRow, 0, len(validConfigKeys))
	for _, key := range validConfigKeyNames() {
		rows = append(rows, configListRow{Key: key, Value: "value-of-" + key, Scope: "global"})
	}

	var buf bytes.Buffer
	if err := renderConfigList(&buf, rows); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	for _, key := range validConfigKeyNames() {
		stanza := "\n" + settings.Canonical(key) + " (alias: " + key + ")\n"
		if !strings.Contains(out, stanza) {
			t.Errorf("%s: no stanza in the listing", key)
			continue
		}
		spec := validConfigKeys[key]
		for _, want := range []string{spec.Meaning, spec.Changes, spec.ReadBy} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: listing omits %q", key, want)
			}
		}
	}
}

// TestConfigListNamesTheStoresItCannotRead pins the completeness claim: a table
// of one store must say which settings live elsewhere, or a value that is in
// force but absent from the table reads as one that does not exist.
func TestConfigListNamesTheStoresItCannotRead(t *testing.T) {
	rows := []configListRow{{Key: "default-model", Value: "m", Scope: "global"}}

	var buf bytes.Buffer
	if err := renderConfigList(&buf, rows); err != nil {
		t.Fatalf("render: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"agents.toml",
		"[chain]",
		"[envelopes]",
		"token_limit",
		"session options",
		"token-limit",
		"contenox model list",
		"derived",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing must name where else settings live: missing %q", want)
		}
	}
}

// TestConfigListShowsTheOverrideInForce pins that a stored value being ignored
// cannot look like a value in use: an environment override is shown beside it.
func TestConfigListShowsTheOverrideInForce(t *testing.T) {
	if got := envOverrideFor(validConfigKeys["log-max-files"]); got != "" {
		t.Errorf("a key with no env override reported %q", got)
	}

	t.Setenv(envDefaultModel, "env-model")
	got := envOverrideFor(validConfigKeys["default-model"])
	if !strings.Contains(got, envDefaultModel) || !strings.Contains(got, "env-model") {
		t.Errorf("an override in force must be shown, got %q", got)
	}
}

// TestConfigCommandHelpAgreesWithTheRegistry pins that the command help is
// generated from the same registry the listing uses, so the two cannot drift.
func TestConfigCommandHelpAgreesWithTheRegistry(t *testing.T) {
	for _, key := range validConfigKeyNames() {
		if !strings.Contains(configCmd.Long, settings.Canonical(key)) {
			t.Errorf("help does not name %s", key)
		}
	}
}

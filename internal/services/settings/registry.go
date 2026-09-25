// Package settings describes persistent user settings and resolves their inherited defaults.
package settings

import (
	"os"
	"sort"
	"strings"
)

// Spec describes a setting independently of its storage and presentation.
type Spec struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	Meaning string `json:"meaning"`
	Changes string `json:"effect"`
	ReadBy  string `json:"read_by"`
	Env     string `json:"environment_variable"`
	Unset   string `json:"unset_behavior"`
}

// ContextWindowTokens is the session history window setting, measured in tokens.
const ContextWindowTokens = "inference.context.window_tokens"

// MaxOutputTokens is the per-request generation limit, measured in tokens.
const MaxOutputTokens = "inference.generation.max_output_tokens"

// Model is the selected inference model.
const Model = "inference.model"

// ReasoningEffort is the requested reasoning effort.
const ReasoningEffort = "inference.reasoning.effort"

// PermissionPolicy is the tool approval policy selection.
const PermissionPolicy = "execution.permissions.policy"

// DefaultOutputTokens is the inherited generation cap.
const DefaultOutputTokens = "8284"

// FallbackContextTokens bounds history when the backend reports no model context limit.
const FallbackContextTokens = 131072

var specs = map[string]Spec{
	"default-model": {
		Key:     "inference.model",
		Label:   "Inference model",
		Meaning: "Default LLM model for a new session.",
		Changes: "Which model a turn runs on, unless the session names another, an agent declaration pins its own, or routing resolves a different backend for the same model.",
		ReadBy:  "The ACP surface's session default, and chain routing through the model template ({{var:model}}). 'contenox model list' marks it with *.",
		Env:     "CONTENOX_DEFAULT_MODEL",
		Unset:   "No default model: a session has to be given one.",
	},
	"default-provider": {
		Key:     "inference.provider",
		Label:   "Model provider",
		Meaning: "Default LLM provider type for a new session (ollama, openai, vertex-google, …).",
		Changes: "Which backend serves inference.model, and therefore which credentials, URL and price apply.",
		ReadBy:  "The ACP surface's session default, and chain routing through the provider template ({{var:provider}}).",
		Env:     "CONTENOX_DEFAULT_PROVIDER",
		Unset:   "No default provider: a session has to be given one.",
	},
	"default-alt-model": {
		Key:     "inference.recovery.model",
		Label:   "inference / recovery / model",
		Meaning: "Optional second model for a chain's recovery and terminal stages.",
		Changes: "Which model answers when the main stage exhausts its rounds or fails, where the chain's routing names the alt template.",
		ReadBy:  "Chain routing: the shipped template is alt_model = {{var:alt_model|var:default_model}}.",
		Env:     "CONTENOX_DEFAULT_ALT_MODEL",
		Unset:   "Falls back to inference.model.",
	},
	"default-alt-provider": {
		Key:     "inference.recovery.provider",
		Label:   "inference / recovery / provider",
		Meaning: "Optional provider type for the alt model, independent of inference.provider.",
		Changes: "Which backend serves the recovery and terminal stages.",
		ReadBy:  "Chain routing, through alt_provider = {{var:alt_provider|var:default_provider}}.",
		Env:     "CONTENOX_DEFAULT_ALT_PROVIDER",
		Unset:   "Falls back to inference.provider.",
	},
	"default-autocomplete-model": {
		Key:     "inference.autocomplete.model",
		Label:   "inference / autocomplete / model",
		Meaning: "Optional model for editor autocomplete, separate from chat.",
		Changes: "Which model fills inline completions; it does not affect a conversation turn.",
		ReadBy:  "The autocomplete route only, which is a chain of its own.",
		Unset:   "Autocomplete falls back to the session's model.",
	},
	"default-autocomplete-provider": {
		Key:     "inference.autocomplete.provider",
		Label:   "inference / autocomplete / provider",
		Meaning: "Optional provider type for the autocomplete model.",
		Changes: "Which backend serves inline completions.",
		ReadBy:  "The autocomplete route only.",
		Unset:   "Falls back to inference.provider.",
	},
	"default-audio-model": {
		Key:     "inference.audio.model",
		Label:   "inference / audio / model",
		Meaning: "Optional model preferred for requests carrying audio attachments.",
		Changes: "Which model transcribes or answers audio; a model that reports no audio capability is never resolved for such a request.",
		ReadBy:  "The ACP surface when a prompt carries audio.",
		Unset:   "Falls back to inference.model, which must still report audio capability.",
	},
	"default-audio-provider": {
		Key:     "inference.audio.provider",
		Label:   "inference / audio / provider",
		Meaning: "Optional provider type for the audio model, independent of inference.provider.",
		Changes: "Which backend serves audio-carrying requests.",
		ReadBy:  "The ACP surface when a prompt carries audio.",
		Unset:   "Falls back to inference.provider.",
	},
	"default-max-tokens": {
		Key:     "inference.generation.max_output_tokens",
		Label:   "Maximum output tokens",
		Meaning: "Maximum output tokens per model call. A positive integer sets a cap; auto or 0 uses the backend default. Reset inherits the built-in cap.",
		Changes: "How long an answer may be, and how much headroom the engine holds back from the context window before sliding history. One value does both: it is the request's output cap and the reserve subtracted when the slide budget is computed, so a value at or above the window leaves nothing to slide.",
		ReadBy:  "The chain [chain] max_tokens template, the task engine's request cap, its overflow reserve, and the provider-ceiling clamp.",
		Env:     "CONTENOX_DEFAULT_MAX_TOKENS",
		Unset:   DefaultOutputTokens + " output tokens. An explicit custom chain max_tokens still wins over an inherited setting.",
	},
	"default-token-limit": {
		Key:     "inference.context.window_tokens",
		Label:   "Context window tokens",
		Meaning: "Default context window a session inherits, in tokens (0 or unset = automatic, the model's reported window).",
		Changes: "How much conversation history a session keeps before sliding, and the usage gauge's denominator, when the session sets no token-limit of its own.",
		ReadBy:  "ACP, Beam and scripted runs when no invocation or session context override is set.",
		Unset:   "Automatic: the model's reported context window.",
	},
	"default-think": {
		Key:     "inference.reasoning.effort",
		Label:   "Reasoning effort",
		Meaning: "Default reasoning level: auto, off, minimal, low, medium, high, xhigh.",
		Changes: "How much reasoning the model is asked to do, where the model supports it, and what that reasoning costs in output tokens.",
		ReadBy:  "The session's think setting and the chain think template ({{var:think}}).",
		Env:     "CONTENOX_DEFAULT_THINK",
		Unset:   "high. The value auto delegates reasoning effort to the model backend.",
	},
	"default-chain": {
		Key:     "execution.chain",
		Label:   "execution / chain",
		Meaning: "Default chain file for this workspace, relative to .contenox/ or absolute.",
		Changes: "Which chain a surface runs for a prompt: the agent loop it selects, its stages and its budgets.",
		ReadBy:  "contenox beam, acp, chat and run when no chain is named for the invocation.",
		Unset:   "The surface's own default chain.",
	},
	"hitl-policy-name": {
		Key:     "execution.permissions.policy",
		Label:   "Tool permission policy",
		Meaning: "Active HITL policy file for this workspace, e.g. hitl-policy-strict.json.",
		Changes: "Which envelope every approval ask is evaluated under: what is allowed outright, what asks, and what is refused.",
		ReadBy:  "The ACP surface, which resolves it per session before any gated call runs.",
		Unset:   "hitl-policy-default.json.",
	},
	"approval-ceiling": {
		Key:     "execution.approval.timeout",
		Label:   "execution / approval / timeout",
		Meaning: "How long an ask waits when its grant names no timeout (30m, 24h, never).",
		Changes: "When an unanswered ask stops waiting and resolves by its on_timeout rule.",
		ReadBy:  "The HITL service, on every ask that has no deadline of its own.",
		Unset:   "168h, seven days.",
	},
	"telemetry-enabled": {
		Key:     "observability.telemetry.enabled",
		Label:   "observability / telemetry / enabled",
		Meaning: "Write local telemetry to <data-dir>/telemetry.log (true/false).",
		Changes: "Whether telemetry is recorded at all. Nothing is sent anywhere either way; the file is local.",
		ReadBy:  "The telemetry writer at startup.",
		Unset:   "Disabled.",
	},
	"update-check": {
		Key:     "updates.check.enabled",
		Label:   "updates / check / enabled",
		Meaning: "Check for a newer release (true/false).",
		Changes: "Whether this machine contacts the update endpoint. Set false for air-gapped or zero-trust hosts.",
		ReadBy:  "The update check at startup.",
		Unset:   "Enabled.",
	},
	"opt-in-beta": {
		Key:     "features.beta.enabled",
		Label:   "features / beta / enabled",
		Meaning: "Enable beta features: agent roster, event triggers (true/false).",
		Changes: "Whether unfinished surfaces are reachable at all.",
		ReadBy:  "The beta gate, which every beta surface consults before registering.",
		Env:     "CONTENOX_OPT_IN_BETA",
		Unset:   "Disabled.",
	},
	"default-mission-agent": {
		Key:     "execution.missions.default_agent",
		Label:   "execution / missions / default agent",
		Meaning: "Default declared agent run as a subagent when none is named.",
		Changes: "Which agent /plan, /mission <intent> and mission fire dispatch when no --agent is given.",
		ReadBy:  "The mission dispatcher.",
		Unset:   "No default: a subagent has to be named.",
	},
	"default-mission-policy": {
		Key:     "execution.missions.permissions.policy",
		Label:   "execution / missions / permissions / policy",
		Meaning: "Default envelope a subagent runs under when none is named.",
		Changes: "What a dispatched unit may do without asking, and what it must ask about.",
		ReadBy:  "The mission dispatcher, when it starts a unit.",
		Unset:   "The dispatcher's own default envelope.",
	},
	"default-oracle-chain": {
		Key:     "execution.oracle.chain",
		Label:   "execution / oracle / chain",
		Meaning: "Chain that adjudicates a subagent's asks, e.g. chain-oracle-default.json.",
		Changes: "Whether a subagent's asks are ruleable by a chain instead of always waiting for a human.",
		ReadBy:  "The ask path, when a unit's envelope routes attention to the oracle.",
		Unset:   "No oracle: every ask waits for a human.",
	},
	"default-oracle-policy": {
		Key:     "execution.oracle.permissions.policy",
		Label:   "execution / oracle / permissions / policy",
		Meaning: "Envelope the oracle chain itself runs under.",
		Changes: "What the adjudicating chain may do while deciding.",
		ReadBy:  "The ask path, when it runs the oracle chain.",
		Unset:   "hitl-policy-oracle.json.",
	},
	"oracle-approves-tool-calls": {
		Key:     "execution.oracle.allow_tool_approvals",
		Label:   "execution / oracle / allow tool approvals",
		Meaning: "Let the oracle rule on a subagent's approve-tier tool calls, not only its questions (true/false).",
		Changes: "Whether gated tool calls in a unit can be granted without a human. The unit's envelope attention.allowAgentApprovals must permit it as well.",
		ReadBy:  "The ask path, before it offers a tool-call ask to the oracle.",
		Unset:   "Disabled: tool calls wait for a human.",
	},
	"fleet-max-parallel": {
		Key:     "execution.missions.max_parallel",
		Label:   "execution / missions / max parallel",
		Meaning: "Fleet admission cap: how many mission units may be open at once (integer; 0 = unlimited).",
		Changes: "How much work the fleet runs concurrently, and therefore how much it can spend at once.",
		ReadBy:  "The fleet dispatcher on every unit admission.",
		Unset:   "8.",
	},
	"log-max-size": {
		Key:     "observability.logs.max_file_size",
		Label:   "observability / logs / max file size",
		Meaning: "Size at which 'contenox beam' starts a new part of the surface log (e.g. 10MB, 512KB).",
		Changes: "How large a single surface log file grows before it rotates.",
		ReadBy:  "The surface log rotator in contenox beam.",
		Unset:   "The rotator's built-in size.",
	},
	"log-max-files": {
		Key:     "observability.logs.max_files",
		Label:   "observability / logs / max files",
		Meaning: "How many surface log files to keep, counted across every date and part (integer; 0 = unlimited).",
		Changes: "How far back surface logs survive after rotation.",
		ReadBy:  "The surface log rotator in contenox beam.",
		Unset:   "The rotator's built-in count.",
	},
	"log-max-age-days": {
		Key:     "observability.logs.max_age_days",
		Label:   "observability / logs / max age days",
		Meaning: "Delete surface logs whose date is older than this many days (integer; 0 = no age limit).",
		Changes: "How long surface logs survive by age, independent of the file count.",
		ReadBy:  "The surface log rotator in contenox beam.",
		Unset:   "The rotator’s built-in age limit; 0 explicitly disables age-based retention.",
	},
}

// Lookup accepts a canonical name or a legacy storage name.
func Lookup(key string) (Spec, bool) {
	spec, ok := specs[StorageKey(key)]
	return spec, ok
}

// StorageKey maps aliases to one existing database row.
func StorageKey(key string) string {
	for legacy, spec := range specs {
		if key == spec.Key {
			return legacy
		}
	}
	return key
}

// Canonical returns the public name of a setting, preserving unknown names.
func Canonical(key string) string {
	if spec, ok := Lookup(key); ok {
		return spec.Key
	}
	return key
}

// All returns the setting definitions sorted by public name.
func All() []Spec {
	out := make([]Spec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Environment returns a supported environment override and its variable name.
func Environment(key string) (string, string) {
	spec, ok := Lookup(key)
	if !ok || spec.Env == "" {
		return "", ""
	}
	if value := strings.TrimSpace(os.Getenv(spec.Env)); value != "" {
		return value, spec.Env
	}
	return "", ""
}

// Value records a resolved default and its provenance, before agent and model limits.
type Value struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Stored string `json:"stored,omitempty"`
	Scope  string `json:"scope"`
}

// Resolve applies an explicit invocation override, environment, stored value, then fallback.
func Resolve(key, stored, scope, fallback string, override *string) Value {
	out := Value{Key: Canonical(key), Value: fallback, Source: "built-in", Stored: stored, Scope: scope}
	if stored != "" {
		out.Value, out.Source = stored, scope
	}
	if value, env := Environment(key); env != "" {
		out.Value, out.Source = value, env
	}
	if override != nil {
		out.Value, out.Source = *override, "invocation"
	}
	return out
}

// Fallback returns the concrete inherited default where the setting has one.
func Fallback(key string) string {
	switch StorageKey(key) {
	case "default-max-tokens":
		return DefaultOutputTokens
	case "default-think":
		return "high"
	case "default-token-limit":
		return "0"
	case "hitl-policy-name":
		return "hitl-policy-default.json"
	case "approval-ceiling":
		return "168h"
	case "telemetry-enabled", "opt-in-beta", "oracle-approves-tool-calls":
		return "false"
	case "update-check":
		return "true"
	case "fleet-max-parallel":
		return "8"
	}
	return ""
}

// ContextBudget bounds a requested window by positive chain and model limits.
func ContextBudget(requested, chain, model int) int {
	result := requested
	for _, limit := range []int{chain, model} {
		if limit > 0 && (result <= 0 || limit < result) {
			result = limit
		}
	}
	if result <= 0 {
		return FallbackContextTokens
	}
	return result
}

// IsCommon identifies the settings shown in the default list.
func IsCommon(key string) bool {
	switch StorageKey(key) {
	case "default-model", "default-provider", "default-max-tokens", "default-token-limit", "default-think", "hitl-policy-name":
		return true
	}
	return false
}

// Name returns the human-readable label for a setting.
func Name(key string) string {
	if spec, ok := Lookup(key); ok {
		return spec.Label
	}
	return key
}

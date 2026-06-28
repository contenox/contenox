package openvino

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/transport"
)

const backendName = "openvino"

type (
	ManifestSegment = contextasm.ManifestSegment
	ContextManifest = contextasm.ContextManifest
)

func hashString(s string) string { return contextasm.HashString(s) }

// normalizeConfig fills tested defaults. OpenVINO device/precision are resolved
// inside modeld (CONTENOX_OPENVINO_DEVICE); the runtime only needs the context
// window. The prompt format/template digest identify the model's OWN chat
// template (set from the model files in the provider) — never a hardcoded format,
// so the cache key is honest across different models. They are left as supplied.
func normalizeConfig(cfg Config) Config {
	if cfg.NumCtx <= 0 {
		cfg.NumCtx = 8192
	}
	if cfg.PromptFormat == "" {
		cfg.PromptFormat = "openvino-chat-template"
	}
	return cfg
}

func runtimeDigest(cfg Config, adapters []transport.AdapterSpec) string {
	cfg = normalizeConfig(cfg)
	type adapterIdentity struct {
		Digest string  `json:"digest,omitempty"`
		Scale  float32 `json:"scale,omitempty"`
	}
	var ids []adapterIdentity
	for _, adapter := range adapters {
		ids = append(ids, adapterIdentity{Digest: adapter.Digest, Scale: adapter.Scale})
	}
	b, _ := json.Marshal(struct {
		NumCtx                  int               `json:"num_ctx"`
		PlannerEffectiveContext int               `json:"planner_effective_context,omitempty"`
		Format                  string            `json:"prompt_format"`
		Adapters                []adapterIdentity `json:"adapters,omitempty"`
	}{cfg.NumCtx, cfg.PlannerEffectiveContext, cfg.PromptFormat, ids})
	return hashString(string(b))
}

type promptIdentity struct {
	ProfileID      string
	ModelDigest    string
	BackendVersion string
	Adapters       []transport.AdapterSpec
}

func appendAdapterIdentity(b *strings.Builder, adapters []transport.AdapterSpec) {
	for i, adapter := range adapters {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(b, "%s@%s", adapter.Digest, strconv.FormatFloat(float64(adapter.Scale), 'g', -1, 32))
	}
}

type promptPlan struct {
	Stable   PrefixInput
	Volatile SuffixInput
}

// buildPromptPlan renders the messages into a stable prefix (leading system
// turns, kept warm) and a volatile suffix (everything from the first non-system
// turn onward), with a manifest keyed on the openvino runtime identity. modeld's
// GenAI adapter reuses the stable prefix's KV via its internal prefix cache.
func buildPromptPlan(messages []modelrepo.Message, cfg Config, id promptIdentity, toolsJSON string) (promptPlan, error) {
	cfg = normalizeConfig(cfg)

	var stable, volatile strings.Builder
	var segments []ManifestSegment
	seenVolatile := false

	for _, m := range messages {
		if err := validateMessage(m); err != nil {
			return promptPlan{}, err
		}
		// Raw content keyed by role: modeld applies the model's own chat template.
		isStable := m.Role == "system" && !seenVolatile
		if !isStable {
			seenVolatile = true
		}
		var tcJSON string
		if len(m.ToolCalls) > 0 {
			b, err := json.Marshal(m.ToolCalls)
			if err != nil {
				return promptPlan{}, fmt.Errorf("openvino: marshal tool calls: %w", err)
			}
			tcJSON = string(b)
		}

		var start, end int
		text := m.Content
		if isStable {
			start = stable.Len()
			stable.WriteString(text)
			end = stable.Len()
		} else {
			start = stable.Len() + volatile.Len()
			volatile.WriteString(text)
			end = stable.Len() + volatile.Len()
		}
		segments = append(segments, ManifestSegment{
			Kind:          segmentKindForRole(m.Role),
			Stable:        isStable,
			ByteStart:     start,
			ByteEnd:       end,
			ToolCallsJSON: tcJSON,
			ToolCallID:    m.ToolCallID,
		})
	}

	stableText := stable.String()
	volatileText := volatile.String()
	manifest, err := contextasm.BuildSplitManifest(stableText, volatileText, segments, contextasm.ManifestIdentity{
		ProfileID:            id.ProfileID,
		Backend:              backendName,
		BackendVersion:       id.BackendVersion,
		ModelDigest:          id.ModelDigest,
		PromptFormat:         cfg.PromptFormat,
		PromptTemplateDigest: cfg.PromptTemplateDigest,
		RuntimeDigest:        runtimeDigest(cfg, id.Adapters),
		AddBOS:               false,
	})
	if err != nil {
		return promptPlan{}, err
	}
	return promptPlan{
		Stable:   PrefixInput{Text: stableText, Manifest: manifest, Tools: toolsJSON},
		Volatile: SuffixInput{Text: volatileText, Manifest: manifest},
	}, nil
}

func validateMessage(m modelrepo.Message) error {
	switch m.Role {
	case "system", "user", "assistant", "tool":
		return nil
	default:
		if strings.TrimSpace(m.Role) == "" {
			return NewUnsupportedFeatureError("empty message role")
		}
		return NewUnsupportedFeatureError("message role " + m.Role)
	}
}

func segmentKindForRole(role string) string {
	switch role {
	case "system":
		return "system"
	case "assistant":
		return "assistant"
	case "tool":
		return "tool"
	default:
		return "user"
	}
}

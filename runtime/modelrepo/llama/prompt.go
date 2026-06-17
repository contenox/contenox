package llama

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
)

const (
	promptFormatChatML = "chatml"
	promptFormatLlama3 = "llama3"

	segmentSystem          = "system"
	segmentUser            = "user"
	segmentAssistant       = "assistant"
	segmentAssistantPrompt = "assistant_prompt"
	segmentBOS             = "bos"
)

type promptIdentity struct {
	ProfileID      string
	ModelDigest    string
	BackendVersion string
}

type promptPlan struct {
	Stable   PrefixInput
	Volatile SuffixInput
}

type promptRenderer struct {
	format             string
	templateDigest     string
	renderMessage      func(modelrepo.Message) (string, error)
	renderAssistantCue func() string
}

func buildPromptPlan(messages []modelrepo.Message, cfg Config, id promptIdentity, toolsJSON string) (promptPlan, error) {
	cfg = normalizeConfig(cfg)
	renderer, err := rendererForFormat(cfg.PromptFormat, cfg.PromptTemplateDigest)
	if err != nil {
		return promptPlan{}, err
	}

	var stable, volatile strings.Builder
	var segments []ManifestSegment
	seenVolatile := false

	for _, m := range messages {
		if err := validateMessage(m); err != nil {
			return promptPlan{}, err
		}
		// Raw content keyed by role: modeld applies the model's OWN chat template
		// (read from the GGUF). The runtime must not render a hardcoded format. BOS
		// and the assistant cue are added by the model template + tokenizer.
		isStable := m.Role == "system" && !seenVolatile
		if !isStable {
			seenVolatile = true
		}
		var tcJSON string
		if len(m.ToolCalls) > 0 {
			b, err := json.Marshal(m.ToolCalls)
			if err != nil {
				return promptPlan{}, fmt.Errorf("llama: marshal tool calls: %w", err)
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
			ByteHash:      hashString(text),
			ToolCallsJSON: tcJSON,
			ToolCallID:    m.ToolCallID,
		})
	}

	stableText := stable.String()
	volatileText := volatile.String()
	manifest := ContextManifest{
		ProfileID:            id.ProfileID,
		Backend:              backendName,
		BackendVersion:       id.BackendVersion,
		ModelDigest:          id.ModelDigest,
		PromptFormat:         renderer.format,
		PromptTemplateDigest: renderer.templateDigest,
		RuntimeDigest:        runtimeDigest(cfg),
		AddBOS:               !cfg.DisableBOS,
		StableBytes:          len(stableText),
		TotalBytes:           len(stableText) + len(volatileText),
		StableByteHash:       hashString(stableText),
		Segments:             segments,
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

func rendererForFormat(format, overrideDigest string) (promptRenderer, error) {
	switch format {
	case "", promptFormatChatML:
		digest := overrideDigest
		if digest == "" {
			digest = promptTemplateDigest(promptFormatChatML)
		}
		return promptRenderer{
			format:             promptFormatChatML,
			templateDigest:     digest,
			renderMessage:      renderChatMLMessage,
			renderAssistantCue: func() string { return "<|assistant|>\n" },
		}, nil
	case promptFormatLlama3:
		digest := overrideDigest
		if digest == "" {
			digest = promptTemplateDigest(promptFormatLlama3)
		}
		return promptRenderer{
			format:             promptFormatLlama3,
			templateDigest:     digest,
			renderMessage:      renderLlama3Message,
			renderAssistantCue: func() string { return "<|start_header_id|>assistant<|end_header_id|>\n\n" },
		}, nil
	default:
		return promptRenderer{}, NewUnsupportedFeatureError("prompt format " + format)
	}
}

func promptTemplateDigest(format string) string {
	switch format {
	case "", promptFormatChatML:
		return hashString("chatml:v1:<|role|>\\ncontent\\n:<|assistant|>\\n")
	case promptFormatLlama3:
		return hashString("llama3:v1:<|start_header_id|>role<|end_header_id|>\\n\\ncontent<|eot_id|>:assistant-cue")
	default:
		return ""
	}
}

func renderChatMLMessage(m modelrepo.Message) (string, error) {
	return fmt.Sprintf("<|%s|>\n%s\n", m.Role, m.Content), nil
}

func renderLlama3Message(m modelrepo.Message) (string, error) {
	return fmt.Sprintf("<|start_header_id|>%s<|end_header_id|>\n\n%s<|eot_id|>", m.Role, m.Content), nil
}

func segmentKindForRole(role string) string {
	switch role {
	case "system":
		return segmentSystem
	case "assistant":
		return segmentAssistant
	case "tool":
		return "tool"
	default:
		return segmentUser
	}
}

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
	Adapters       []AdapterSpec
}

type promptPlan struct {
	Stable   PrefixInput
	Volatile SuffixInput
}

type promptRenderer struct {
	format         string
	templateDigest string
}

func buildPromptPlan(messages []modelrepo.Message, cfg Config, id promptIdentity, toolsJSON string) (promptPlan, error) {
	cfg = normalizeConfig(cfg)
	renderer, err := rendererForFormat(cfg.PromptFormat, cfg.PromptTemplateDigest)
	if err != nil {
		return promptPlan{}, err
	}
	messages, err = normalizeMessagesForTemplate(messages, toolsJSON)
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
		RuntimeDigest:        runtimeDigest(cfg, id.Adapters),
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

func normalizeMessagesForTemplate(messages []modelrepo.Message, toolsJSON string) ([]modelrepo.Message, error) {
	textOnly := strings.TrimSpace(toolsJSON) == ""
	systemParts := make([]string, 0, 1)
	turns := make([]modelrepo.Message, 0, len(messages))
	for _, m := range messages {
		if err := validateMessage(m); err != nil {
			return nil, err
		}
		switch m.Role {
		case "system":
			if content := strings.TrimSpace(m.Content); content != "" {
				systemParts = append(systemParts, content)
			}
		case "tool":
			if textOnly {
				turns = append(turns, modelrepo.Message{Role: "user", Content: toolResultText(m)})
			} else {
				turns = append(turns, m)
			}
		case "assistant":
			if textOnly && len(m.ToolCalls) > 0 {
				m.Content = assistantToolCallText(m)
				m.ToolCalls = nil
			}
			turns = append(turns, m)
		default:
			turns = append(turns, m)
		}
	}
	if textOnly {
		turns = coalesceAlternatingTextTurns(turns)
	}
	out := make([]modelrepo.Message, 0, len(turns)+1)
	if len(systemParts) > 0 {
		out = append(out, modelrepo.Message{Role: "system", Content: strings.Join(systemParts, "\n\n")})
	}
	out = append(out, turns...)
	return out, nil
}

func coalesceAlternatingTextTurns(messages []modelrepo.Message) []modelrepo.Message {
	out := make([]modelrepo.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Role {
		case "user", "assistant":
		default:
			m.Content = roleLabel(m.Role) + ":\n" + m.Content
			m.Role = "user"
		}
		m.ToolCalls = nil
		m.ToolCallID = ""
		if strings.TrimSpace(m.Content) == "" {
			continue
		}
		if len(out) == 0 && m.Role != "user" {
			m.Content = roleLabel(m.Role) + ":\n" + m.Content
			m.Role = "user"
		}
		if n := len(out); n > 0 && out[n-1].Role == m.Role {
			out[n-1].Content = joinTurnText(out[n-1].Content, m.Content)
			continue
		}
		out = append(out, m)
	}
	return out
}

func assistantToolCallText(m modelrepo.Message) string {
	var b strings.Builder
	if content := strings.TrimSpace(m.Content); content != "" {
		b.WriteString(content)
		b.WriteString("\n\n")
	}
	b.WriteString("Assistant requested tool calls:")
	if payload, err := json.Marshal(m.ToolCalls); err == nil {
		b.WriteByte('\n')
		b.Write(payload)
	}
	return b.String()
}

func toolResultText(m modelrepo.Message) string {
	label := "Tool result"
	if id := strings.TrimSpace(m.ToolCallID); id != "" {
		label += " for " + id
	}
	if content := strings.TrimSpace(m.Content); content != "" {
		return label + ":\n" + content
	}
	return label + "."
}

func roleLabel(role string) string {
	if strings.TrimSpace(role) == "" {
		return "Message"
	}
	return strings.ToUpper(role[:1]) + role[1:] + " message"
}

func joinTurnText(a, b string) string {
	if strings.TrimSpace(a) == "" {
		return b
	}
	if strings.TrimSpace(b) == "" {
		return a
	}
	return a + "\n\n" + b
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
			format:         promptFormatChatML,
			templateDigest: digest,
		}, nil
	case promptFormatLlama3:
		digest := overrideDigest
		if digest == "" {
			digest = promptTemplateDigest(promptFormatLlama3)
		}
		return promptRenderer{
			format:         promptFormatLlama3,
			templateDigest: digest,
		}, nil
	default:
		return promptRenderer{}, NewUnsupportedFeatureError("prompt format " + format)
	}
}

func promptTemplateDigest(format string) string {
	switch format {
	case "", promptFormatChatML:
		return hashString("llama-runtime-prompt-metadata:chatml:v1")
	case promptFormatLlama3:
		return hashString("llama-runtime-prompt-metadata:llama3:v1")
	default:
		return ""
	}
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

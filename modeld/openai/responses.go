package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/runtime/modeld"
)

// request payloads and response parsing for the OpenAI Responses API.

type openAIResponsesRequest struct {
	Model           string                    `json:"model"`
	Input           []openAIResponseInput     `json:"input"`
	Instructions    string                    `json:"instructions,omitempty"`
	MaxOutputTokens *int                      `json:"max_output_tokens,omitempty"`
	Temperature     *float64                  `json:"temperature,omitempty"`
	TopP            *float64                  `json:"top_p,omitempty"`
	Seed            *int                      `json:"seed,omitempty"`
	Reasoning       *openAIResponsesReasoning `json:"reasoning,omitempty"`
	Tools           []openAIResponsesTool     `json:"tools,omitempty"`
	ToolChoice      string                    `json:"tool_choice,omitempty"`
	Stream          bool                      `json:"stream,omitempty"`
}

type openAIResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type openAIResponsesTool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
	Strict      bool   `json:"strict"`
}

type openAIResponseInput struct {
	Type string `json:"type"`
	// message fields
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`
	// function_call fields (assistant tool-call history)
	CallID    string `json:"call_id,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	// function_call_output fields (tool result)
	Output string `json:"output,omitempty"`
}

type openAIResponse struct {
	Output    []openAIResponseOutputItem `json:"output"`
	Reasoning struct {
		Effort  string `json:"effort"`
		Summary string `json:"summary"`
	} `json:"reasoning"`
}

type openAIResponseOutputItem struct {
	Type      string                  `json:"type"`
	ID        string                  `json:"id"`
	Role      string                  `json:"role"`
	CallID    string                  `json:"call_id"`
	Name      string                  `json:"name"`
	Arguments string                  `json:"arguments"`
	Content   []openAIResponseContent `json:"content"`
	Status    string                  `json:"status"`
	Phase     string                  `json:"phase"`
}

type openAIResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func buildOpenAIResponsesRequestWithCapabilities(modelName string, messages []modeld.Message, args []modeld.ChatArgument, supportsThink bool) (openAIResponsesRequest, map[string]string) {
	req := openAIResponsesRequest{
		Model: modelName,
	}

	cfg := &modeld.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}

	req.Temperature = cfg.Temperature
	req.MaxOutputTokens = cfg.MaxTokens
	req.TopP = cfg.TopP
	req.Seed = cfg.Seed

	if supportsThink {
		reasoningEffort := openAIReasoningEffort(modelName, cfg.Think)
		if reasoningEffort != "" {
			req.Reasoning = &openAIResponsesReasoning{
				Effort: reasoningEffort,
			}
		}
	}

	if openAIShouldOmitSamplingParams(modelName, func() string {
		if req.Reasoning == nil {
			return ""
		}
		return req.Reasoning.Effort
	}()) {
		req.Temperature = nil
		req.TopP = nil
	}

	nameMap := make(map[string]string)
	origToSanitized := map[string]string{}
	if len(cfg.Tools) > 0 {
		seen := map[string]int{}
		tools := make([]openAIResponsesTool, 0, len(cfg.Tools))
		for i, t := range cfg.Tools {
			if strings.ToLower(t.Type) != "function" || t.Function == nil {
				continue
			}
			orig := t.Function.Name
			name := sanitizeToolName(orig)
			if name == "" {
				name = fmt.Sprintf("tool_%d", i)
			}
			name = uniquifyToolName(seen, name)
			nameMap[name] = orig
			origToSanitized[orig] = name
			tools = append(tools, openAIResponsesTool{
				Type:        "function",
				Name:        name,
				Description: t.Function.Description,
				Parameters:  openAIResponsesToolParameters(t.Function.Parameters),
				Strict:      false,
			})
		}
		if len(tools) > 0 {
			req.Tools = tools
		}
	}

	// Hoist system messages into the top-level instructions field.
	var systemParts []string
	for _, msg := range messages {
		if strings.TrimSpace(msg.Role) == "system" && msg.Content != "" {
			systemParts = append(systemParts, msg.Content)
		}
	}
	if len(systemParts) > 0 {
		req.Instructions = strings.Join(systemParts, "\n\n")
	}

	input := make([]openAIResponseInput, 0, len(messages))
	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)

		switch role {
		case "system":
			// Already hoisted to Instructions above.
			continue

		case "tool":
			// Tool result → function_call_output item, correlated by call_id.
			input = append(input, openAIResponseInput{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: msg.Content,
			})
			continue

		case "assistant", "model":
			// If there is text, emit it as a regular assistant message first.
			if msg.Content != "" {
				input = append(input, openAIResponseInput{
					Type:    "message",
					Role:    "assistant",
					Content: msg.Content,
				})
			}
			// Each tool call becomes a function_call item so the model can
			// correlate it with the following function_call_output items.
			for _, tc := range msg.ToolCalls {
				name := tc.Function.Name
				if san, ok := origToSanitized[name]; ok && san != "" {
					name = san
				} else {
					name = sanitizeToolName(name)
					if name == "" {
						name = "tool"
					}
				}
				args := strings.TrimSpace(tc.Function.Arguments)
				if args == "" {
					args = "{}"
				}
				input = append(input, openAIResponseInput{
					Type:      "function_call",
					CallID:    tc.ID,
					Name:      name,
					Arguments: args,
				})
			}
			continue

		case "user", "developer":
		default:
			role = "user"
		}

		if msg.Content == "" {
			continue
		}
		input = append(input, openAIResponseInput{
			Type:    "message",
			Role:    role,
			Content: msg.Content,
		})
	}
	req.Input = input

	return req, nameMap
}

func openAIResponsesToolParameters(params any) any {
	if params == nil {
		return map[string]any{}
	}
	return params
}

func parseOpenAIResponsesResponse(nameMap map[string]string, raw []byte) (modeld.ChatResult, error) {
	var resp openAIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return modeld.ChatResult{}, fmt.Errorf("responses: decode response: %w", err)
	}
	return parseOpenAIResponsesResponseFromObject(nameMap, resp)
}

func parseOpenAIResponsesResponseFromObject(nameMap map[string]string, response openAIResponse) (modeld.ChatResult, error) {
	resp := response

	if len(resp.Output) == 0 {
		return modeld.ChatResult{}, fmt.Errorf("responses: empty output")
	}

	var textBuilder strings.Builder
	var toolCalls []modeld.ToolCall
	role := "assistant"

	for _, item := range resp.Output {
		switch strings.ToLower(item.Type) {
		case "message":
			if item.Role != "" {
				role = item.Role
			}
			for _, chunk := range item.Content {
				if chunk.Type == "output_text" && chunk.Text != "" {
					textBuilder.WriteString(chunk.Text)
				}
			}
		case "function_call":
			tcID := item.CallID
			if tcID == "" {
				tcID = item.ID
			}
			name := item.Name
			if orig, ok := nameMap[name]; ok && orig != "" {
				name = orig
			}
			args := strings.TrimSpace(item.Arguments)
			if args == "" {
				args = "{}"
			}
			toolCalls = append(toolCalls, modeld.ToolCall{
				ID:   tcID,
				Type: "function",
				Function: struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				}{
					Name:      name,
					Arguments: args,
				},
			})
		}
	}

	if textBuilder.Len() == 0 && len(toolCalls) == 0 {
		return modeld.ChatResult{}, fmt.Errorf("responses: empty output")
	}

	return modeld.ChatResult{
		Message: modeld.Message{
			Role:     role,
			Content:  textBuilder.String(),
			Thinking: strings.TrimSpace(resp.Reasoning.Summary),
		},
		ToolCalls: toolCalls,
	}, nil
}

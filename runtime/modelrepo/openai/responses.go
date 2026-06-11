package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/runtime/runtime/modelrepo"
)

// request payloads and response parsing for the OpenAI Responses API.

type openAIResponsesRequest struct {
	Model          string                  `json:"model"`
	Input          []openAIResponseInput   `json:"input"`
	MaxOutputTokens *int                    `json:"max_output_tokens,omitempty"`
	Temperature    *float64                `json:"temperature,omitempty"`
	TopP           *float64                `json:"top_p,omitempty"`
	Seed           *int                    `json:"seed,omitempty"`
	Reasoning      *openAIResponsesReasoning `json:"reasoning,omitempty"`
	Tools          []openAIResponsesTool    `json:"tools,omitempty"`
	ToolChoice     string                  `json:"tool_choice,omitempty"`
	Stream         bool                    `json:"stream,omitempty"`
}

type openAIResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

type openAIResponsesTool struct {
	Type     string                  `json:"type"`
	Function openAIResponsesToolFunction `json:"function"`
}

type openAIResponsesToolFunction struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

type openAIResponseInput struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openAIResponse struct {
	Output   []openAIResponseOutputItem `json:"output"`
	Reasoning struct {
		Effort  string `json:"effort"`
		Summary string `json:"summary"`
	} `json:"reasoning"`
}

type openAIResponseOutputItem struct {
	Type      string                    `json:"type"`
	ID        string                    `json:"id"`
	Role      string                    `json:"role"`
	CallID    string                    `json:"call_id"`
	Name      string                    `json:"name"`
	Arguments string                    `json:"arguments"`
	Content   []openAIResponseContent   `json:"content"`
	Status    string                    `json:"status"`
	Phase     string                    `json:"phase"`
}

type openAIResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func buildOpenAIResponsesRequestWithCapabilities(modelName string, messages []modelrepo.Message, args []modelrepo.ChatArgument, supportsThink bool) (openAIResponsesRequest, map[string]string) {
	req := openAIResponsesRequest{
		Model: modelName,
	}

	cfg := &modelrepo.ChatConfig{}
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
				Type: "function",
				Function: openAIResponsesToolFunction{
					Name:        name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				},
			})
		}
		if len(tools) > 0 {
			req.Tools = tools
		}
	}

	input := make([]openAIResponseInput, 0, len(messages))
	for _, msg := range messages {
		content := msg.Content

		role := strings.TrimSpace(msg.Role)
		switch role {
		case "tool":
			role = "user"
			if strings.TrimSpace(msg.ToolCallID) != "" && strings.TrimSpace(content) != "" {
				content = "Tool output for " + msg.ToolCallID + ": " + content
			}
		case "assistant", "user", "system", "developer":
		default:
			role = "user"
		}

		if len(msg.ToolCalls) > 0 && (msg.Content != "" || role == "assistant") {
			var toolLines []string
			if content != "" {
				toolLines = append(toolLines, content)
			}
			for _, tc := range msg.ToolCalls {
				name := tc.Function.Name
				if san, ok := origToSanitized[name]; ok && san != "" {
					name = san
				} else if san := sanitizeToolName(name); san != "" {
					name = san
				}
				if name == "" {
					name = "tool"
				}
				args := strings.TrimSpace(tc.Function.Arguments)
				if args == "" {
					args = "{}"
				}
				toolLines = append(toolLines, fmt.Sprintf("Tool call %s: %s", name, args))
			}
			content = strings.Join(toolLines, "\n")
		}

	var contentAny any = content
	if content == "" {
		contentAny = nil
	}

		input = append(input, openAIResponseInput{
			Type:    "message",
			Role:    role,
			Content: contentAny,
		})
	}
	req.Input = input

	return req, nameMap
}

func parseOpenAIResponsesResponse(nameMap map[string]string, raw []byte) (modelrepo.ChatResult, error) {
	var resp openAIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return modelrepo.ChatResult{}, fmt.Errorf("responses: decode response: %w", err)
	}
	return parseOpenAIResponsesResponseFromObject(nameMap, resp)
}

func parseOpenAIResponsesResponseFromObject(nameMap map[string]string, response openAIResponse) (modelrepo.ChatResult, error) {
	resp := response

	if len(resp.Output) == 0 {
		return modelrepo.ChatResult{}, fmt.Errorf("responses: empty output")
	}

	var textBuilder strings.Builder
	var toolCalls []modelrepo.ToolCall
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
			toolCalls = append(toolCalls, modelrepo.ToolCall{
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
		return modelrepo.ChatResult{}, fmt.Errorf("responses: empty output")
	}

	return modelrepo.ChatResult{
		Message: modelrepo.Message{
			Role:     role,
			Content:  textBuilder.String(),
			Thinking: strings.TrimSpace(resp.Reasoning.Summary),
		},
		ToolCalls: toolCalls,
	}, nil
}

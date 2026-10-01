package openai

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/contenox/contenox/internal/models/modelrepo"
)

// openAIResponsesRequest always disables server-side response storage.
type openAIResponsesRequest struct {
	Include         []string                  `json:"include,omitempty"`
	Model           string                    `json:"model"`
	Input           []openAIResponseInput     `json:"input"`
	Instructions    string                    `json:"instructions"`
	MaxOutputTokens *int                      `json:"max_output_tokens,omitempty"`
	Temperature     *float64                  `json:"temperature,omitempty"`
	TopP            *float64                  `json:"top_p,omitempty"`
	Seed            *int                      `json:"seed,omitempty"`
	Reasoning       *openAIResponsesReasoning `json:"reasoning,omitempty"`
	Tools           []openAIResponsesTool     `json:"tools,omitempty"`
	ToolChoice      string                    `json:"tool_choice,omitempty"`
	Stream          bool                      `json:"stream,omitempty"`
	Store           *bool                     `json:"store,omitempty"`
	PromptCacheKey  string                    `json:"prompt_cache_key,omitempty"`
}

type openAIResponsesReasoning struct {
	Effort  string `json:"effort,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type openAIResponsesTool struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters"`
	Strict      bool   `json:"strict"`
}

type openAIResponseInput struct {
	raw              json.RawMessage
	ID               string                   `json:"id,omitempty"`
	EncryptedContent string                   `json:"encrypted_content,omitempty"`
	Summary          *[]openAIResponseContent `json:"summary,omitempty"`
	Type             string                   `json:"type"`
	Role             string                   `json:"role,omitempty"`
	Content          any                      `json:"content,omitempty"`
	CallID           string                   `json:"call_id,omitempty"`
	Name             string                   `json:"name,omitempty"`
	Arguments        string                   `json:"arguments,omitempty"`
	Output           *string                  `json:"output,omitempty"`
}

func (item openAIResponseInput) MarshalJSON() ([]byte, error) {
	if item.raw != nil {
		return item.raw, nil
	}
	type wire openAIResponseInput
	return json.Marshal(wire(item))
}

type openAIResponse struct {
	Output []openAIResponseOutputItem `json:"output"`
	Usage  *openAIResponsesUsage      `json:"usage"`
	Error  *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Status            string `json:"status"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type openAIResponsesUsage struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	TotalTokens        int `json:"total_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputTokensDetails struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
	CacheWriteTokens int `json:"cache_write_tokens"`
}

func (u *openAIResponsesUsage) neutralUsage() *modelrepo.TokenUsage {
	if u == nil {
		return nil
	}
	total := u.TotalTokens
	if total == 0 {
		total = u.InputTokens + u.OutputTokens
	}
	return &modelrepo.TokenUsage{
		PromptTokens:     u.InputTokens,
		CompletionTokens: u.OutputTokens,
		ThinkingTokens:   u.OutputTokensDetails.ReasoningTokens,
		TotalTokens:      total,
		CacheReadTokens:  u.InputTokensDetails.CachedTokens,
		CacheWriteTokens: u.CacheWriteTokens,
	}
}

type openAIResponseOutputItem struct {
	raw              json.RawMessage
	EncryptedContent string                  `json:"encrypted_content,omitempty"`
	Type             string                  `json:"type"`
	ID               string                  `json:"id"`
	Role             string                  `json:"role"`
	CallID           string                  `json:"call_id"`
	Name             string                  `json:"name"`
	Arguments        string                  `json:"arguments"`
	Content          []openAIResponseContent `json:"content"`
	Summary          []openAIResponseContent `json:"summary"`
	Status           string                  `json:"status"`
	Phase            string                  `json:"phase"`
}

func (item *openAIResponseOutputItem) UnmarshalJSON(data []byte) error {
	type wire openAIResponseOutputItem
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*item = openAIResponseOutputItem(decoded)
	item.raw = append(json.RawMessage(nil), data...)
	return nil
}

type openAIResponseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type openAIResponseInputContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

func openAIResponsesImageContent(msg modelrepo.Message) []openAIResponseInputContent {
	parts := make([]openAIResponseInputContent, 0, len(msg.Images)+1)
	if msg.Content != "" {
		parts = append(parts, openAIResponseInputContent{Type: "input_text", Text: msg.Content})
	}
	for _, img := range msg.Images {
		parts = append(parts, openAIResponseInputContent{
			Type:     "input_image",
			ImageURL: imageDataURI(img.MimeType, img.Data),
		})
	}
	return parts
}

func buildOpenAIResponsesRequestWithCapabilities(modelName string, messages []modelrepo.Message, args []modelrepo.ChatArgument, supportsThink bool) (openAIResponsesRequest, map[string]string) {
	return buildOpenAIResponsesRequestWithContinuation(modelName, messages, args, supportsThink, nil)
}

func buildOpenAIResponsesRequestWithContinuation(modelName string, messages []modelrepo.Message, args []modelrepo.ChatArgument, supportsThink bool, continuations map[int][]openAIResponseInput) (openAIResponsesRequest, map[string]string) {
	req := openAIResponsesRequest{
		Model: modelName,
	}
	storeFalse := false
	req.Store = &storeFalse

	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}

	req.Temperature = cfg.Temperature
	req.MaxOutputTokens = cfg.MaxTokens
	req.TopP = cfg.TopP
	req.Seed = cfg.Seed

	if cfg.CacheHints != nil && cfg.CacheHints.SessionKey != "" {
		req.PromptCacheKey = cfg.CacheHints.SessionKey
	}

	if supportsThink {
		reasoningEffort := openAIReasoningEffort(modelName, cfg.Think)
		if reasoningEffort != "" && reasoningEffort != "none" {
			req.Reasoning = &openAIResponsesReasoning{
				Effort:  reasoningEffort,
				Summary: "auto",
			}
		} else if reasoningEffort == "none" {
			req.Reasoning = &openAIResponsesReasoning{Effort: reasoningEffort}
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
	for i, msg := range messages {
		role := strings.TrimSpace(msg.Role)
		input = append(input, continuations[i]...)
		if len(continuations[i]) > 0 && continuations[i][0].raw != nil {
			continue
		}

		switch role {
		case "system":
			continue

		case "tool":
			input = append(input, openAIResponseInput{
				Type:   "function_call_output",
				CallID: msg.ToolCallID,
				Output: &msg.Content,
			})
			continue

		case "assistant", "model":
			if msg.Content != "" {
				input = append(input, openAIResponseInput{
					Type:    "message",
					Role:    "assistant",
					Content: msg.Content,
				})
			}
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

		if msg.Content == "" && len(msg.Images) == 0 {
			continue
		}
		var content any = msg.Content
		if len(msg.Images) > 0 {
			content = openAIResponsesImageContent(msg)
		}
		input = append(input, openAIResponseInput{
			Type:    "message",
			Role:    role,
			Content: content,
		})
	}
	req.Input = input

	return req, nameMap
}

func (c *openAIClient) buildResponsesRequest(messages []modelrepo.Message, args []modelrepo.ChatArgument) (openAIResponsesRequest, map[string]string, error) {
	continuations := make(map[int][]openAIResponseInput)
	if c.codex != nil {
		for i, m := range messages {
			if m.Continuation == nil || m.Role != "assistant" || m.Continuation.Provider != c.codex.Type() || m.Continuation.Model != c.modelName {
				continue
			}
			items, err := decodeResponseContinuation(m.Continuation.Items)
			if err != nil {
				return openAIResponsesRequest{}, nil, err
			}
			continuations[i] = items
		}
	}
	req, names := buildOpenAIResponsesRequestWithContinuation(c.modelName, messages, args, c.supportsThink, continuations)
	c.clampResponsesMaxOutputTokens(&req)
	if c.codex != nil {
		req.Include = []string{"reasoning.encrypted_content"}
		req.MaxOutputTokens, req.Temperature, req.TopP, req.Seed = nil, nil, nil, nil
		for i := range req.Input {
			item := &req.Input[i]
			if text, ok := item.Content.(string); ok {
				typ := "input_text"
				if item.Role == "assistant" {
					typ = "output_text"
				}
				item.Content = []openAIResponseContent{{Type: typ, Text: text}}
			}
		}
	}
	return req, names, nil
}

type responseContinuation struct {
	Output []json.RawMessage `json:"output"`
}

func decodeResponseContinuation(raw json.RawMessage) ([]openAIResponseInput, error) {
	var replay responseContinuation
	if len(raw) > 0 && strings.HasPrefix(strings.TrimSpace(string(raw)), "[") {
		var items []openAIResponseInput
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("invalid ChatGPT continuation: %w", err)
		}
		for _, item := range items {
			if item.Type != "reasoning" || item.EncryptedContent == "" {
				return nil, fmt.Errorf("invalid ChatGPT reasoning item")
			}
		}
		return items, nil
	}
	if err := json.Unmarshal(raw, &replay); err != nil {
		return nil, fmt.Errorf("invalid ChatGPT continuation: %w", err)
	}
	if len(replay.Output) == 0 {
		return nil, fmt.Errorf("invalid ChatGPT continuation: missing output")
	}
	items := make([]openAIResponseInput, len(replay.Output))
	for i, raw := range replay.Output {
		var item openAIResponseInput
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("invalid ChatGPT continuation item: %w", err)
		}
		switch item.Type {
		case "reasoning", "message", "function_call":
		default:
			return nil, fmt.Errorf("unsupported ChatGPT continuation item type %q", item.Type)
		}
		item.raw = raw
		items[i] = item
	}
	return items, nil
}

func (c *openAIClient) responseContinuation(response *openAIResponse) *modelrepo.Continuation {
	if c.codex == nil || response == nil || len(response.Output) == 0 {
		return nil
	}
	output := make([]json.RawMessage, 0, len(response.Output))
	for _, item := range response.Output {
		output = append(output, item.raw)
	}
	raw, err := json.Marshal(responseContinuation{Output: output})
	if err != nil {
		return nil
	}
	return &modelrepo.Continuation{Provider: c.codex.Type(), Model: c.modelName, Items: raw}
}

func responsesReasoningSummaryText(resp *openAIResponse) string {
	if resp == nil {
		return ""
	}
	var b strings.Builder
	for _, item := range resp.Output {
		if strings.ToLower(item.Type) != "reasoning" {
			continue
		}
		for _, part := range item.Summary {
			if part.Type == "summary_text" && part.Text != "" {
				if b.Len() > 0 {
					b.WriteString("\n\n")
				}
				b.WriteString(part.Text)
			}
		}
	}
	return b.String()
}

func openAIResponsesToolParameters(params any) any {
	if params == nil {
		return map[string]any{}
	}
	return params
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
	var thinkingBuilder strings.Builder
	var toolCalls []modelrepo.ToolCall
	role := "assistant"

	for _, item := range resp.Output {
		switch strings.ToLower(item.Type) {
		case "reasoning":
			for _, part := range item.Summary {
				if part.Type == "summary_text" && part.Text != "" {
					if thinkingBuilder.Len() > 0 {
						thinkingBuilder.WriteString("\n\n")
					}
					thinkingBuilder.WriteString(part.Text)
				}
			}
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

	finishReason := ""
	if resp.Status == "incomplete" && resp.IncompleteDetails != nil {
		finishReason = resp.IncompleteDetails.Reason
	}

	return modelrepo.ChatResult{
		Message: modelrepo.Message{
			Role:     role,
			Content:  textBuilder.String(),
			Thinking: strings.TrimSpace(thinkingBuilder.String()),
		},
		ToolCalls:    toolCalls,
		Usage:        resp.Usage.neutralUsage(),
		FinishReason: finishReason,
	}, nil
}

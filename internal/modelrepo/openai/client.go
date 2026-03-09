package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/contenox/contenox/internal/modelrepo"
	"github.com/contenox/contenox/libtracker"
)

type openAIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	modelName  string
	maxTokens  int
	tracker    libtracker.ActivityTracker
}

type openAIChatRequest struct {
	Model       string           `json:"model"`
	Messages    []apiChatMessage `json:"messages"`
	Temperature *float64         `json:"temperature,omitempty"`
	MaxTokens   *int             `json:"max_tokens,omitempty"`
	TopP        *float64         `json:"top_p,omitempty"`
	Seed        *int             `json:"seed,omitempty"`
	Stream      bool             `json:"stream,omitempty"`
	Tools       []openAITool     `json:"tools,omitempty"`
	// ReasoningEffort controls thinking depth for o-series models (o1, o3, o4-mini etc.).
	// Accepted values: "low", "medium", "high". Empty = omitted (non-reasoning models).
	// Note: when set, Temperature must be omitted (OpenAI rejects temperature on o-series).
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// apiChatMessage is the wire-format message sent to the OpenAI REST API.
// We use *string for Content so assistant messages with tool_calls can have null content.
type apiChatMessage struct {
	Role       string           `json:"role"`
	Content    *string          `json:"content"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []apiToolCallReq `json:"tool_calls,omitempty"`
}

type apiToolCallReq struct {
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Function openAIFunction2 `json:"function"`
}

type openAIFunction2 struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAITool struct {
	Type     string         `json:"type"` // must be "function"
	Function openAIFunction `json:"function"`
}

type openAIFunction struct {
	Name        string      `json:"name"`                  // ^[a-zA-Z0-9_-]+$
	Description string      `json:"description,omitempty"` // optional
	Parameters  interface{} `json:"parameters,omitempty"`  // JSON Schema
}

func (c *openAIClient) sendRequest(ctx context.Context, endpoint string, request interface{}, response interface{}) error {
	url := c.baseURL + endpoint

	tracker := c.tracker
	auth := "***"
	if len(c.apiKey) > 24 {
		auth = c.apiKey[:24]
	}
	reportErr, reportChange, end := tracker.Start(
		ctx,
		"http_request",
		"openai",
		"model", c.modelName,
		"endpoint", endpoint,
		"base_url", c.baseURL,
		"auth", auth,
	)
	defer end()

	var reqBody io.Reader
	if request != nil {
		marshaledReqBody, err := json.Marshal(request)
		if err != nil {
			err = fmt.Errorf("failed to marshal request: %w", err)
			reportErr(err)
			return err
		}
		reqBody = bytes.NewBuffer(marshaledReqBody)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, reqBody)
	if err != nil {
		err = fmt.Errorf("failed to create request: %w", err)
		reportErr(err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
		reportErr(err)
		return err
	}
	defer resp.Body.Close()

	// Log response headers (including rate-limit headers) via tracker
	reportChange("http_response", map[string]any{
		"status_code": resp.StatusCode,
		"headers":     resp.Header,
	})

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Error struct {
				Message string      `json:"message"`
				Type    string      `json:"type"`
				Code    interface{} `json:"code"`
			} `json:"error"`
		}
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			if jsonErr := json.Unmarshal(bodyBytes, &errorResponse); jsonErr == nil && errorResponse.Error.Message != "" {
				err = fmt.Errorf("OpenAI API returned non-200 status: %d, Type: %s, Code: %v, Message: %s for model %s",
					resp.StatusCode, errorResponse.Error.Type, errorResponse.Error.Code, errorResponse.Error.Message, c.modelName)
				reportErr(err)
				return err
			}
			err = fmt.Errorf("OpenAI API returned non-200 status: %d, body: %s for model %s",
				resp.StatusCode, string(bodyBytes), c.modelName)
			reportErr(err)
			return err
		}
		err = fmt.Errorf("OpenAI API returned non-200 status: %d for model %s", resp.StatusCode, c.modelName)
		reportErr(err)
		return err
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			err = fmt.Errorf("failed to decode response for model %s: %w", c.modelName, err)
			reportErr(err)
			return err
		}
	}

	reportChange("request_completed", nil)
	return nil
}

// buildOpenAIRequest builds a compliant request and sanitizes tool names per
// OpenAI's pattern (^[a-zA-Z0-9_-]+$). It ALSO returns a map from
// sanitized->original so callers can translate tool-call names back.
//
// Critically, it also sanitizes tool_calls[].function.name in the message
// history: the taskengine qualifies tool names as "hookName.toolName"
// (e.g. "filesystem.list_directory"). The dot violates OpenAI's pattern,
// so any prior-turn assistant messages must have their tool call names
// sanitized before being forwarded to the API.
func buildOpenAIRequest(modelName string, messages []modelrepo.Message, args []modelrepo.ChatArgument) (openAIChatRequest, map[string]string) {
	req := openAIChatRequest{
		Model: modelName,
	}

	// Apply chat args
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}
	req.Temperature = cfg.Temperature
	req.MaxTokens = cfg.MaxTokens
	req.TopP = cfg.TopP
	req.Seed = cfg.Seed

	// Wire reasoning_effort for o-series models.
	// "true"/"high" = "high", "medium" = "medium", "low" = "low".
	// When reasoning_effort is set, temperature must be cleared (OpenAI rejects it on o-series).
	if cfg.Think != nil {
		switch *cfg.Think {
		case "true", "high":
			req.ReasoningEffort = "high"
			req.Temperature = nil
		case "medium":
			req.ReasoningEffort = "medium"
			req.Temperature = nil
		case "low":
			req.ReasoningEffort = "low"
			req.Temperature = nil
		}
	}

	// Convert tools to OpenAI tools with sanitized/unique function names.
	nameMap := make(map[string]string) // sanitized -> original
	seen := map[string]int{}
	if len(cfg.Tools) > 0 {
		tools := make([]openAITool, 0, len(cfg.Tools))
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
			tools = append(tools, openAITool{
				Type: "function",
				Function: openAIFunction{
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

	// Build reverse map: original tool name -> sanitized name, for rewriting history.
	origToSanitized := make(map[string]string, len(nameMap))
	for san, orig := range nameMap {
		origToSanitized[orig] = san
	}

	// Convert messages to the explicit wire format.
	// • Content is *string so assistant messages with tool_calls can have a null body.
	// • ToolCalls in assistant messages have their names sanitized via origToSanitized.
	// • tool_call_id is preserved on tool-role messages.
	apiMsgs := make([]apiChatMessage, 0, len(messages))
	for _, msg := range messages {
		content := msg.Content
		var contentPtr *string
		// For assistant messages that only have tool calls, content may be empty — send null.
		if content != "" || len(msg.ToolCalls) == 0 {
			contentPtr = &content
		}

		apiMsg := apiChatMessage{
			Role:       msg.Role,
			Content:    contentPtr,
			ToolCallID: msg.ToolCallID,
		}

		if len(msg.ToolCalls) > 0 {
			apiMsg.ToolCalls = make([]apiToolCallReq, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				name := tc.Function.Name
				if san, ok := origToSanitized[name]; ok {
					name = san
				} else {
					name = sanitizeToolName(name)
				}
				apiMsg.ToolCalls = append(apiMsg.ToolCalls, apiToolCallReq{
					ID:   tc.ID,
					Type: tc.Type,
					Function: openAIFunction2{
						Name:      name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
		apiMsgs = append(apiMsgs, apiMsg)
	}
	req.Messages = apiMsgs

	return req, nameMap
}


// sanitizeToolName replaces invalid characters with '_' and trims leading/trailing separators.
// Allowed: letters, digits, underscore, hyphen.
func sanitizeToolName(in string) string {
	if in == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range in {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	s := b.String()
	// avoid leading/trailing separators
	s = strings.Trim(s, "_-")
	return s
}

// uniquifyToolName ensures we don't send duplicate names (OpenAI recommends unique names)
func uniquifyToolName(seen map[string]int, name string) string {
	if _, ok := seen[name]; !ok {
		seen[name] = 1
		return name
	}
	// append an incrementing suffix until unique
	i := seen[name]
	for {
		candidate := fmt.Sprintf("%s_%d", name, i)
		if _, ok := seen[candidate]; !ok {
			seen[name] = i + 1
			seen[candidate] = 1
			return candidate
		}
		i++
	}
}

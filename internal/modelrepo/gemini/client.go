package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/contenox/runtime/internal/modelrepo"
	"github.com/contenox/runtime/libtracker"
)

type geminiClient struct {
	apiKey     string
	modelName  string
	baseURL    string
	httpClient *http.Client
	maxTokens  int
	tracker    libtracker.ActivityTracker
}

type geminiGenerateContentRequest struct {
	SystemInstruction *geminiSystemInstruction `json:"system_instruction,omitempty"`
	Contents          []geminiContent          `json:"contents"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
	Tools             []geminiToolRequest      `json:"tools,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	TopK            *int     `json:"topK,omitempty"`
	CandidateCount  *int     `json:"candidateCount,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
	Seed            *int     `json:"seed,omitempty"`
}

// sendRequest: shared HTTP helper for Gemini clients
func (c *geminiClient) sendRequest(ctx context.Context, endpoint string, request interface{}, response interface{}) error {
	fullURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)

	tracker := c.tracker
	reportErr, reportChange, end := tracker.Start(
		ctx,
		"http_request",
		"gemini",
		"model", c.modelName,
		"endpoint", endpoint,
		"base_url", c.baseURL,
	)
	defer end()

	var reqBody io.Reader
	if request != nil {
		b, err := json.Marshal(request)
		if err != nil {
			err = fmt.Errorf("failed to marshal request: %w", err)
			reportErr(err)
			return err
		}
		reqBody = bytes.NewBuffer(b)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, reqBody)
	if err != nil {
		err = fmt.Errorf("failed to create request: %w", err)
		reportErr(err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
		reportErr(err)
		return err
	}
	defer resp.Body.Close()

	// Log headers via tracker
	reportChange("http_response", map[string]any{
		"status_code": resp.StatusCode,
		"headers":     resp.Header,
	})

	if resp.StatusCode != http.StatusOK {
		var eresp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		body, _ := io.ReadAll(resp.Body)
		if jsonErr := json.Unmarshal(body, &eresp); jsonErr == nil && eresp.Error.Message != "" {
			err = fmt.Errorf("gemini API error: %d %s - %s (model=%s url=%s)",
				resp.StatusCode, eresp.Error.Status, eresp.Error.Message, c.modelName, fullURL)
			reportErr(err)
			return err
		}
		err = fmt.Errorf("gemini API error: %d - %s (model=%s url=%s)", resp.StatusCode, string(body), c.modelName, fullURL)
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

// buildGeminiRequest builds a proper Gemini generateContent request using modelrepo args & tools
func buildGeminiRequest(_ string, messages []modelrepo.Message, systemInstruction *geminiSystemInstruction, args []modelrepo.ChatArgument) geminiGenerateContentRequest {
	// Collect chat args
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}

	// Convert tools -> Gemini tool declarations
	var tools []geminiToolRequest
	if len(cfg.Tools) > 0 {
		decls := make([]geminiFunctionDeclaration, 0, len(cfg.Tools))
		for _, t := range cfg.Tools {
			if t.Type == "function" && t.Function != nil {
				decls = append(decls, geminiFunctionDeclaration{
					Name:        t.Function.Name,
					Description: t.Function.Description,
					Parameters:  t.Function.Parameters,
				})
			}
		}
		if len(decls) > 0 {
			tools = append(tools, geminiToolRequest{
				FunctionDeclarations: decls,
			})
		}
	}

	req := geminiGenerateContentRequest{
		SystemInstruction: systemInstruction,
		Contents:          convertToGeminiMessages(messages),
		GenerationConfig:  &geminiGenerationConfig{},
		Tools:             tools,
	}
	req.GenerationConfig.Temperature = cfg.Temperature
	req.GenerationConfig.TopP = cfg.TopP
	if cfg.MaxTokens != nil {
		req.GenerationConfig.MaxOutputTokens = cfg.MaxTokens
	}
	req.GenerationConfig.Seed = cfg.Seed

	return req
}

// convert modelrepo messages to Gemini "contents"
func convertToGeminiMessages(messages []modelrepo.Message) []geminiContent {
	out := make([]geminiContent, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" {
			// handled via SystemInstruction
			continue
		}
		role := m.Role
		if role == "assistant" {
			role = "model"
		}
		parts := []geminiPart{}
		if m.Content != "" {
			parts = append(parts, geminiPart{Text: m.Content})
		}
		if len(parts) == 0 {
			continue
		}
		out = append(out, geminiContent{
			Role:  role,
			Parts: parts,
		})
	}
	return out
}

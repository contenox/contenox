package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/contenox/runtime/internal/modelrepo"
)

type geminiClient struct {
	apiKey     string
	modelName  string
	baseURL    string
	httpClient *http.Client
	maxTokens  int
}

type geminiGenerateContentRequest struct {
	SystemInstruction *geminiSystemInstruction `json:"system_instruction,omitempty"`
	Contents          []geminiContent          `json:"contents"`
	GenerationConfig  *geminiGenerationConfig  `json:"generationConfig,omitempty"`
	Tools             []modelrepo.Tool         `json:"tools,omitempty"`
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

func (c *geminiClient) sendRequest(ctx context.Context, endpoint string, request interface{}, response interface{}) error {
	fullURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)

	var reqBody io.Reader
	if request != nil {
		marshaledReqBody, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewBuffer(marshaledReqBody)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", fullURL, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		bodyBytes, readErr := io.ReadAll(resp.Body)
		if readErr == nil {
			if jsonErr := json.Unmarshal(bodyBytes, &errorResponse); jsonErr == nil && errorResponse.Error.Message != "" {
				return fmt.Errorf("gemini API returned non-200 status: %d, Status: %s, Message: %s for model %s", resp.StatusCode, errorResponse.Error.Status, errorResponse.Error.Message, c.modelName)
			}
			return fmt.Errorf("gemini API returned non-200 status: %d, body: %s for model %s", resp.StatusCode, string(bodyBytes), c.modelName)
		}
		return fmt.Errorf("gemini API returned non-200 status: %d for model %s", resp.StatusCode, c.modelName)
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("failed to decode response for model %s: %w", c.modelName, err)
		}
	}

	return nil
}

func buildGeminiRequest(modelName string, messages []modelrepo.Message, systemInstruction *geminiSystemInstruction, args []modelrepo.ChatArgument) geminiGenerateContentRequest {
	request := geminiGenerateContentRequest{
		SystemInstruction: systemInstruction,
		Contents:          convertToGeminiMessages(messages),
		GenerationConfig:  &geminiGenerationConfig{},
	}

	// Apply all arguments to configure the request
	config := &modelrepo.ChatConfig{}
	for _, arg := range args {
		arg.Apply(config)
	}

	request.GenerationConfig.Temperature = config.Temperature
	request.GenerationConfig.TopP = config.TopP
	if config.MaxTokens != nil {
		request.GenerationConfig.MaxOutputTokens = config.MaxTokens
	}
	request.GenerationConfig.Seed = config.Seed
	request.Tools = config.Tools

	return request
}

func convertToGeminiMessages(messages []modelrepo.Message) []geminiContent {
	geminiMsgs := make([]geminiContent, 0)
	for _, msg := range messages {
		// Skip system messages as they're handled separately
		if msg.Role == "system" {
			continue
		}

		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		geminiMsgs = append(geminiMsgs, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}
	return geminiMsgs
}

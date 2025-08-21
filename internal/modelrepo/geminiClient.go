package modelrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type geminiClient struct {
	apiKey     string
	modelName  string
	baseURL    string
	httpClient *http.Client
	maxTokens  int // Max output tokens, or context length for models like embeddings
}

type geminiPromptClient struct {
	geminiClient
}

func (c *geminiPromptClient) Prompt(ctx context.Context, systeminstruction string, temperature float32, prompt string) (string, error) {
	// Convert the single prompt string into a Gemini-style message array
	geminiMessages := []geminiContent{
		{
			Role:  "user",
			Parts: []geminiPart{{Text: prompt}},
		},
	}

	request := geminiGenerateContentRequest{
		Contents: geminiMessages,
		SystemInstruction: &geminiSystemInstruction{
			Parts: []geminiPart{
				geminiPart{
					Text: systeminstruction,
				},
			},
		},
		GenerationConfig: &geminiGenerationConfig{
			Temperature:     float64(temperature),
			MaxOutputTokens: c.maxTokens,
		},
	}

	endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", c.modelName)
	var response geminiGenerateContentResponse
	if err := c.sendRequest(ctx, endpoint, request, &response); err != nil {
		return "", err
	}

	if len(response.Candidates) == 0 {
		return "", fmt.Errorf("no candidates returned from Gemini for model %s", c.modelName)
	}

	candidate := response.Candidates[0]
	if len(candidate.Content.Parts) == 0 || candidate.Content.Parts[0].Text == "" {
		// Check for specific finish reasons for more informative errors
		if len(candidate.FinishReason) > 0 {
			return "", fmt.Errorf("empty content from model %s despite normal completion. Finish reason: %v", c.modelName, candidate.FinishReason)
		}
		return "", fmt.Errorf("empty content from model %s", c.modelName)
	}
	return candidate.Content.Parts[0].Text, nil
}

type geminiChatClient struct {
	geminiClient
}

func (c *geminiChatClient) Chat(ctx context.Context, messages []Message, options ...ChatOption) (Message, error) {
	geminiMessages, systemInstruction := convertToGeminiMessages(messages)

	request := geminiGenerateContentRequest{
		SystemInstruction: &systemInstruction,
		Contents:          geminiMessages,
		GenerationConfig: &geminiGenerationConfig{
			Temperature:     0.7,         // default
			MaxOutputTokens: c.maxTokens, // default
		},
	}

	// Apply ChatOptions via adapter
	adapter := &geminiChatRequestAdapter{req: &request}
	for _, opt := range options {
		if opt != nil {
			opt.SetTemperature(adapter.req.GenerationConfig.Temperature)
			opt.SetMaxTokens(adapter.req.GenerationConfig.MaxOutputTokens)
		}
	}

	endpoint := fmt.Sprintf("/v1beta/models/%s:generateContent", c.modelName)
	var response geminiGenerateContentResponse
	if err := c.sendRequest(ctx, endpoint, request, &response); err != nil {
		return Message{}, err
	}

	if len(response.Candidates) == 0 {
		return Message{}, fmt.Errorf("no candidates returned from Gemini for model %s", c.modelName)
	}

	candidate := response.Candidates[0]
	if len(candidate.Content.Parts) == 0 || candidate.Content.Parts[0].Text == "" {
		if len(candidate.FinishReason) > 0 {
			return Message{}, fmt.Errorf(
				"empty content from model %s despite normal completion. Finish reason: %v",
				c.modelName, candidate.FinishReason,
			)
		}
		return Message{}, fmt.Errorf("empty content from model %s", c.modelName)
	}

	return Message{Role: "assistant", Content: candidate.Content.Parts[0].Text}, nil
}

// Adapter so ChatOption can modify Gemini requests
type geminiChatRequestAdapter struct {
	req *geminiGenerateContentRequest
}

func (a *geminiChatRequestAdapter) SetTemperature(temp float64) {
	a.req.GenerationConfig.Temperature = temp
}

func (a *geminiChatRequestAdapter) SetMaxTokens(max int) {
	a.req.GenerationConfig.MaxOutputTokens = max
}

// geminiEmbedClient implements serverops.LLMEmbedClient
type geminiEmbedClient struct {
	geminiClient
}

func (c *geminiEmbedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	request := geminiEmbedContentRequest{
		Model: "models/" + c.modelName, // Gemini requires model in request body for embedContent
		Content: geminiContent{
			Parts: []geminiPart{{Text: prompt}},
		},
	}

	endpoint := fmt.Sprintf("/v1beta/models/%s:embedContent", c.modelName)
	var response geminiEmbedContentResponse
	if err := c.sendRequest(ctx, endpoint, request, &response); err != nil {
		return nil, err
	}

	if len(response.Embedding.Values) == 0 {
		return nil, fmt.Errorf("no embedding values returned from Gemini for model %s", c.modelName)
	}
	return response.Embedding.Values, nil
}

type geminiStreamClient struct {
	geminiClient
}

func (c *geminiStreamClient) Stream(ctx context.Context, prompt string) (<-chan *StreamParcel, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *geminiClient) sendRequest(ctx context.Context, endpoint string, request any, response any) error {
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

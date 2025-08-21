package modelrepo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type openAIClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	modelName  string
	maxTokens  int
}

type openAIPromptClient struct {
	openAIClient
}

func (c *openAIPromptClient) Prompt(ctx context.Context, systemMessage string, temperature float32, prompt string) (string, error) {
	request := openAIChatRequest{
		Model:       c.modelName,
		Messages:    []Message{{Role: "system", Content: systemMessage}, {Role: "user", Content: prompt}},
		Temperature: float64(temperature),
		MaxTokens:   c.maxTokens,
	}

	var response openAIChatResponse
	if err := c.sendRequest(ctx, "/chat/completions", request, &response); err != nil {
		return "", err
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no chat completion choices returned from OpenAI for model %s", c.modelName)
	}

	choice := response.Choices[0]
	if choice.Message.Content == "" {
		return "", fmt.Errorf("empty content from model %s despite normal completion. Finish reason: %s", c.modelName, choice.FinishReason)
	}
	return choice.Message.Content, nil
}

type openAIChatClient struct {
	openAIClient
}

type chatRequestAdapter struct {
	req *openAIChatRequest
}

func (a *chatRequestAdapter) SetTemperature(temp float64) {
	a.req.Temperature = temp
}

func (a *chatRequestAdapter) SetMaxTokens(max int) {
	a.req.MaxTokens = max
}

func (c *openAIChatClient) Chat(ctx context.Context, messages []Message, opts ...ChatOption) (Message, error) {
	request := openAIChatRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: 0.5,         // default
		MaxTokens:   c.maxTokens, // default
	}

	// Apply options via adapter
	adapter := &chatRequestAdapter{req: &request}
	for _, opt := range opts {
		if opt != nil {
			// Option receives current value and can override it
			opt.SetTemperature(adapter.req.Temperature)
			opt.SetMaxTokens(adapter.req.MaxTokens)
		}
	}

	var response openAIChatResponse
	if err := c.sendRequest(ctx, "/chat/completions", request, &response); err != nil {
		return Message{}, err
	}

	if len(response.Choices) == 0 {
		return Message{}, fmt.Errorf("no chat choices returned from OpenAI for model %s", c.modelName)
	}

	choice := response.Choices[0]
	if choice.Message.Content == "" {
		return Message{}, fmt.Errorf("empty content from model %s despite normal completion. Finish reason: %s", c.modelName, choice.FinishReason)
	}

	return choice.Message, nil
}

type openAIEmbedClient struct {
	openAIClient
}

func (c *openAIEmbedClient) Embed(ctx context.Context, prompt string) ([]float64, error) {
	request := openAIEmbedRequest{
		Model:          c.modelName,
		Input:          prompt,
		EncodingFormat: "float", // Request float output
	}

	var response openAIEmbedResponse
	if err := c.sendRequest(ctx, "/embeddings", request, &response); err != nil {
		return nil, err
	}

	if len(response.Data) == 0 || len(response.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("no embedding data returned from OpenAI for model %s", c.modelName)
	}
	return response.Data[0].Embedding, nil
}

type openAIStreamClient struct {
	openAIClient
}

func (c *openAIStreamClient) Stream(ctx context.Context, prompt string) (<-chan *StreamParcel, error) {
	return nil, fmt.Errorf("streaming not supported yet for OpenAI")
}

func (c *openAIClient) sendRequest(ctx context.Context, endpoint string, request interface{}, response interface{}) error {
	url := c.baseURL + endpoint

	var reqBody io.Reader
	if request != nil {
		marshaledReqBody, err := json.Marshal(request)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}
		reqBody = bytes.NewBuffer(marshaledReqBody)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
	}
	defer resp.Body.Close()

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
				return fmt.Errorf("OpenAI API returned non-200 status: %d, Type: %s, Code: %v, Message: %s for model %s", resp.StatusCode, errorResponse.Error.Type, errorResponse.Error.Code, errorResponse.Error.Message, c.modelName)
			}
			return fmt.Errorf("OpenAI API returned non-200 status: %d, body: %s for model %s", resp.StatusCode, string(bodyBytes), c.modelName)
		}
		return fmt.Errorf("OpenAI API returned non-200 status: %d for model %s", resp.StatusCode, c.modelName)
	}

	if response != nil {
		if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
			return fmt.Errorf("failed to decode response for model %s: %w", c.modelName, err)
		}
	}

	return nil
}

type openAIChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Choices []struct {
		Index        int     `json:"index"`
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIChatStreamResponseChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index        int     `json:"index"`
		Delta        Message `json:"delta"` // Delta contains partial content
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
	// TODO: usage is only present in the last chunk when stream_options={"include_usage": true}
	// Usage *struct {
	// 	PromptTokens     int `json:"prompt_tokens"`
	// 	CompletionTokens int `json:"completion_tokens"`
	// 	TotalTokens      int `json:"total_tokens"`
	// } `json:"usage,omitempty"`
}

// Structures for Embeddings API
type openAIEmbedRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	EncodingFormat string `json:"encoding_format,omitempty"` // "float" or "base64"
}

type openAIEmbedResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Object    string    `json:"object"`
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

func readLine(r io.Reader) ([]byte, error) {
	var line []byte
	buf := make([]byte, 1) // Read byte by byte
	for {
		n, err := r.Read(buf)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			line = append(line, buf[0])
			if buf[0] == '\n' {
				return line, nil
			}
		}
	}
}

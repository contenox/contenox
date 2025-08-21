package modelrepo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Base client configuration
type vLLMClient struct {
	baseURL    string
	httpClient *http.Client
	modelName  string
	maxTokens  int
	apiKey     string
}

// VLLMPromptClient handles prompt execution
type VLLMPromptClient struct {
	vLLMClient
}

// VLLMChatClient handles chat interaction
type VLLMChatClient struct {
	vLLMClient
}

// NewVLLMPromptClient creates a new prompt client
func NewVLLMPromptClient(ctx context.Context, baseURL, modelName string, contextLength int, httpClient *http.Client, apiKey string) (LLMPromptExecClient, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	client := &VLLMPromptClient{
		vLLMClient: vLLMClient{
			baseURL:    baseURL,
			httpClient: httpClient,
			modelName:  modelName,
			apiKey:     apiKey,
		},
	}
	client.maxTokens = 2048
	if contextLength > 0 {
		client.maxTokens = min(contextLength, client.maxTokens)
	}

	return client, nil
}

func NewVLLMChatClient(ctx context.Context, baseURL, modelName string, contextLength int, httpClient *http.Client, apiKey string) (LLMChatClient, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	client := &VLLMChatClient{
		vLLMClient: vLLMClient{
			baseURL:    baseURL,
			httpClient: httpClient,
			modelName:  modelName,
			apiKey:     apiKey,
		},
	}

	client.maxTokens = 2048
	if contextLength > 0 {
		client.maxTokens = min(contextLength, client.maxTokens)
	}
	return client, nil
}

// Prompt implements LLMPromptExecClient interface
func (c *vLLMClient) Prompt(ctx context.Context, systeminstruction string, temperature float32, prompt string) (string, error) {
	// Convert system instruction and prompt to proper message format
	messages := []Message{
		{Role: "system", Content: systeminstruction},
		{Role: "user", Content: prompt},
	}

	// Create chat request using the same structure as the Chat API
	request := chatRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: float64(temperature),
		MaxTokens:   c.maxTokens,
	}

	// Send request to the chat completions endpoint
	var response chatResponse
	if err := c.sendRequest(ctx, "/v1/chat/completions", request, &response); err != nil {
		return "", err
	}

	// Handle response
	if len(response.Choices) == 0 {
		return "", fmt.Errorf("no completion choices returned from vLLM for model %s", c.modelName)
	}

	choice := response.Choices[0]
	switch choice.FinishReason {
	case "stop":
		if choice.Message.Content == "" {
			return "", fmt.Errorf("empty content from model %s despite normal completion", c.modelName)
		}
		return choice.Message.Content, nil
	case "length":
		return "", fmt.Errorf("token limit reached for model %s (partial response: %q)", c.modelName, choice.Message.Content)
	case "content_filter":
		return "", fmt.Errorf("content filtered for model %s (partial response: %q)", c.modelName, choice.Message.Content)
	default:
		return "", fmt.Errorf("unexpected completion reason %q for model %s", choice.FinishReason, c.modelName)
	}
}

// Chat implements LLMChatClient interface
func (c *VLLMChatClient) Chat(ctx context.Context, messages []Message, options ...ChatOption) (Message, error) {
	request := chatRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: 0.7,         // default
		MaxTokens:   c.maxTokens, // default
	}

	// Apply ChatOptions via adapter
	adapter := &vllmChatRequestAdapter{req: &request}
	for _, opt := range options {
		if opt != nil {
			// First: let option observe current values (for relative adjustments)
			opt.SetTemperature(request.Temperature)
			opt.SetMaxTokens(request.MaxTokens)
			// Second: let option set new values (using adapter to modify request)
			opt.SetTemperature(adapter.req.Temperature)
			opt.SetMaxTokens(adapter.req.MaxTokens)
		}
	}

	var response chatResponse
	if err := c.sendRequest(ctx, "/v1/chat/completions", request, &response); err != nil {
		return Message{}, err
	}

	if len(response.Choices) == 0 {
		return Message{}, fmt.Errorf("no chat choices returned from vLLM for model %s", c.modelName)
	}

	choice := response.Choices[0]
	switch choice.FinishReason {
	case "stop":
		if choice.Message.Content == "" {
			return Message{}, fmt.Errorf("empty content from model %s despite normal completion", c.modelName)
		}
		return choice.Message, nil
	case "length":
		return Message{}, fmt.Errorf(
			"token limit reached for model %s (partial response: %q)",
			c.modelName,
			choice.Message.Content,
		)
	case "content_filter":
		return Message{}, fmt.Errorf(
			"content filtered for model %s (partial response: %q)",
			c.modelName,
			choice.Message.Content,
		)
	default:
		return Message{}, fmt.Errorf(
			"unexpected completion reason %q for model %s",
			choice.FinishReason,
			c.modelName,
		)
	}
}

// Adapter so ChatOption can modify vLLM chat requests
type vllmChatRequestAdapter struct {
	req *chatRequest
}

func (a *vllmChatRequestAdapter) SetTemperature(temp float64) {
	a.req.Temperature = temp
}

func (a *vllmChatRequestAdapter) SetMaxTokens(max int) {
	a.req.MaxTokens = max
}

func (c *vLLMClient) sendRequest(ctx context.Context, endpoint string, request interface{}, response interface{}) error {
	url := c.baseURL + endpoint

	reqBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("vLLM API returned non-200 status: %d for model %s", resp.StatusCode, c.modelName)
	}

	if err := json.NewDecoder(resp.Body).Decode(response); err != nil {
		return fmt.Errorf("failed to decode response for model %s: %w", c.modelName, err)
	}

	return nil
}

type completionRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

type completionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Text         string `json:"text"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
	Stream      bool      `json:"stream,omitempty"`
}

type chatResponse struct {
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

var (
	_ LLMPromptExecClient = (*VLLMPromptClient)(nil)
	_ LLMChatClient       = (*VLLMChatClient)(nil)
)

// VLLMStreamClient handles streaming responses from vLLM
type VLLMStreamClient struct {
	vLLMClient
}

// NewVLLMStreamClient creates a new streaming client
func NewVLLMStreamClient(ctx context.Context, baseURL, modelName string, contextLength int, httpClient *http.Client, apiKey string) (LLMStreamClient, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	client := &VLLMStreamClient{
		vLLMClient: vLLMClient{
			baseURL:    baseURL,
			httpClient: httpClient,
			modelName:  modelName,
			apiKey:     apiKey,
		},
	}

	client.maxTokens = 2048
	if contextLength > 0 {
		client.maxTokens = min(contextLength, client.maxTokens)
	}
	return client, nil
}

// Stream implements LLMStreamClient interface
// Stream implements LLMStreamClient interface
func (c *VLLMStreamClient) Stream(ctx context.Context, prompt string) (<-chan *StreamParcel, error) {
	// Convert prompt to message format
	messages := []Message{
		{Role: "user", Content: prompt},
	}

	// Create streaming request
	request := chatRequest{
		Model:       c.modelName,
		Messages:    messages,
		Temperature: 0.7,
		MaxTokens:   c.maxTokens,
		Stream:      true, // Important: enable streaming
	}

	// Prepare the request
	url := c.baseURL + "/v1/chat/completions"
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vLLM API returned non-200 status: %d - %s for model %s",
			resp.StatusCode, string(body), c.modelName)
	}

	// Set up the stream channel
	streamCh := make(chan *StreamParcel)

	// Process the stream in a separate goroutine
	go func() {
		defer close(streamCh)
		defer resp.Body.Close()

		// Create a scanner to read the response line by line
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// SSE format starts with "data: "
			if strings.HasPrefix(line, "data: ") {
				jsonData := strings.TrimPrefix(line, "data: ")

				// Skip [DONE] messages
				if jsonData == "[DONE]" {
					continue
				}

				var chunk chatStreamResponse
				if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
					select {
					case streamCh <- &StreamParcel{
						Error: fmt.Errorf("failed to decode SSE data: %w, raw: %s", err, jsonData),
					}:
					case <-ctx.Done():
						return
					}
					continue
				}

				// Handle error chunks
				if chunk.Error != nil {
					select {
					case streamCh <- &StreamParcel{
						Error: fmt.Errorf("vLLM stream error: %s", *chunk.Error),
					}:
					case <-ctx.Done():
						return
					}
					return
				}

				// Process the chunk
				if len(chunk.Choices) > 0 {
					delta := chunk.Choices[0].Delta
					if delta.Content != "" {
						select {
						case streamCh <- &StreamParcel{Data: delta.Content}:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			select {
			case streamCh <- &StreamParcel{
				Error: fmt.Errorf("stream scanning error: %w", err),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return streamCh, nil
}

// chatStreamResponse represents a single chunk in the streaming response
type chatStreamResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`
		Index        int    `json:"index"`
		FinishReason string `json:"finish_reason,omitempty"`
	} `json:"choices,omitempty"`
	Error *string `json:"error,omitempty"`
}

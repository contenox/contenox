package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/contenox/runtime/internal/modelrepo"
)

type OpenAIStreamClient struct {
	openAIClient
}

type openAIChatStreamResponseChunk struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int    `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role      string `json:"role,omitempty"`
			Content   string `json:"content,omitempty"`
			ToolCalls []struct {
				ID       string `json:"id,omitempty"`
				Type     string `json:"type,omitempty"`
				Function struct {
					Name      string `json:"name,omitempty"`
					Arguments string `json:"arguments,omitempty"`
				} `json:"function,omitempty"`
			} `json:"tool_calls,omitempty"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

func (c *OpenAIStreamClient) Stream(ctx context.Context, prompt string, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	messages := []modelrepo.Message{{Role: "user", Content: prompt}}

	// buildOpenAIRequest now returns (request, nameMap); we only need the request here.
	request, _ := buildOpenAIRequest(c.modelName, messages, args)
	request.Stream = true

	url := c.baseURL + "/chat/completions"
	reqBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("OpenAI API returned non-200 status: %d - %s for model %s",
			resp.StatusCode, string(body), c.modelName)
	}

	streamCh := make(chan *modelrepo.StreamParcel)

	go func() {
		defer close(streamCh)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			jsonData := strings.TrimPrefix(line, "data: ")
			if jsonData == "[DONE]" {
				continue
			}

			var chunk openAIChatStreamResponseChunk
			if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
				select {
				case streamCh <- &modelrepo.StreamParcel{
					Error: fmt.Errorf("failed to decode SSE data: %w, raw: %s", err, jsonData),
				}:
				case <-ctx.Done():
					return
				}
				continue
			}

			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta
				if delta.Content != "" {
					select {
					case streamCh <- &modelrepo.StreamParcel{Data: delta.Content}:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			select {
			case streamCh <- &modelrepo.StreamParcel{
				Error: fmt.Errorf("stream scanning error: %w", err),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()

	return streamCh, nil
}

var _ modelrepo.LLMStreamClient = (*OpenAIStreamClient)(nil)

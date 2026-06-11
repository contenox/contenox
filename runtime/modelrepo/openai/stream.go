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

	"github.com/contenox/runtime/runtime/modelrepo"
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
			Role             string `json:"role,omitempty"`
			Content          string `json:"content,omitempty"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
			ToolCalls        []struct {
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

func (c *OpenAIStreamClient) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	// Start tracking the operation
	reportErr, reportChange, end := c.tracker.Start(ctx, "stream", "openai", "model", c.modelName)
	// Note: We don't defer end() here because the stream is asynchronous

	streamCh := make(chan *modelrepo.StreamParcel)
	usesResponses := openAIUsesResponsesEndpoint(c.modelName)
	endpoint := "/chat/completions"
	var requestBody []byte
	var responseNameMap map[string]string
	var err error

	if usesResponses {
		var req openAIResponsesRequest
		req, responseNameMap = buildOpenAIResponsesRequestWithCapabilities(c.modelName, messages, args, c.supportsThink)
		// We intentionally avoid SSE parsing for the responses path while keeping this
		// call functional for GPT-5 models.
		req.Stream = false
		requestBody, err = json.Marshal(req)
		endpoint = "/responses"
	} else {
		var req openAIChatRequest
		req, _ = buildOpenAIRequestWithCapabilities(c.modelName, messages, args, c.supportsThink)
		req.Stream = true
		requestBody, err = json.Marshal(req)
	}
	if err != nil {
		end()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+endpoint, bytes.NewBuffer(requestBody))
	if err != nil {
		end()
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		err = fmt.Errorf("HTTP request failed for model %s: %w", c.modelName, err)
		reportErr(err)
		end()
		return nil, err
	}

	// Check response status
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		err = fmt.Errorf("OpenAI API returned non-200 status: %d - %s for model %s",
			resp.StatusCode, string(body), c.modelName)
		reportErr(err)
		end()
		return nil, err
	}

	go func() {
		defer close(streamCh)
		defer resp.Body.Close()
		defer end() // End tracking when the stream completes

		if usesResponses {
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				err = fmt.Errorf("failed to read response body: %w", readErr)
				reportErr(err)
				select {
				case streamCh <- &modelrepo.StreamParcel{Error: err}:
				case <-ctx.Done():
				}
				return
			}

			result, parseErr := parseOpenAIResponsesResponse(responseNameMap, body)
			if parseErr != nil {
				reportErr(parseErr)
				select {
				case streamCh <- &modelrepo.StreamParcel{Error: parseErr}:
				case <-ctx.Done():
				}
				return
			}

			if result.Message.Content == "" && result.Message.Thinking == "" {
				return
			}
			select {
			case streamCh <- &modelrepo.StreamParcel{
				Data:     result.Message.Content,
				Thinking: result.Message.Thinking,
			}:
			case <-ctx.Done():
				return
			}

			reportChange("stream_completed", map[string]any{
				"path":         "responses",
				"content_len":  len(result.Message.Content),
				"thinking_len": len(result.Message.Thinking),
			})
			return
		}

		// Create a scanner to read the response line by line
		scanner := bufio.NewScanner(resp.Body)
		var chunkCount int
		var totalContent strings.Builder

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

				// Process the chunk
				if len(chunk.Choices) > 0 {
					delta := chunk.Choices[0].Delta
					if delta.Content != "" || delta.ReasoningContent != "" {
						if delta.Content != "" {
							chunkCount++
							totalContent.WriteString(delta.Content)
						}
						select {
						case streamCh <- &modelrepo.StreamParcel{
							Data:     delta.Content,
							Thinking: delta.ReasoningContent,
						}:
						case <-ctx.Done():
							return
						}
					}
				}
			}
		}

		if err := scanner.Err(); err != nil && err != io.EOF {
			err = fmt.Errorf("stream scanning error: %w", err)
			reportErr(err)
			select {
			case streamCh <- &modelrepo.StreamParcel{
				Error: err,
			}:
			case <-ctx.Done():
				return
			}
		}

		reportChange("stream_completed", map[string]any{
			"chunk_count":     chunkCount,
			"total_length":    totalContent.Len(),
			"content_preview": truncateString(totalContent.String(), 100),
		})
	}()

	return streamCh, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

var _ modelrepo.LLMStreamClient = (*OpenAIStreamClient)(nil)

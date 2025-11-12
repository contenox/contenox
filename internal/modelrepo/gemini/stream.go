package gemini

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

type GeminiStreamClient struct {
	geminiClient
}

func (c *GeminiStreamClient) Stream(ctx context.Context, prompt string, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	parcels := make(chan *modelrepo.StreamParcel)

	messages := []modelrepo.Message{
		{Role: "user", Content: prompt},
	}
	request := buildGeminiRequest(c.modelName, messages, nil, args)

	go func() {
		defer close(parcels)

		body, err := json.Marshal(request)
		if err != nil {
			parcels <- &modelrepo.StreamParcel{Error: fmt.Errorf("failed to marshal stream request: %w", err)}
			return
		}

		endpoint := fmt.Sprintf("/v1beta/models/%s:streamGenerateContent?alt=sse", c.modelName)
		fullURL := fmt.Sprintf("%s%s", c.baseURL, endpoint)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, bytes.NewBuffer(body))
		if err != nil {
			parcels <- &modelrepo.StreamParcel{Error: fmt.Errorf("failed to create stream request: %w", err)}
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Goog-Api-Key", c.apiKey)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Cache-Control", "no-cache")
		req.Header.Set("Connection", "keep-alive")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			parcels <- &modelrepo.StreamParcel{Error: fmt.Errorf("HTTP stream request failed for model %s: %w", c.modelName, err)}
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			parcels <- &modelrepo.StreamParcel{Error: fmt.Errorf("gemini API returned non-200 status for stream: %d, body: %s", resp.StatusCode, string(b))}
			return
		}

		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			jsonData := strings.TrimPrefix(line, "data: ")
			if jsonData == "" || jsonData == "[DONE]" {
				continue
			}

			var chunk geminiGenerateContentResponse
			if err := json.Unmarshal([]byte(jsonData), &chunk); err != nil {
				// ignore malformed frame; continue
				continue
			}

			if chunk.PromptFeedback.BlockReason != "" {
				parcels <- &modelrepo.StreamParcel{Error: fmt.Errorf("stream blocked by API for reason: %s", chunk.PromptFeedback.BlockReason)}
				return
			}
			if len(chunk.Candidates) > 0 && len(chunk.Candidates[0].Content.Parts) > 0 {
				text := chunk.Candidates[0].Content.Parts[0].Text
				if text != "" {
					select {
					case parcels <- &modelrepo.StreamParcel{Data: text}:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		if err := sc.Err(); err != nil {
			select {
			case parcels <- &modelrepo.StreamParcel{Error: fmt.Errorf("error reading from stream: %w", err)}:
			case <-ctx.Done():
			}
		}
	}()

	return parcels, nil
}

var _ modelrepo.LLMStreamClient = (*GeminiStreamClient)(nil)

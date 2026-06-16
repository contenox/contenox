package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/modeld"
	"github.com/contenox/runtime/modeld/codec/chatcompletions"
)

type orClient struct {
	baseURL         string
	apiKey          string
	modelName       string
	maxOutputTokens int
	httpClient      *http.Client
	tracker         libtracker.ActivityTracker
}

func (c *orClient) url(path string) string {
	return strings.TrimRight(c.baseURL, "/") + path
}

func (c *orClient) post(ctx context.Context, path string, request any) ([]byte, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: request failed for model %s: %w", c.modelName, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter API error: %d - %s (model=%s)", resp.StatusCode, strings.TrimSpace(string(body)), c.modelName)
	}
	return body, nil
}

func (c *orClient) openStream(ctx context.Context, path string, request any) (*http.Response, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("openrouter: marshal stream request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), bytes.NewBuffer(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openrouter: stream request failed for model %s: %w", c.modelName, err)
	}
	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openrouter API stream error: %d - %s", resp.StatusCode, strings.TrimSpace(string(bd)))
	}
	return resp, nil
}

func chatConfigFromArgs(args []modeld.ChatArgument) *modeld.ChatConfig {
	cfg := &modeld.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}
	return cfg
}

// orChatClient implements modeld.LLMChatClient.
type orChatClient struct{ orClient }

func (c *orChatClient) Chat(ctx context.Context, messages []modeld.Message, args ...modeld.ChatArgument) (modeld.ChatResult, error) {
	reportErr, reportChange, end := c.tracker.Start(ctx, "chat", "openrouter", "model", c.modelName)
	defer end()

	cfg := chatConfigFromArgs(args)
	req, nameMap := chatcompletions.Build(c.modelName, messages, cfg)
	req.MaxTokens = modeld.ClampMaxOutputTokensPtr(req.MaxTokens, c.maxOutputTokens)
	raw, err := c.post(ctx, "/chat/completions", req)
	if err != nil {
		reportErr(err)
		return modeld.ChatResult{}, err
	}
	res, err := chatcompletions.DecodeResponse(raw, nameMap)
	if err != nil {
		reportErr(err)
		return modeld.ChatResult{}, err
	}
	reportChange("chat_completed", res)
	return res, nil
}

// orStreamClient implements modeld.LLMStreamClient.
type orStreamClient struct{ orClient }

func (c *orStreamClient) Stream(ctx context.Context, messages []modeld.Message, args ...modeld.ChatArgument) (<-chan *modeld.StreamParcel, error) {
	cfg := chatConfigFromArgs(args)
	req, nameMap := chatcompletions.Build(c.modelName, messages, cfg)
	req.MaxTokens = modeld.ClampMaxOutputTokensPtr(req.MaxTokens, c.maxOutputTokens)
	req.Stream = true
	dec := chatcompletions.NewStreamDecoder(nameMap)

	reportErr, reportChange, end := c.tracker.Start(ctx, "stream", "openrouter", "model", c.modelName)
	resp, err := c.openStream(ctx, "/chat/completions", req)
	if err != nil {
		reportErr(err)
		end()
		return nil, err
	}

	parcels := make(chan *modeld.StreamParcel)
	go func() {
		defer close(parcels)
		defer resp.Body.Close()
		defer end()
		sc := bufio.NewScanner(resp.Body)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		var chunkCount int
		for sc.Scan() {
			line := sc.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" {
				continue
			}
			p, derr := dec.DecodeLine([]byte(payload))
			if derr != nil {
				continue
			}
			if p != nil {
				chunkCount++
				select {
				case parcels <- p:
				case <-ctx.Done():
					return
				}
			}
		}
		if err := sc.Err(); err != nil && err != io.EOF {
			reportErr(err)
			select {
			case parcels <- &modeld.StreamParcel{Error: fmt.Errorf("openrouter: stream read: %w", err)}:
			case <-ctx.Done():
			}
			return
		}
		reportChange("stream_completed", map[string]any{"chunk_count": chunkCount})
	}()
	return parcels, nil
}

// orPromptClient implements modeld.LLMPromptExecClient.
type orPromptClient struct{ orClient }

func (c *orPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	msgs := []modeld.Message{{Role: "user", Content: prompt}}
	if s := strings.TrimSpace(systemInstruction); s != "" {
		msgs = append([]modeld.Message{{Role: "system", Content: s}}, msgs...)
	}
	chat := &orChatClient{orClient: c.orClient}
	res, err := chat.Chat(ctx, msgs, modeld.WithTemperature(float64(temperature)))
	if err != nil {
		return "", err
	}
	return res.Message.Content, nil
}

var (
	_ modeld.LLMChatClient       = (*orChatClient)(nil)
	_ modeld.LLMStreamClient     = (*orStreamClient)(nil)
	_ modeld.LLMPromptExecClient = (*orPromptClient)(nil)
)

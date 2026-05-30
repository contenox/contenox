package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/contenox/agent/libtracker"
	"github.com/contenox/agent/runtime/modelrepo"
	msgcodec "github.com/contenox/agent/runtime/modelrepo/codec/messages"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	// anthropicAPIVersion is the direct-API version header value (Vertex uses a
	// different value placed in the body; that path lives in the vertex package).
	anthropicAPIVersion = "2023-06-01"
)

// anthropicClient is the shared transport for the direct Anthropic API.
type anthropicClient struct {
	baseURL    string
	apiKey     string
	modelName  string
	httpClient *http.Client
	tracker    libtracker.ActivityTracker
}

func chatConfigFromArgs(args []modelrepo.ChatArgument) *modelrepo.ChatConfig {
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}
	return cfg
}

func (c *anthropicClient) newRequest(ctx context.Context, path string, body []byte, stream bool) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	return req, nil
}

func (c *anthropicClient) post(ctx context.Context, path string, request any) ([]byte, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}
	req, err := c.newRequest(ctx, path, b, false)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed for model %s: %w", c.modelName, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic API error: %d - %s (model=%s)", resp.StatusCode, strings.TrimSpace(string(body)), c.modelName)
	}
	return body, nil
}

func (c *anthropicClient) openStream(ctx context.Context, path string, request any) (*http.Response, error) {
	b, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal stream request: %w", err)
	}
	req, err := c.newRequest(ctx, path, b, true)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("anthropic: stream request failed for model %s: %w", c.modelName, err)
	}
	if resp.StatusCode != http.StatusOK {
		bd, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic API stream error: %d - %s", resp.StatusCode, strings.TrimSpace(string(bd)))
	}
	return resp, nil
}

type anthropicChatClient struct{ anthropicClient }

func (c *anthropicChatClient) Chat(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	reportErr, reportChange, end := c.tracker.Start(ctx, "chat", "anthropic", "model", c.modelName)
	defer end()

	cfg := chatConfigFromArgs(args)
	req := msgcodec.Build(messages, cfg)
	req.Model = c.modelName // direct: model in body, version via header

	raw, err := c.post(ctx, "/v1/messages", req)
	if err != nil {
		reportErr(err)
		return modelrepo.ChatResult{}, err
	}
	res, err := msgcodec.DecodeResponse(raw)
	if err != nil {
		reportErr(err)
		return modelrepo.ChatResult{}, err
	}
	reportChange("chat_completed", res)
	return res, nil
}

type anthropicStreamClient struct{ anthropicClient }

func (c *anthropicStreamClient) Stream(ctx context.Context, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := chatConfigFromArgs(args)
	req := msgcodec.Build(messages, cfg)
	req.Model = c.modelName
	req.Stream = true
	dec := msgcodec.NewStreamDecoder()

	reportErr, reportChange, end := c.tracker.Start(ctx, "stream", "anthropic", "model", c.modelName)
	resp, err := c.openStream(ctx, "/v1/messages", req)
	if err != nil {
		reportErr(err)
		end()
		return nil, err
	}

	parcels := make(chan *modelrepo.StreamParcel)
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
				continue // skip SSE `event:` lines and blanks
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
			case parcels <- &modelrepo.StreamParcel{Error: fmt.Errorf("anthropic: stream read: %w", err)}:
			case <-ctx.Done():
			}
			return
		}
		reportChange("stream_completed", map[string]any{"chunk_count": chunkCount})
	}()
	return parcels, nil
}

type anthropicPromptClient struct{ anthropicClient }

func (c *anthropicPromptClient) Prompt(ctx context.Context, systemInstruction string, temperature float32, prompt string) (string, error) {
	msgs := []modelrepo.Message{{Role: "user", Content: prompt}}
	if s := strings.TrimSpace(systemInstruction); s != "" {
		msgs = append([]modelrepo.Message{{Role: "system", Content: s}}, msgs...)
	}
	chat := &anthropicChatClient{anthropicClient: c.anthropicClient}
	res, err := chat.Chat(ctx, msgs, modelrepo.WithTemperature(float64(temperature)))
	if err != nil {
		return "", err
	}
	return res.Message.Content, nil
}

var (
	_ modelrepo.LLMChatClient       = (*anthropicChatClient)(nil)
	_ modelrepo.LLMStreamClient     = (*anthropicStreamClient)(nil)
	_ modelrepo.LLMPromptExecClient = (*anthropicPromptClient)(nil)
)

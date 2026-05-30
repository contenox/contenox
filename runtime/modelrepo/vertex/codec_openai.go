package vertex

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/contenox/agent/runtime/modelrepo"
	"github.com/contenox/agent/runtime/modelrepo/codec/chatcompletions"
)

// chatConfigFromArgs collapses ChatArguments into a ChatConfig.
func chatConfigFromArgs(args []modelrepo.ChatArgument) *modelrepo.ChatConfig {
	cfg := &modelrepo.ChatConfig{}
	for _, a := range args {
		a.Apply(cfg)
	}
	return cfg
}

// sseLineDecoder is satisfied by both codec stream decoders (chatcompletions
// and messages); it turns one SSE `data:` payload into an optional parcel.
type sseLineDecoder interface {
	DecodeLine(payload []byte) (*modelrepo.StreamParcel, error)
}

// streamSSE opens an SSE stream to endpoint and feeds each `data:` payload to
// dec, emitting the parcels it returns. Tool-call fragments accumulated by the
// decoder are not surfaced on the channel (parity with the Gemini path); the
// channel carries visible text/thinking deltas only.
func (c *vertexStreamClient) streamSSE(ctx context.Context, endpoint string, request any, dec sseLineDecoder) (<-chan *modelrepo.StreamParcel, error) {
	reportErr, reportChange, end := c.tracker.Start(
		ctx, "http_stream", "vertex",
		"model", c.modelName, "publisher", c.publisher, "endpoint", endpoint,
	)

	resp, err := c.openStream(ctx, endpoint, request)
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
				continue // skip blank lines and SSE `event:` lines
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
			err = fmt.Errorf("error reading from stream: %w", err)
			reportErr(err)
			select {
			case parcels <- &modelrepo.StreamParcel{Error: err}:
			case <-ctx.Done():
			}
			return
		}
		reportChange("stream_completed", map[string]any{"chunk_count": chunkCount})
	}()

	return parcels, nil
}

// chatOpenAICompat handles Mistral (rawPredict) and Meta/open-model MaaS
// (openapi chat/completions) via the shared chat-completions codec.
func (c *vertexChatClient) chatOpenAICompat(ctx context.Context, messages []modelrepo.Message, args []modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	reportErr, reportChange, end := c.tracker.Start(ctx, "chat", "vertex", "model", c.modelName, "publisher", c.publisher)
	defer end()

	cfg := chatConfigFromArgs(args)
	req, nameMap := chatcompletions.Build(c.openAICompatBodyModel(), messages, cfg)

	raw, err := c.postJSON(ctx, c.openAICompatURL(false), req)
	if err != nil {
		reportErr(err)
		return modelrepo.ChatResult{}, err
	}
	res, err := chatcompletions.DecodeResponse(raw, nameMap)
	if err != nil {
		reportErr(err)
		return modelrepo.ChatResult{}, err
	}
	reportChange("chat_completed", res)
	return res, nil
}

func (c *vertexStreamClient) streamOpenAICompat(ctx context.Context, messages []modelrepo.Message, args []modelrepo.ChatArgument) (<-chan *modelrepo.StreamParcel, error) {
	cfg := chatConfigFromArgs(args)
	req, nameMap := chatcompletions.Build(c.openAICompatBodyModel(), messages, cfg)
	req.Stream = true
	return c.streamSSE(ctx, c.openAICompatURL(true), req, chatcompletions.NewStreamDecoder(nameMap))
}

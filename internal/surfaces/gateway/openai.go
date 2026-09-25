package gateway

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/modelrepo/codec/chatcompletions"
	"github.com/google/uuid"
)

type openAIChatRequest struct {
	chatcompletions.Request
}

type openAIErrorWriter struct {
	http.ResponseWriter
	status int
}

func (w *openAIErrorWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *openAIErrorWriter) Write(data []byte) (int, error) {
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return 0, err
	}
	err := json.NewEncoder(w.ResponseWriter).Encode(openAIErrorBody(w.status, body.Error))
	if err != nil {
		return 0, err
	}
	return len(data), nil
}

func openAIErrorBody(status int, message string) any {
	kind := "invalid_request_error"
	switch {
	case status == http.StatusUnauthorized:
		kind = "authentication_error"
	case status == http.StatusForbidden:
		kind = "permission_error"
	case status == http.StatusTooManyRequests:
		kind = "rate_limit_error"
	case status >= 500:
		kind = "server_error"
	}
	return map[string]any{"error": map[string]any{"message": message, "type": kind, "param": nil, "code": nil}}
}

func openAIJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func openAIDecode(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 32<<20)
	d := json.NewDecoder(r.Body)
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("request must contain one JSON object")
	}
	return nil
}

// AddOpenAIProxyRoutes mounts the supported OpenAI-compatible inference endpoints.
func (s *service) AddOpenAIProxyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/models", s.handleOpenAIModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleOpenAIChat)
	mux.HandleFunc("POST /v1/embeddings", s.handleOpenAIEmbeddings)
}

// handleOpenAIModels lists observed models permitted by the caller's key.
func (s *service) handleOpenAIModels(w http.ResponseWriter, r *http.Request) {
	claims, _, err := s.authenticate(r)
	if err != nil {
		openAIJSON(w, 401, openAIErrorBody(401, "unauthorized"))
		return
	}
	seen := map[string]bool{}
	names := []string{}
	for _, state := range s.runtime.Get(r.Context()) {
		for _, model := range state.PulledModels {
			name := strings.TrimSpace(model.Model)
			if name == "" {
				name = strings.TrimSpace(model.Name)
			}
			if name != "" && !seen[name] && s.isModelAllowed(claims, name) {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	sort.Strings(names)
	models := make([]any, 0, len(names))
	for _, name := range names {
		models = append(models, map[string]any{"id": name, "object": "model", "created": 0, "owned_by": "contenox"})
	}
	openAIJSON(w, 200, map[string]any{"object": "list", "data": models})
}

func (req openAIChatRequest) convert() ([]modelrepo.Message, []modelrepo.ChatArgument, error) {
	if strings.TrimSpace(req.Model) == "" || len(req.Messages) == 0 {
		return nil, nil, fmt.Errorf("model and messages are required")
	}
	if req.N != nil && *req.N != 1 {
		return nil, nil, fmt.Errorf("only n=1 is supported")
	}
	if (req.Logprobs != nil && *req.Logprobs) || req.TopLogprobs != nil || len(req.LogitBias) > 0 || req.Store || req.ParallelToolCalls != nil {
		return nil, nil, fmt.Errorf("logprobs, logit_bias, store and parallel_tool_calls are not supported")
	}
	if req.FrequencyPenalty != nil && *req.FrequencyPenalty != 0 || req.PresencePenalty != nil && *req.PresencePenalty != 0 {
		return nil, nil, fmt.Errorf("frequency_penalty and presence_penalty are not supported")
	}
	if len(req.Stop) > 0 && string(req.Stop) != "null" {
		return nil, nil, fmt.Errorf("stop is not supported")
	}
	if req.Format != nil && req.Format.Type != "text" {
		return nil, nil, fmt.Errorf("only text response_format is supported")
	}
	for _, modality := range req.Modalities {
		if modality != "text" {
			return nil, nil, fmt.Errorf("only text output modality is supported")
		}
	}
	tools := make([]modelrepo.Tool, 0, len(req.Tools))
	if req.ToolChoice != "" && req.ToolChoice != "auto" && req.ToolChoice != "none" {
		return nil, nil, fmt.Errorf("only auto and none tool_choice are supported")
	}
	if req.ToolChoice != "none" {
		for _, tool := range req.Tools {
			if tool.Type != "function" || tool.Function.Name == "" {
				return nil, nil, fmt.Errorf("tools must be named functions")
			}
			tools = append(tools, modelrepo.Tool{Type: tool.Type, Function: &modelrepo.FunctionTool{
				Name: tool.Function.Name, Description: tool.Function.Description, Parameters: tool.Function.Parameters,
			}})
		}
	}
	args := []modelrepo.ChatArgument{modelrepo.WithTools(tools...)}
	if req.Temperature != nil {
		args = append(args, modelrepo.WithTemperature(*req.Temperature))
	}
	if req.TopP != nil {
		args = append(args, modelrepo.WithTopP(*req.TopP))
	}
	if req.Seed != nil {
		args = append(args, modelrepo.WithSeed(*req.Seed))
	}
	maxTokens := req.MaxCompletionTokens
	if maxTokens == nil {
		maxTokens = req.MaxTokens
	}
	if maxTokens != nil {
		if *maxTokens <= 0 {
			return nil, nil, fmt.Errorf("token limit must be positive")
		}
		args = append(args, modelrepo.WithMaxTokens(*maxTokens))
	}
	if req.ReasoningEffort != "" {
		effort := req.ReasoningEffort
		if effort == "none" {
			effort = "off"
		}
		switch effort {
		case "off", "minimal", "low", "medium", "high", "xhigh":
		default:
			return nil, nil, fmt.Errorf("unsupported reasoning_effort")
		}
		args = append(args, modelrepo.WithThink(effort))
	}
	messages := make([]modelrepo.Message, 0, len(req.Messages))
	for _, message := range req.Messages {
		m := modelrepo.Message{Role: message.Role, ToolCallID: message.ToolCallID}
		for _, call := range message.ToolCalls {
			converted := modelrepo.ToolCall{ID: call.ID, Type: call.Type}
			converted.Function.Name = call.Function.Name
			converted.Function.Arguments = call.Function.Arguments
			m.ToolCalls = append(m.ToolCalls, converted)
		}
		if m.Role == "developer" {
			m.Role = "system"
		}
		switch m.Role {
		case "system", "user", "assistant", "tool":
		default:
			return nil, nil, fmt.Errorf("unsupported message role %q", m.Role)
		}
		if m.Role == "tool" && m.ToolCallID == "" {
			return nil, nil, fmt.Errorf("tool messages require tool_call_id")
		}
		if message.Content != nil {
			switch content := message.Content.(type) {
			case string:
				m.Content = content
			case []any:
				raw, err := json.Marshal(content)
				if err != nil {
					return nil, nil, fmt.Errorf("invalid content parts: %w", err)
				}
				var parts []chatcompletions.ContentPart
				if err := json.Unmarshal(raw, &parts); err != nil {
					return nil, nil, fmt.Errorf("content must be text or content parts")
				}
				for _, part := range parts {
					switch part.Type {
					case "text":
						m.Content += part.Text
					case "image_url":
						if part.ImageURL == nil {
							return nil, nil, fmt.Errorf("image_url is required")
						}
						header, data, ok := strings.Cut(part.ImageURL.URL, ",")
						if !ok || !strings.HasPrefix(header, "data:image/") || !strings.HasSuffix(header, ";base64") {
							return nil, nil, fmt.Errorf("images require base64 image data URLs; remote URLs are not fetched")
						}
						raw, err := base64.StdEncoding.DecodeString(data)
						if err != nil || len(raw) == 0 {
							return nil, nil, fmt.Errorf("invalid image data")
						}
						m.Images = append(m.Images, modelrepo.ImagePart{Data: raw, MimeType: strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")})
					case "input_audio":
						if part.InputAudio == nil || part.InputAudio.Format != "wav" {
							return nil, nil, fmt.Errorf("input_audio supports wav only")
						}
						audio, err := toModelAudio([]string{part.InputAudio.Data})
						if err != nil {
							return nil, nil, err
						}
						m.Audio = append(m.Audio, audio...)
					default:
						return nil, nil, fmt.Errorf("unsupported content part %q", part.Type)
					}
				}
			default:
				return nil, nil, fmt.Errorf("content must be text or content parts")
			}
		}
		messages = append(messages, m)
	}
	return messages, args, nil
}

// handleOpenAIChat serves Chat Completions using the shared metered repository.
func (s *service) handleOpenAIChat(w http.ResponseWriter, r *http.Request) {
	var input openAIChatRequest
	if err := openAIDecode(w, r, &input); err != nil {
		openAIJSON(w, 400, openAIErrorBody(400, err.Error()))
		return
	}
	messages, args, err := input.convert()
	if err != nil {
		openAIJSON(w, 400, openAIErrorBody(400, err.Error()))
		return
	}
	who, err := s.callerForRequest(r)
	if err != nil {
		openAIJSON(w, 401, openAIErrorBody(401, "unauthorized"))
		return
	}
	errorWriter := &openAIErrorWriter{ResponseWriter: w}
	if !s.admitRequest(errorWriter, r, who, input.Model) {
		return
	}
	session := sessionKeyFor(who.key, who.claims, input.Model, input.Session, messages)
	req, health := s.chatRequest(input.Model, session, messages)
	repo := &Repo{svc: s, bearer: bearerFrom(r)}
	id, created := "chatcmpl-"+uuid.NewString(), time.Now().Unix()
	if input.Stream != nil && *input.Stream {
		s.streamOpenAI(w, r, repo, req, health, input, messages, args, id, created)
		return
	}
	result, meta, err := repo.Chat(r.Context(), req, messages, args...)
	health.record()
	if err != nil {
		s.handleProviderTurnError(errorWriter, r, "chat", err)
		return
	}
	s.recordBackendSuccess(meta.BackendID)
	message := map[string]any{"role": "assistant", "content": result.Message.Content}
	if result.Message.Thinking != "" {
		message["reasoning_content"] = result.Message.Thinking
	}
	calls := result.ToolCalls
	if len(calls) == 0 {
		calls = result.Message.ToolCalls
	}
	if len(calls) > 0 {
		message["tool_calls"] = openAIToolCalls(calls)
	}
	openAIJSON(w, 200, map[string]any{"id": id, "object": "chat.completion", "created": created, "model": input.Model,
		"choices": []any{map[string]any{"index": 0, "message": message, "finish_reason": openAIFinish(result.FinishReason, len(calls) > 0), "logprobs": nil}}, "usage": openAIUsage(result.Usage)})
}

func openAIToolCalls(calls []modelrepo.ToolCall) []chatcompletions.ToolCall {
	wire := make([]chatcompletions.ToolCall, 0, len(calls))
	for _, call := range calls {
		wire = append(wire, chatcompletions.ToolCall{
			ID: call.ID, Type: call.Type,
			Function: chatcompletions.ToolFunction{Name: call.Function.Name, Arguments: call.Function.Arguments},
		})
	}
	return wire
}

func openAIFinish(reason string, tools bool) string {
	if tools {
		return "tool_calls"
	}
	switch reason {
	case "length", "max_tokens", "max_output_tokens":
		return "length"
	case "tool_calls", "tool_use":
		return "tool_calls"
	case "content_filter", "refusal":
		return "content_filter"
	default:
		return "stop"
	}
}

func openAIUsage(usage *modelrepo.TokenUsage) any {
	if usage == nil {
		return nil
	}
	return map[string]any{"prompt_tokens": usage.PromptTokens, "completion_tokens": usage.CompletionTokens, "total_tokens": usage.PromptTokens + usage.CompletionTokens,
		"prompt_tokens_details": map[string]int{"cached_tokens": usage.CacheReadTokens}, "completion_tokens_details": map[string]int{"reasoning_tokens": usage.ThinkingTokens}}
}

func (s *service) streamOpenAI(w http.ResponseWriter, r *http.Request, repo *Repo, req llmrepo.Request, health *healthTracker, input openAIChatRequest, messages []modelrepo.Message, args []modelrepo.ChatArgument, id string, created int64) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	stream, meta, err := repo.Stream(ctx, req, messages, args...)
	if err != nil {
		health.record()
		s.handleProviderTurnError(&openAIErrorWriter{ResponseWriter: w}, r, "stream_start", err)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	write := func(value any) error {
		data, err := json.Marshal(value)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return err
		}
		return http.NewResponseController(w).Flush()
	}
	chunk := func(delta any, finish any, usage any, empty bool) any {
		choices := []any{}
		if !empty {
			choices = append(choices, map[string]any{"index": 0, "delta": delta, "finish_reason": finish, "logprobs": nil})
		}
		return map[string]any{"id": id, "object": "chat.completion.chunk", "created": created, "model": input.Model, "choices": choices, "usage": usage}
	}
	if write(chunk(map[string]string{"role": "assistant", "content": ""}, nil, nil, false)) != nil {
		return
	}
	var usage modelrepo.TokenUsage
	haveUsage, tools := false, false
	finish := ""
	for parcel := range stream {
		if parcel == nil {
			continue
		}
		if parcel.Error != nil {
			health.record()
			s.recordTurnFailure(ctx, "stream_upstream", parcel.Error)
			_ = write(openAIErrorBody(503, sanitizeTurnError(parcel.Error).Error()))
			return
		}
		if parcel.Usage != nil {
			usage.Merge(parcel.Usage)
			haveUsage = true
		}
		if parcel.Terminal != nil {
			finish = parcel.Terminal.FinishReason
			if parcel.Terminal.Usage != nil {
				usage.Merge(parcel.Terminal.Usage)
				haveUsage = true
			}
		}
		delta := map[string]any{}
		if parcel.Data != "" {
			delta["content"] = parcel.Data
		}
		if parcel.Thinking != "" {
			delta["reasoning_content"] = parcel.Thinking
		}
		if call := parcel.ToolCall; call != nil {
			tools = true
			function := map[string]any{"arguments": call.ArgsFragment}
			if call.Name != "" {
				function["name"] = call.Name
			}
			tool := map[string]any{"index": call.Index, "function": function}
			if call.ID != "" {
				tool["id"] = call.ID
			}
			if call.Type != "" {
				tool["type"] = call.Type
			}
			delta["tool_calls"] = []any{tool}
		}
		if len(delta) > 0 && write(chunk(delta, nil, nil, false)) != nil {
			return
		}
	}
	if ctx.Err() != nil {
		return
	}
	s.recordBackendSuccess(meta.BackendID)
	if write(chunk(map[string]any{}, openAIFinish(finish, tools), nil, false)) != nil {
		return
	}
	if input.StreamOptions != nil && input.StreamOptions.IncludeUsage {
		var report any
		if haveUsage {
			report = openAIUsage(&usage)
		}
		if write(chunk(nil, nil, report, true)) != nil {
			return
		}
	}
	_, _ = io.WriteString(w, "data: [DONE]\n\n")
	_ = http.NewResponseController(w).Flush()
}

// handleOpenAIEmbeddings adapts string inputs and float/base64 vectors to the shared embedding path.
func (s *service) handleOpenAIEmbeddings(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Model          string `json:"model"`
		Input          any    `json:"input"`
		Dimensions     int    `json:"dimensions"`
		EncodingFormat string `json:"encoding_format"`
	}
	if err := openAIDecode(w, r, &input); err != nil {
		openAIJSON(w, 400, openAIErrorBody(400, err.Error()))
		return
	}
	if input.Model == "" || input.Dimensions != 0 || input.EncodingFormat != "" && input.EncodingFormat != "float" && input.EncodingFormat != "base64" {
		openAIJSON(w, 400, openAIErrorBody(400, "model is required; dimensions is unsupported; encoding_format must be float or base64"))
		return
	}
	inputs, err := embedInputs(input.Input)
	if err != nil {
		openAIJSON(w, 400, openAIErrorBody(400, err.Error()))
		return
	}
	who, err := s.callerForRequest(r)
	if err != nil {
		openAIJSON(w, 401, openAIErrorBody(401, "unauthorized"))
		return
	}
	errorWriter := &openAIErrorWriter{ResponseWriter: w}
	if !s.admitRequest(errorWriter, r, who, input.Model) {
		return
	}
	repo := &Repo{svc: s, bearer: bearerFrom(r)}
	data := make([]any, 0, len(inputs))
	promptTokens := 0
	for index, text := range inputs {
		vector, meta, err := repo.Embed(r.Context(), llmrepo.EmbedRequest{ModelName: input.Model, BackendHealth: s.backendHealth()}, text)
		if err != nil {
			s.handleProviderTurnError(errorWriter, r, "embed", err)
			return
		}
		s.recordBackendSuccess(meta.BackendID)
		var embedding any = toFloat32(vector)
		if input.EncodingFormat == "base64" {
			bytes := make([]byte, len(vector)*4)
			for i, value := range vector {
				binary.LittleEndian.PutUint32(bytes[i*4:], math.Float32bits(float32(value)))
			}
			embedding = base64.StdEncoding.EncodeToString(bytes)
		}
		data = append(data, map[string]any{"object": "embedding", "index": index, "embedding": embedding})
		if count, err := s.tokenizer.CountTokens(r.Context(), input.Model, text); err == nil {
			promptTokens += count
		}
	}
	openAIJSON(w, 200, map[string]any{"object": "list", "model": input.Model, "data": data, "usage": map[string]int{"prompt_tokens": promptTokens, "total_tokens": promptTokens}})
}

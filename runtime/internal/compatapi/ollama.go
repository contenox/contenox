package compatapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/contenox/runtime/runtime/agentservice"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/taskengine"
)

// AddOllamaRoutes registers the Ollama-native compat aliases (/api/chat,
// /api/generate, ...) for clients that point an Ollama base URL at the bare host.
//
// openapi:exclude mounted on the serve ROOT mux (contenoxcli/serve_cmd.go); under the spec's servers:/api these would document /api/api/* URLs that do not exist
func AddOllamaRoutes(mux *http.ServeMux, deps CompatDeps) {
	h := &ollamaHandler{deps: deps}

	mux.HandleFunc("GET /api/tags", h.tags)
	mux.HandleFunc("GET /api/ps", h.ps)
	mux.HandleFunc("POST /api/show", h.show)
	mux.HandleFunc("POST /api/chat", h.chat)
	mux.HandleFunc("POST /api/generate", h.generate)
}

type ollamaHandler struct {
	deps CompatDeps
}

type ollamaTagsResponse struct {
	Models []ollamaModel `json:"models"`
}

type ollamaModel struct {
	Name       string             `json:"name"`
	Model      string             `json:"model"`
	ModifiedAt time.Time          `json:"modified_at"`
	Size       int64              `json:"size"`
	Digest     string             `json:"digest"`
	Details    ollamaModelDetails `json:"details"`
	ExpiresAt  *time.Time         `json:"expires_at,omitempty"`
	SizeVRAM   int64              `json:"size_vram,omitempty"`
}

type ollamaModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families,omitempty"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Images are base64-encoded image attachments (Ollama's native multimodal
	// field). Request-only: responses never carry images, so omitempty keeps
	// them out of the reply.
	Images []string `json:"images,omitempty"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   *bool           `json:"stream,omitempty"`
	Options  map[string]any  `json:"options,omitempty"`
}

type ollamaChatResponse struct {
	Model           string         `json:"model"`
	CreatedAt       time.Time      `json:"created_at"`
	Message         *ollamaMessage `json:"message,omitempty"`
	Done            bool           `json:"done"`
	DoneReason      string         `json:"done_reason,omitempty"`
	PromptEvalCount int            `json:"prompt_eval_count,omitempty"`
	EvalCount       int            `json:"eval_count,omitempty"`
}

type ollamaGenerateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Suffix  string         `json:"suffix,omitempty"`
	Images  []string       `json:"images,omitempty"`
	Stream  *bool          `json:"stream,omitempty"`
	Options map[string]any `json:"options,omitempty"`
}

type ollamaGenerateResponse struct {
	Model           string    `json:"model"`
	CreatedAt       time.Time `json:"created_at"`
	Response        string    `json:"response,omitempty"`
	Done            bool      `json:"done"`
	DoneReason      string    `json:"done_reason,omitempty"`
	PromptEvalCount int       `json:"prompt_eval_count,omitempty"`
	EvalCount       int       `json:"eval_count,omitempty"`
}

type ollamaShowRequest struct {
	Model string `json:"model"`
	Name  string `json:"name,omitempty"`
}

type ollamaShowResponse struct {
	Details      ollamaModelDetails `json:"details"`
	ModelInfo    map[string]any     `json:"model_info"`
	Capabilities []string           `json:"capabilities"`
	ModifiedAt   time.Time          `json:"modified_at"`
	Parameters   string             `json:"parameters,omitempty"`
}

type ollamaModelInfo struct {
	Model         ollamaModel
	ContextLength int
	CanChat       bool
	CanEmbed      bool
	CanPrompt     bool
	CanStream     bool
	CanThink      bool
	CanVision     bool
}

func (h *ollamaHandler) tags(w http.ResponseWriter, r *http.Request) {
	if err := authorizeCompatRequest(r, h.deps, false); err != nil {
		writeOllamaError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	models, err := h.ollamaModels(r.Context(), runtimeDefaults(r.Context(), h.deps))
	if err != nil {
		writeOllamaError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ollamaTagsResponse{Models: models})
}

func (h *ollamaHandler) ps(w http.ResponseWriter, r *http.Request) {
	if err := authorizeCompatRequest(r, h.deps, false); err != nil {
		writeOllamaError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	models, err := h.ollamaModels(r.Context(), runtimeDefaults(r.Context(), h.deps))
	if err != nil {
		writeOllamaError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ollamaTagsResponse{Models: models})
}

func (h *ollamaHandler) show(w http.ResponseWriter, r *http.Request) {
	if err := authorizeCompatRequest(r, h.deps, false); err != nil {
		writeOllamaError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req ollamaShowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOllamaError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := strings.TrimSpace(req.Model)
	if name == "" {
		name = strings.TrimSpace(req.Name)
	}
	if name == "" {
		writeOllamaError(w, http.StatusBadRequest, "model is required")
		return
	}

	info, ok, err := h.ollamaModelInfo(r.Context(), runtimeDefaults(r.Context(), h.deps), name)
	if err != nil {
		writeOllamaError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !ok {
		writeOllamaError(w, http.StatusNotFound, "model not found")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ollamaShowResponse{
		Details:      info.Model.Details,
		ModelInfo:    info.modelInfo(),
		Capabilities: info.capabilities(),
		ModifiedAt:   info.Model.ModifiedAt,
		Parameters:   info.parameters(),
	})
}

func (h *ollamaHandler) chat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := authorizeCompatRequest(r, h.deps, true); err != nil {
		writeOllamaError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	defaults := runtimeDefaults(ctx, h.deps)
	if h.deps.Agent == nil || h.deps.Chains == nil {
		writeOllamaError(w, http.StatusInternalServerError, "compat dependencies are not configured")
		return
	}

	var req ollamaChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOllamaError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.Messages) == 0 {
		writeOllamaError(w, http.StatusBadRequest, "messages is required")
		return
	}
	if strings.TrimSpace(defaults.ChainRef) == "" {
		writeOllamaError(w, http.StatusInternalServerError, "no compat chain configured")
		return
	}

	chain, err := h.deps.Chains.Get(ctx, defaults.ChainRef)
	if err != nil {
		writeOllamaError(w, http.StatusBadRequest, fmt.Sprintf("chain not found: %s", defaults.ChainRef))
		return
	}

	model := resolveRequestedModel(ctx, h.deps, defaults, req.Model)
	temp, maxTokens := ollamaExecOverrides(req.Options)
	templateVars := buildTemplateVars(chain, defaults, model, maxTokens)
	chain = patchExecOverrides(chain, temp)
	sessionID, err := compatSessionID(ctx, w, r, h.deps)
	if err != nil {
		writeOllamaError(w, http.StatusInternalServerError, err.Error())
		return
	}

	messages := make([]taskengine.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		images, err := decodeOllamaImages(m.Images)
		if err != nil {
			writeOllamaError(w, http.StatusBadRequest, err.Error())
			return
		}
		messages = append(messages, taskengine.Message{
			Role:    m.Role,
			Content: m.Content,
			Images:  images,
		})
	}
	resp, err := h.deps.Agent.Prompt(ctx, agentservice.PromptRequest{
		SessionID:    sessionID,
		InputValue:   taskengine.ChatHistory{Messages: messages},
		InputType:    taskengine.DataTypeChatHistory,
		Chain:        chain,
		TemplateVars: templateVars,
	})
	if err != nil {
		writeOllamaError(w, http.StatusInternalServerError, err.Error())
		return
	}

	reply := applyStopStrings(extractChatReply(resp), ollamaStopStrings(req.Options))
	usage := extractUsage(resp)
	createdAt := time.Now().UTC()
	if ollamaStream(req.Stream) {
		setNDJSONHeaders(w)
		w.WriteHeader(http.StatusOK)
		_ = writeJSONLine(w, ollamaChatResponse{
			Model:     model,
			CreatedAt: createdAt,
			Message:   &ollamaMessage{Role: "assistant", Content: reply},
			Done:      false,
		})
		_ = writeJSONLine(w, ollamaChatResponse{
			Model:           model,
			CreatedAt:       createdAt,
			Done:            true,
			DoneReason:      ollamaDoneReason(resp),
			PromptEvalCount: usage.PromptTokens,
			EvalCount:       usage.CompletionTokens,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ollamaChatResponse{
		Model:           model,
		CreatedAt:       createdAt,
		Message:         &ollamaMessage{Role: "assistant", Content: reply},
		Done:            true,
		DoneReason:      ollamaDoneReason(resp),
		PromptEvalCount: usage.PromptTokens,
		EvalCount:       usage.CompletionTokens,
	})
}

func (h *ollamaHandler) generate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := authorizeCompatRequest(r, h.deps, true); err != nil {
		writeOllamaError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	defaults := runtimeDefaults(ctx, h.deps)
	if h.deps.Agent == nil || h.deps.Chains == nil {
		writeOllamaError(w, http.StatusInternalServerError, "compat dependencies are not configured")
		return
	}

	var req ollamaGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOllamaError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	chainRef := strings.TrimSpace(defaults.FIMChainRef)
	useFIM := chainRef != "" || req.Suffix != ""
	if chainRef == "" {
		chainRef = strings.TrimSpace(defaults.ChainRef)
	}
	if chainRef == "" {
		writeOllamaError(w, http.StatusInternalServerError, "no compat chain configured")
		return
	}

	chain, err := h.deps.Chains.Get(ctx, chainRef)
	if err != nil {
		writeOllamaError(w, http.StatusBadRequest, fmt.Sprintf("chain not found: %s", chainRef))
		return
	}

	model := resolveRequestedModel(ctx, h.deps, defaults, req.Model)
	temp, maxTokens := ollamaExecOverrides(req.Options)
	templateVars := buildTemplateVars(chain, defaults, model, maxTokens)
	chain = patchExecOverrides(chain, temp)
	sessionID, err := compatSessionID(ctx, w, r, h.deps)
	if err != nil {
		writeOllamaError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var input taskengine.ChatHistory
	if useFIM {
		input = buildFIMHistory(req.Prompt, req.Suffix)
	} else {
		images, imgErr := decodeOllamaImages(req.Images)
		if imgErr != nil {
			writeOllamaError(w, http.StatusBadRequest, imgErr.Error())
			return
		}
		input = taskengine.ChatHistory{Messages: []taskengine.Message{{Role: "user", Content: req.Prompt, Images: images}}}
	}
	resp, err := h.deps.Agent.Prompt(ctx, agentservice.PromptRequest{
		SessionID:    sessionID,
		InputValue:   input,
		InputType:    taskengine.DataTypeChatHistory,
		Chain:        chain,
		TemplateVars: templateVars,
	})
	if err != nil {
		writeOllamaError(w, http.StatusInternalServerError, err.Error())
		return
	}

	completion := extractFIMCompletion(resp)
	completion = applyStopStrings(completion, ollamaStopStrings(req.Options))
	usage := extractUsage(resp)
	createdAt := time.Now().UTC()
	if ollamaStream(req.Stream) {
		setNDJSONHeaders(w)
		w.WriteHeader(http.StatusOK)
		_ = writeJSONLine(w, ollamaGenerateResponse{
			Model:     model,
			CreatedAt: createdAt,
			Response:  completion,
			Done:      false,
		})
		_ = writeJSONLine(w, ollamaGenerateResponse{
			Model:           model,
			CreatedAt:       createdAt,
			Done:            true,
			DoneReason:      ollamaDoneReason(resp),
			PromptEvalCount: usage.PromptTokens,
			EvalCount:       usage.CompletionTokens,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ollamaGenerateResponse{
		Model:           model,
		CreatedAt:       createdAt,
		Response:        completion,
		Done:            true,
		DoneReason:      ollamaDoneReason(resp),
		PromptEvalCount: usage.PromptTokens,
		EvalCount:       usage.CompletionTokens,
	})
}

func (h *ollamaHandler) ollamaModels(ctx context.Context, defaults stateservice.RuntimeDefaults) ([]ollamaModel, error) {
	infos, err := h.ollamaModelInfos(ctx, defaults)
	if err != nil {
		return nil, err
	}
	models := make([]ollamaModel, 0, len(infos))
	for _, info := range infos {
		models = append(models, info.Model)
	}
	return models, nil
}

func (h *ollamaHandler) ollamaModelInfo(ctx context.Context, defaults stateservice.RuntimeDefaults, name string) (ollamaModelInfo, bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ollamaModelInfo{}, false, nil
	}
	infos, err := h.ollamaModelInfos(ctx, defaults)
	if err != nil {
		return ollamaModelInfo{}, false, err
	}
	for _, info := range infos {
		if name == info.Model.Name || name == info.Model.Model {
			return info, true, nil
		}
	}
	return ollamaModelInfo{}, false, nil
}

func (h *ollamaHandler) ollamaModelInfos(ctx context.Context, defaults stateservice.RuntimeDefaults) ([]ollamaModelInfo, error) {
	now := time.Now().UTC()
	byName := map[string]ollamaModelInfo{}
	defaultProvider := strings.TrimSpace(defaults.Provider)
	if h.deps.StateService != nil {
		states, err := h.deps.StateService.Get(ctx)
		if err != nil {
			return nil, err
		}
		for _, state := range states {
			if defaultProvider != "" && !stateMatchesProvider(state, defaultProvider) {
				continue
			}
			for _, pulled := range state.PulledModels {
				info := ollamaModelInfoFromPulled(pulled, now)
				if info.Model.Name == "" {
					continue
				}
				existing := byName[info.Model.Name]
				byName[info.Model.Name] = mergeOllamaModelInfo(existing, info)
			}
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	seen := map[string]struct{}{}
	models := make([]ollamaModelInfo, 0, len(names)+2)
	add := func(info ollamaModelInfo) {
		name := strings.TrimSpace(info.Model.Name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		models = append(models, info)
	}
	defaultModel := strings.TrimSpace(defaults.Model)
	if defaultModel != "" {
		defaultInfo := byName[defaultModel]
		if defaultInfo.Model.Name == "" || !defaultInfo.supportsCompletion() {
			defaultInfo = ollamaPlaceholderModelInfo(defaultModel, now)
		}
		alias := defaultInfo
		alias.Model = ollamaPlaceholderModel("default", now)
		alias.Model.Details.ParentModel = defaultModel
		add(alias)
		add(defaultInfo)
	}
	for _, name := range names {
		info := byName[name]
		if !info.supportsCompletion() {
			continue
		}
		add(info)
	}
	return models, nil
}

func ollamaModelInfoFromPulled(pulled statetype.ModelPullStatus, fallback time.Time) ollamaModelInfo {
	name := strings.TrimSpace(pulled.Model)
	if name == "" {
		name = strings.TrimSpace(pulled.Name)
	}
	if pulled.ModifiedAt.IsZero() {
		pulled.ModifiedAt = fallback
	}
	return ollamaModelInfo{
		Model: ollamaModel{
			Name:       name,
			Model:      name,
			ModifiedAt: pulled.ModifiedAt.UTC(),
			Size:       pulled.Size,
			Digest:     pulled.Digest,
			Details: ollamaModelDetails{
				ParentModel:       pulled.Details.ParentModel,
				Format:            pulled.Details.Format,
				Family:            pulled.Details.Family,
				Families:          pulled.Details.Families,
				ParameterSize:     pulled.Details.ParameterSize,
				QuantizationLevel: pulled.Details.QuantizationLevel,
			},
		},
		ContextLength: pulled.ContextLength,
		CanChat:       pulled.CanChat,
		CanEmbed:      pulled.CanEmbed,
		CanPrompt:     pulled.CanPrompt,
		CanStream:     pulled.CanStream,
		CanThink:      pulled.CanThink,
		CanVision:     pulled.CanVision,
	}
}

func ollamaPlaceholderModel(name string, modifiedAt time.Time) ollamaModel {
	return ollamaModel{
		Name:       name,
		Model:      name,
		ModifiedAt: modifiedAt,
		Details: ollamaModelDetails{
			Format: "contenox",
			Family: "contenox",
		},
	}
}

func ollamaPlaceholderModelInfo(name string, modifiedAt time.Time) ollamaModelInfo {
	return ollamaModelInfo{
		Model:     ollamaPlaceholderModel(name, modifiedAt),
		CanChat:   true,
		CanPrompt: true,
		CanStream: true,
	}
}

func mergeOllamaModelInfo(existing, next ollamaModelInfo) ollamaModelInfo {
	if existing.Model.Name == "" {
		return next
	}
	merged := existing
	if next.Model.ModifiedAt.After(existing.Model.ModifiedAt) || existing.Model.Size == 0 {
		merged.Model = next.Model
	}
	if next.ContextLength > merged.ContextLength {
		merged.ContextLength = next.ContextLength
	}
	merged.CanChat = existing.CanChat || next.CanChat
	merged.CanEmbed = existing.CanEmbed || next.CanEmbed
	merged.CanPrompt = existing.CanPrompt || next.CanPrompt
	merged.CanStream = existing.CanStream || next.CanStream
	merged.CanThink = existing.CanThink || next.CanThink
	merged.CanVision = existing.CanVision || next.CanVision
	return merged
}

func (m ollamaModelInfo) supportsCompletion() bool {
	return m.CanChat || m.CanPrompt || m.CanStream
}

func (m ollamaModelInfo) capabilities() []string {
	caps := []string{}
	if m.supportsCompletion() {
		caps = append(caps, "completion")
	}
	if m.CanEmbed {
		caps = append(caps, "embedding")
	}
	if m.CanThink {
		caps = append(caps, "thinking")
	}
	if m.CanVision {
		caps = append(caps, "vision")
	}
	return caps
}

func (m ollamaModelInfo) modelInfo() map[string]any {
	info := map[string]any{"general.architecture": "contenox"}
	if m.ContextLength > 0 {
		info["contenox.context_length"] = m.ContextLength
	}
	return info
}

func (m ollamaModelInfo) parameters() string {
	if m.ContextLength <= 0 {
		return ""
	}
	return fmt.Sprintf("num_ctx %d", m.ContextLength)
}

func ollamaStream(stream *bool) bool {
	return stream == nil || *stream
}

func ollamaExecOverrides(options map[string]any) (*float64, *int) {
	var temp *float64
	if v, ok := optionFloat(options["temperature"]); ok {
		temp = &v
	}
	var maxTokens *int
	if v, ok := optionInt(options["num_predict"]); ok && v >= 0 {
		maxTokens = &v
	}
	return temp, maxTokens
}

func ollamaStopStrings(options map[string]any) []string {
	raw, ok := options["stop"]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func optionFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func optionInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case float32:
		return int(n), true
	case int:
		return n, true
	case int64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	default:
		return 0, false
	}
}

func ollamaDoneReason(resp *agentservice.PromptResponse) string {
	if openAIFinishReason(resp) == "length" {
		return "length"
	}
	return "stop"
}

func setNDJSONHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeJSONLine(w http.ResponseWriter, payload any) error {
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		return err
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	return nil
}

// decodeOllamaImages decodes Ollama's native base64 image list into image
// attachments. Ollama sends raw base64 (no data: URI and no declared media
// type), so the media type is sniffed from the decoded bytes; a payload that
// does not sniff as an image is rejected rather than forwarded as a phantom
// attachment the model would silently ignore.
func decodeOllamaImages(images []string) ([]taskengine.ImagePart, error) {
	if len(images) == 0 {
		return nil, nil
	}
	out := make([]taskengine.ImagePart, 0, len(images))
	for i, enc := range images {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(enc))
		if err != nil {
			return nil, fmt.Errorf("image %d is not valid base64: %w", i, err)
		}
		mimeType := http.DetectContentType(raw)
		if !strings.HasPrefix(mimeType, "image/") {
			return nil, fmt.Errorf("image %d is not a recognized image (sniffed %q)", i, mimeType)
		}
		out = append(out, taskengine.ImagePart{Data: raw, MimeType: mimeType})
	}
	return out, nil
}

func writeOllamaError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

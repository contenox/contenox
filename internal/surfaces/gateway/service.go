package gateway

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	apiframework "github.com/contenox/contenox/apiframework"
	"github.com/contenox/contenox/internal/kernel/llmresolver"
	"github.com/contenox/contenox/internal/models/llmrepo"
	"github.com/contenox/contenox/internal/models/modelrepo"
	"github.com/contenox/contenox/internal/models/modelrepo/ollama"
	"github.com/contenox/contenox/internal/models/ollamatokenizer"
	"github.com/contenox/contenox/internal/models/runtimestate"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/internal/trouble"
	"github.com/contenox/contenox/internal/version"
	"github.com/contenox/contenox/libbus"
	libdb "github.com/contenox/contenox/libdbexec"
	"github.com/contenox/contenox/liblicense"
	"github.com/contenox/contenox/libtokenkey"
	"github.com/contenox/contenox/libtracker"
	"github.com/google/uuid"
)

const embedFinishReason = "embed"

func ollamaError(w http.ResponseWriter, status int, message string) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	return json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// ErrAllowanceExhausted, ErrGlobalCapReached and ErrModelNotAllowed classify
// the three refusals AuthorizeTurn and ChatTurn can raise, so a caller can tell
// "a key spent its week" from "the deployment is at its cap" from "the licence
// does not cover this model".
var (
	ErrAllowanceExhausted = errors.New("gateway: weekly allowance exhausted")
	ErrGlobalCapReached   = errors.New("gateway: global weekly proxy cap reached")
	ErrModelNotAllowed    = errors.New("gateway: model not permitted by this license")
)

// ClaimsVerifier verifies and decrypts license tokens.
type ClaimsVerifier interface {
	Verify(token string) (*liblicense.Claims, error)
}

// Config configures a gateway Service.
type Config struct {
	DB                 libdb.DBManager
	Verifier           ClaimsVerifier
	Models             llmrepo.ModelRepo
	Runtime            *runtimestate.State
	Hasher             *libtokenkey.Hasher
	GlobalWeeklyTokens int64
	Tracker            libtracker.ActivityTracker
	Trouble            trouble.Recorder
	Tokenizer          ollamatokenizer.Tokenizer
	Bus                libbus.Messenger
	// MeterLeasePath is where this deployment elects its one meter writer. A
	// usage snapshot is the previous snapshot plus this turn, so several writers
	// lose turns; the path must be shared by every instance that has to agree on
	// the writer. Empty means every instance writes, which is what one process
	// needs and what a deployment of several must not rely on.
	MeterLeasePath string
}
type Service interface {
	AddOllamaProxyRoutes(mux *http.ServeMux)
	AddOpenAIProxyRoutes(mux *http.ServeMux)
	// SubscribeControlPlane follows revocations published by the control plane
	// and answers a peer's request for the cutoffs this instance holds.
	SubscribeControlPlane(ctx context.Context) error
	// StartUsageConsumer drains turn usage events into the metering tables.
	StartUsageConsumer(ctx context.Context) error
	// ChatTurn runs one turn in process, spending the bearer's allowance by the
	// same rules a proxied turn does.
	ChatTurn(ctx context.Context, bearer, model string, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error)
	// BroadcastRevocation cuts a client or key off on every gateway sharing the
	// bus, without waiting for the ledger to be re-read.
	BroadcastRevocation(ctx context.Context, ev ProxyControlRevocationEvent) error
}

type service struct {
	db                 libdb.DBManager
	keys               runtimetypes.Store
	usage              runtimetypes.UsageStore
	verifier           ClaimsVerifier
	models             llmrepo.ModelRepo
	runtime            *runtimestate.State
	hasher             *libtokenkey.Hasher
	globalWeeklyTokens int64
	tracker            libtracker.ActivityTracker
	trouble            trouble.Recorder
	bus                libbus.Messenger
	instanceID         string
	tokenizer          ollamatokenizer.Tokenizer
	revocations        *revocationCache
	breaker            *providerCircuitBreaker
	meter              *meterWriter
}

// New constructs a gateway Service.
func New(cfg Config) (Service, error) {
	if cfg.Verifier == nil {
		return nil, fmt.Errorf("gateway: claims verifier is required")
	}
	return newService(cfg)
}

func newService(cfg Config) (*service, error) {
	if cfg.DB == nil {
		return nil, fmt.Errorf("gateway: database manager is required")
	}
	if cfg.Runtime == nil {
		return nil, fmt.Errorf("gateway: runtime state is required")
	}
	if cfg.Models == nil {
		return nil, fmt.Errorf("gateway: the model repo is required")
	}
	tracker := cfg.Tracker
	if tracker == nil {
		tracker = libtracker.NoopTracker{}
	}
	tokenizer := cfg.Tokenizer
	if tokenizer == nil {
		tokenizer = ollamatokenizer.NewEstimateTokenizer()
	}

	return &service{
		db:                 cfg.DB,
		instanceID:         uuid.NewString(),
		keys:               runtimetypes.New(cfg.DB.WithoutTransaction()),
		usage:              runtimetypes.NewUsageStore(cfg.DB),
		verifier:           cfg.Verifier,
		models:             cfg.Models,
		runtime:            cfg.Runtime,
		hasher:             cfg.Hasher,
		globalWeeklyTokens: cfg.GlobalWeeklyTokens,
		tracker:            tracker,
		trouble:            cfg.Trouble,
		bus:                cfg.Bus,
		tokenizer:          tokenizer,
		revocations:        newRevocationCache(),
		breaker:            newProviderCircuitBreaker(breakerThreshold, breakerCooldown),
		meter:              newMeterWriter(runtimetypes.NewUsageStore(cfg.DB), cfg.MeterLeasePath),
	}, nil
}

// AddOllamaProxyRoutes mounts Ollama API endpoints onto mux.
func (s *service) AddOllamaProxyRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tags", s.handleTags)
	mux.HandleFunc("POST /api/show", s.handleShow)
	mux.HandleFunc("POST /api/chat", s.handleChat)
	mux.HandleFunc("POST /api/generate", s.handleGenerate)
	mux.HandleFunc("POST /api/embed", s.handleEmbed)
	mux.HandleFunc("GET /api/version", s.handleVersion)
	mux.HandleFunc("GET /api/contenox", s.handleContenox)
	mux.HandleFunc("GET /api/contenox/usage", s.handleUsage)
}

const ollamaAPICompatibility = "0.5.1"

var endpointExtensions = []string{ollama.ExtensionAudios, ollama.ExtensionSession}

// handleVersion answers as vanilla Ollama.
//
// @security none
func (s *service) handleVersion(w http.ResponseWriter, r *http.Request) {
	resp := VersionResponse{Version: ollamaAPICompatibility}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response gateway.VersionResponse
}

// handleContenox declares this endpoint's identity and the wire extensions it
// understands, for a client that knows to ask.
//
// @security none
func (s *service) handleContenox(w http.ResponseWriter, r *http.Request) {
	resp := ContenoxResponse{
		Product:       ollama.ProductContenoxGateway,
		Version:       version.Get(),
		Build:         version.GetProvenance().String(),
		OllamaVersion: ollamaAPICompatibility,
		Extensions:    endpointExtensions,
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response gateway.ContenoxResponse
}

// handleUsage returns the authenticated client's usage and allowances for one
// model.
func (s *service) handleUsage(w http.ResponseWriter, r *http.Request) {
	claims, key, err := s.authenticate(r)
	if err != nil {
		_ = ollamaError(w, http.StatusUnauthorized, "unauthorized: "+err.Error())
		return
	}
	model := strings.TrimSpace(r.URL.Query().Get("model"))
	if model == "" {
		_ = ollamaError(w, http.StatusBadRequest, "model query parameter is required")
		return
	}
	if !s.isModelAllowed(claims, model) {
		_ = ollamaError(w, http.StatusForbidden, fmt.Sprintf("model %q not allowed for current license", model))
		return
	}
	clientID := claims.Subject
	if key != nil {
		clientID = key.ClientID
	}
	now := time.Now().UTC()
	read := func(kind string) (UsageWindow, error) {
		snapshot, err := s.usage.ReadUsage(r.Context(), runtimetypes.UsageScope{
			Scope: runtimetypes.UsageScopeClient, ScopeID: clientID,
			Model: model, WindowKind: kind,
		}, now)
		if err != nil {
			return UsageWindow{}, err
		}
		return usageWindow(kind, snapshot), nil
	}
	fiveHour, err := read(runtimetypes.UsageWindowFiveHour)
	if err != nil {
		_ = ollamaError(w, http.StatusInternalServerError, "failed to read usage")
		return
	}
	week, err := read(runtimetypes.UsageWindowWeek)
	if err != nil {
		_ = ollamaError(w, http.StatusInternalServerError, "failed to read usage")
		return
	}
	month, err := read(runtimetypes.UsageWindowMonth)
	if err != nil {
		_ = ollamaError(w, http.StatusInternalServerError, "failed to read usage")
		return
	}
	total, err := read(runtimetypes.UsageWindowTotal)
	if err != nil {
		_ = ollamaError(w, http.StatusInternalServerError, "failed to read usage")
		return
	}
	resp := UsageResponse{
		ClientID: clientID,
		Model:    model,
		Windows: UsageWindows{
			FiveHour: fiveHour,
			Week:     week,
			Month:    month,
			Total:    total,
		},
		Allowances: UsageAllowances{
			WeeklyOutputTokens:         liblicense.OutputAllowanceFor(claims, model),
			WeeklyInputTokens:          liblicense.InputAllowanceFor(claims, model),
			FiveHourTokens:             liblicense.FiveHourAllowanceFor(claims, model),
			MonthlyBudgetMicrodollars:  int64(liblicense.MonthlyBudgetUSDFor(claims, model) * 1_000_000),
			WeeklyImages:               liblicense.ImageAllowanceFor(claims, model),
			WeeklyAudioMiB:             liblicense.AudioAllowanceFor(claims, model),
			CacheDiscountMultiplier:    liblicense.CacheDiscountMultiplierFor(claims, model),
			ThinkingDiscountMultiplier: liblicense.ThinkingDiscountMultiplierFor(claims, model),
		},
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response gateway.UsageResponse
}

func usageWindow(kind string, snapshot runtimetypes.UsageSnapshot) UsageWindow {
	return UsageWindow{
		Kind:  kind,
		Start: snapshot.WindowStart,
		Usage: UsageCounters{
			PromptTokens:          snapshot.PromptTokens,
			CompletionTokens:      snapshot.CompletionTokens,
			ThinkingTokens:        snapshot.ThinkingTokens,
			VisibleOutputTokens:   snapshot.CompletionTokens - snapshot.ThinkingTokens,
			TotalTokens:           snapshot.TotalTokens,
			CacheReadTokens:       snapshot.CacheReadTokens,
			CacheWriteTokens:      snapshot.CacheWriteTokens,
			EffectiveInputTokens:  snapshot.EffectiveInput,
			EffectiveOutputTokens: snapshot.EffectiveOutput,
			EffectiveTotalTokens:  snapshot.EffectiveTokens(),
			CostMicrodollars:      snapshot.CostMicrodollars,
			ImageCount:            snapshot.ImageCount,
			AudioBytes:            snapshot.AudioBytes,
		},
	}
}

// handleTags lists the models the runtime state currently observes, filtered to
// the authenticated key's allowed set.
//
// @security none
func (s *service) handleTags(w http.ResponseWriter, r *http.Request) {
	claims, _, err := s.authenticate(r)
	if err != nil {
		_ = ollamaError(w, http.StatusUnauthorized, "unauthorized: "+err.Error())
		return
	}

	allowed := s.resolveAllowedModels(claims)
	var modelInfos []ModelInfo
	for _, st := range s.runtime.Get(r.Context()) {
		for _, m := range st.PulledModels {
			name := strings.TrimSpace(m.Model)
			if name == "" {
				name = strings.TrimSpace(m.Name)
			}
			if !modelAllowed(allowed, name) {
				continue
			}
			modelInfos = append(modelInfos, ModelInfo{
				Name:       name,
				Model:      name,
				ModifiedAt: m.ModifiedAt,
				Size:       m.Size,
				Digest:     m.Digest,
				Details:    modelDetailsFromRuntime(m.Details),
			})
		}
	}

	_ = apiframework.Encode(w, r, http.StatusOK, TagsResponse{Models: modelInfos}) // @response gateway.TagsResponse
}

// handleShow returns the runtime state's metadata for one model.
//
// @security none
func (s *service) handleShow(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[ShowRequest](r) // @request gateway.ShowRequest
	if err != nil {
		_ = ollamaError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	claims, _, err := s.authenticate(r)
	if err != nil {
		_ = ollamaError(w, http.StatusUnauthorized, "unauthorized: "+err.Error())
		return
	}

	if !s.isModelAllowed(claims, req.Model) {
		_ = ollamaError(w, http.StatusForbidden, fmt.Sprintf("model %q not allowed for current license", req.Model))
		return
	}

	pull := s.findPulledModel(r.Context(), req.Model)
	if pull == nil {
		_ = ollamaError(w, http.StatusNotFound, fmt.Sprintf("model %q is not available", req.Model))
		return
	}

	capabilities, info, parameters := reportFacts(pull)
	resp := ShowResponse{
		Modelfile:    fmt.Sprintf("FROM %s", pull.Model),
		Details:      modelDetailsFromRuntime(pull.Details),
		Capabilities: capabilities,
		ModelInfo:    info,
		Parameters:   parameters,
		ModifiedAt:   pull.ModifiedAt,
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response gateway.ShowResponse
}

func (s *service) handleChat(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[ChatRequest](r) // @request gateway.ChatRequest
	if err != nil {
		_ = ollamaError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	who, err := s.callerForRequest(r)
	if err != nil {
		_ = ollamaError(w, http.StatusUnauthorized, "unauthorized: "+err.Error())
		return
	}
	if !s.admitRequest(w, r, who, req.Model) {
		return
	}
	claims, key := who.claims, who.key

	messages, err := toModelMessages(req.Messages)
	if err != nil {
		_ = ollamaError(w, http.StatusBadRequest, err.Error())
		return
	}
	session := sessionKeyFor(key, claims, req.Model, req.Session, messages)
	resReq, health := s.chatRequest(req.Model, session, messages)
	args := toModelArgs(req)

	if req.Stream == nil || *req.Stream {
		s.streamTurn(w, r, resReq, health, req.Model, messages, args, false, key, claims)
		return
	}
	s.runOnce(w, r, resReq, health, req.Model, messages, args, false, key, claims)
}

// handleGenerate processes a generation request by turning it into a chat turn.
//
// @security none
func (s *service) handleGenerate(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[GenerateRequest](r) // @request gateway.GenerateRequest
	if err != nil {
		_ = ollamaError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	who, err := s.callerForRequest(r)
	if err != nil {
		_ = ollamaError(w, http.StatusUnauthorized, "unauthorized: "+err.Error())
		return
	}
	if !s.admitRequest(w, r, who, req.Model) {
		return
	}
	claims, key := who.claims, who.key

	audio, err := toModelAudio(req.Audios)
	if err != nil {
		_ = ollamaError(w, http.StatusBadRequest, err.Error())
		return
	}
	messages := make([]modelrepo.Message, 0, 2)
	if req.System != "" {
		messages = append(messages, modelrepo.Message{Role: "system", Content: req.System})
	}
	messages = append(messages, modelrepo.Message{Role: "user", Content: req.Prompt, Audio: audio})

	session := sessionKeyFor(key, claims, req.Model, req.Session, messages)
	resReq, health := s.chatRequest(req.Model, session, messages)

	if req.Stream == nil || *req.Stream {
		s.streamTurn(w, r, resReq, health, req.Model, messages, nil, true, key, claims)
		return
	}
	s.runOnce(w, r, resReq, health, req.Model, messages, nil, true, key, claims)
}

// chatRequest names the model as the client did and its normalized alternate,
// which the resolver may match against a backend that spells it differently. It
// carries the session among the routing constraints, so a conversation's turns
// share a backend and a warm prefix cache.
//
// Options carries no routing constraint: num_ctx is the client's request for a
// context window, while llmrepo.Request.ContextLength is the MINIMUM a candidate
// provider must offer, so passing one through as the other refuses every model
// smaller than the ask. A provider takes num_ctx as a load option, which is the
// provider's layer to honour, not the router's.
// backendHealth is the breaker's view handed to the model layer: a backend it
// has taken out of service is skipped rather than picked and then abandoned, so
// the failure is not spent again on every turn.
func (s *service) backendHealth() llmrepo.BackendHealth {
	if s.breaker == nil {
		return nil
	}
	return func(backendID string) bool {
		return s.breaker.IsAvailable(backendID, time.Now().UTC())
	}
}

func (s *service) chatRequest(model, sessionKey string, messages []modelrepo.Message) (llmrepo.Request, *healthTracker) {
	health := &healthTracker{next: s.tracker, svc: s}
	req := llmrepo.Request{
		ModelNames:    []string{model},
		SessionKey:    sessionKey,
		CacheHints:    sessionHints(sessionKey, messages),
		Tracker:       health,
		BackendHealth: s.backendHealth(),
	}
	if normalized := llmresolver.NormalizeModelName(model); normalized != strings.ToLower(model) {
		req.ModelNames = append(req.ModelNames, normalized)
	}
	return req, health
}

// handleEmbed serves one embedding request, or one per input when the caller
// batches, metered on the input ceilings a turn is.
//
// @security none
func (s *service) handleEmbed(w http.ResponseWriter, r *http.Request) {
	req, err := apiframework.Decode[EmbedRequest](r) // @request gateway.EmbedRequest
	if err != nil {
		_ = ollamaError(w, http.StatusBadRequest, "malformed request body")
		return
	}

	who, err := s.callerForRequest(r)
	if err != nil {
		_ = ollamaError(w, http.StatusUnauthorized, "unauthorized: "+err.Error())
		return
	}
	if !s.admitRequest(w, r, who, req.Model) {
		return
	}
	claims, key := who.claims, who.key

	if req.Dimensions > 0 {
		_ = ollamaError(w, http.StatusBadRequest, "dimensions is not served: a provider's embedding client returns its model's own vectors, so honouring it is not something this gateway can do — ask for the model whose dimension you want")
		return
	}

	inputs, err := embedInputs(req.Input)
	if err != nil {
		ollamaError(w, http.StatusBadRequest, err.Error())
		return
	}

	start := time.Now()
	embeddings := make([][]float32, 0, len(inputs))
	promptTokens := int64(0)
	backend := ""
	for _, input := range inputs {
		vector, meta, err := s.models.Embed(r.Context(), llmrepo.EmbedRequest{ModelName: req.Model, BackendHealth: s.backendHealth()}, input)
		if err != nil {
			s.recordBackendHealth(backendRefusal{
				backend:     s.backendIDFor(r.Context(), meta.BackendID),
				terminal:    modelrepo.IsBackendTerminal(err),
				rateLimited: errors.Is(err, modelrepo.ErrRateLimited),
			})
			s.handleProviderTurnError(w, r, "embed", err)
			return
		}
		s.recordBackendSuccess(meta.BackendID)
		backend = meta.BackendID
		embeddings = append(embeddings, toFloat32(vector))
		if count, err := s.tokenizer.CountTokens(r.Context(), req.Model, input); err == nil {
			promptTokens += int64(count)
		}
	}

	s.chargeTurn(r.Context(), key, claims, turnUsage{
		backend: backend, model: req.Model, prompt: promptTokens,
		durationMs: time.Since(start).Milliseconds(), finishReason: embedFinishReason,
	})

	_ = apiframework.Encode(w, r, http.StatusOK, EmbedResponse{ // @response gateway.EmbedResponse
		Model:           req.Model,
		Embeddings:      embeddings,
		PromptEvalCount: int(promptTokens),
		TotalDuration:   time.Since(start),
	})
}

func embedInputs(input any) ([]string, error) {
	switch value := input.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("input must not be empty")
		}
		return []string{value}, nil
	case []any:
		if len(value) == 0 {
			return nil, errors.New("input must not be empty")
		}
		inputs := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, errors.New("every input must be a non-empty string")
			}
			inputs = append(inputs, text)
		}
		return inputs, nil
	default:
		return nil, errors.New("input must be a string or an array of strings")
	}
}

func toFloat32(vector []float64) []float32 {
	out := make([]float32, len(vector))
	for i, value := range vector {
		out[i] = float32(value)
	}
	return out
}

func (s *service) findPulledModel(ctx context.Context, name string) *runtimestate.ModelPullStatus {
	state := s.runtime.Get(ctx)
	ids := make([]string, 0, len(state))
	for id := range state {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		st := state[id]
		if model := pulledModelIn(&st, name); model != nil {
			return model
		}
	}
	return nil
}

func (s *service) pulledModelOn(ctx context.Context, backendURL, name string) *runtimestate.ModelPullStatus {
	backendURL = strings.TrimSpace(backendURL)
	if backendURL == "" {
		return s.findPulledModel(ctx, name)
	}
	for _, st := range s.runtime.Get(ctx) {
		state := st
		if strings.TrimSpace(state.Backend.BaseURL) != backendURL {
			continue
		}
		if model := pulledModelIn(&state, name); model != nil {
			return model
		}
	}
	return nil
}

func pulledModelIn(state *runtimestate.BackendRuntimeState, name string) *runtimestate.ModelPullStatus {
	name = strings.TrimSpace(name)
	for i := range state.PulledModels {
		m := &state.PulledModels[i]
		if strings.TrimSpace(m.Model) == name || strings.TrimSpace(m.Name) == name {
			return m
		}
	}
	return nil
}

type healthTracker struct {
	next libtracker.ActivityTracker
	svc  *service
	mu   sync.Mutex
	last backendRefusal
}

// backendRefusal is one backend excluded from a turn, as llmrepo reported it.
type backendRefusal struct {
	backend     string
	terminal    bool
	rateLimited bool
}

func (h *healthTracker) Start(ctx context.Context, operation, subject string, kvArgs ...any) (func(error), func(string, any), func()) {
	reportErr, reportChange, end := h.next.Start(ctx, operation, subject, kvArgs...)
	return reportErr, func(id string, data any) {
		h.capture(id, data)
		reportChange(id, data)
	}, end
}

func (h *healthTracker) capture(id string, data any) {
	if h == nil || id != "backend_excluded" {
		return
	}
	fields, ok := data.(map[string]any)
	if !ok {
		return
	}
	backend, _ := fields["backend_id"].(string)
	if backend == "" {
		return
	}
	terminal, _ := fields["terminal"].(bool)
	rateLimited, _ := fields["rate_limited"].(bool)
	h.mu.Lock()
	h.last = backendRefusal{backend: backend, terminal: terminal, rateLimited: rateLimited}
	h.mu.Unlock()
}

func (h *healthTracker) record() {
	if h == nil {
		return
	}
	h.mu.Lock()
	last := h.last
	h.last = backendRefusal{}
	h.mu.Unlock()
	if last.backend == "" {
		return
	}
	last.backend = h.svc.backendIDFor(context.WithoutCancel(context.Background()), last.backend)
	h.svc.recordBackendHealth(last)
}

func (s *service) backendIDFor(ctx context.Context, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	for _, st := range s.runtime.Get(ctx) {
		state := st
		if strings.TrimSpace(state.Backend.BaseURL) == endpoint {
			return strings.TrimSpace(state.Backend.ID)
		}
	}
	return ""
}

// quarantineError separates "wait for the cooldown" from "the upstream failed":
// every backend that can serve the model has been taken out of service, so the
// answer is a retry window rather than the failure of one endpoint.
var quarantineError = errors.New("no backend is currently serving requests for this model")

func (s *service) quarantined(ctx context.Context, model string) bool {
	if s.breaker == nil {
		return false
	}
	capable := 0
	healthy := 0
	for _, st := range s.runtime.Get(ctx) {
		state := st
		if pulledModelIn(&state, model) == nil {
			continue
		}
		capable++
		if s.breaker.WouldServe(strings.TrimSpace(state.Backend.ID), time.Now().UTC()) {
			healthy++
		}
	}
	return capable > 0 && healthy == 0
}

func (s *service) recordBackendHealth(refusal backendRefusal) {
	if s.breaker == nil || refusal.backend == "" {
		return
	}
	open := s.breaker.RecordFailure(refusal.backend, refusal, time.Now().UTC())
	if open && s.trouble != nil {
		s.trouble.Record(context.WithoutCancel(context.Background()), "gateway", "backend_quarantined")
	}
}

func (s *service) recordBackendSuccess(backendID string) {
	if s.breaker == nil || backendID == "" {
		return
	}
	s.breaker.RecordSuccess(backendID)
}

// admitRequest authenticates the caller, runs the same authorization an
// in-process turn runs, and reports whether the request may proceed. It exists so
// the HTTP routes and the gateway's own model repo share one policy: the checks
// below are not repeated per transport, they are called per transport.
func (s *service) admitRequest(w http.ResponseWriter, r *http.Request, who caller, model string) bool {
	switch err := s.authorizeCaller(r.Context(), who, model); {
	case err == nil:
		return true
	case errors.Is(err, ErrModelNotAllowed):
		_ = ollamaError(w, http.StatusForbidden, fmt.Sprintf("model %q not allowed for current license", model))
	case errors.Is(err, quarantineError):
		w.Header().Set("Retry-After", strconv.Itoa(int(breakerCooldown.Seconds())))
		_ = ollamaError(w, http.StatusServiceUnavailable, err.Error())
	default:
		s.denyTurn(w, r, err)
	}
	return false
}

func (s *service) callerForRequest(r *http.Request) (caller, error) {
	claims, key, err := s.authenticate(r)
	if err != nil {
		return caller{}, err
	}
	return caller{claims: claims, key: key}, nil
}

func (s *service) streamTurn(w http.ResponseWriter, r *http.Request, resReq llmrepo.Request, health *healthTracker, model string, messages []modelrepo.Message, args []modelrepo.ChatArgument, generate bool, key *runtimetypes.ProxyKey, claims *liblicense.Claims) {
	start := time.Now()
	ch, meta, err := s.models.Stream(r.Context(), resReq, messages, args...)
	if err != nil {
		health.record()
		s.handleProviderTurnError(w, r, "stream_start", err)
		return
	}

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)

	var promptTokens, completionTokens, thinkingTokens, cacheReadTokens, cacheWriteTokens int64
	var finishReason string
	images := int64(modelrepo.MessagesImageCount(messages))
	_, audioBytes := modelrepo.MessagesAudioBytes(messages)
	charged := false
	charge := func(finish string) {
		if charged {
			return
		}
		charged = true
		s.chargeTurn(r.Context(), key, claims, turnUsage{
			backend: meta.BackendID, model: model,
			prompt: promptTokens, completion: completionTokens,
			thinking:  thinkingTokens,
			cacheRead: cacheReadTokens, cacheWrite: cacheWriteTokens,
			images: images, audioBytes: int64(audioBytes),
			durationMs: time.Since(start).Milliseconds(), finishReason: finish,
		})
	}

	for parcel := range ch {
		if parcel == nil {
			continue
		}
		if parcel.Terminal != nil {
			finishReason = parcel.Terminal.FinishReason
			if parcel.Terminal.Usage != nil {
				promptTokens = int64(parcel.Terminal.Usage.PromptTokens)
				completionTokens = int64(parcel.Terminal.Usage.CompletionTokens)
				thinkingTokens = int64(parcel.Terminal.Usage.ThinkingTokens)
				cacheReadTokens = int64(parcel.Terminal.Usage.CacheReadTokens)
				cacheWriteTokens = int64(parcel.Terminal.Usage.CacheWriteTokens)
			}
		} else if parcel.Usage != nil {
			promptTokens = int64(parcel.Usage.PromptTokens)
			completionTokens = int64(parcel.Usage.CompletionTokens)
			thinkingTokens = int64(parcel.Usage.ThinkingTokens)
			cacheReadTokens = int64(parcel.Usage.CacheReadTokens)
			cacheWriteTokens = int64(parcel.Usage.CacheWriteTokens)
		}

		var frame any
		if generate {
			frame = parcelToGenerateResponse(model, parcel)
		} else {
			frame = parcelToChatResponse(model, parcel)
		}
		if err := enc.Encode(frame); err != nil {
			s.recordTurnFailure(r.Context(), "stream_encode", err)
			charge(finishReason)
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		if parcel.Error != nil {
			s.recordTurnFailure(r.Context(), "stream_upstream", parcel.Error)
			health.record()
			charge(finishReason)
			return
		}
	}
	s.recordBackendSuccess(s.backendIDFor(context.WithoutCancel(r.Context()), meta.BackendID))
	charge(finishReason)
}

func (s *service) runOnce(w http.ResponseWriter, r *http.Request, resReq llmrepo.Request, health *healthTracker, model string, messages []modelrepo.Message, args []modelrepo.ChatArgument, generate bool, key *runtimetypes.ProxyKey, claims *liblicense.Claims) {
	start := time.Now()
	res, meta, err := s.models.Chat(r.Context(), resReq, messages, args...)
	health.record()
	if err != nil {
		s.handleProviderTurnError(w, r, "chat", err)
		return
	}
	s.recordBackendSuccess(s.backendIDFor(r.Context(), meta.BackendID))

	var promptTokens, completionTokens, thinkingTokens, cacheReadTokens, cacheWriteTokens int64
	if res.Usage != nil {
		promptTokens = int64(res.Usage.PromptTokens)
		completionTokens = int64(res.Usage.CompletionTokens)
		thinkingTokens = int64(res.Usage.ThinkingTokens)
		cacheReadTokens = int64(res.Usage.CacheReadTokens)
		cacheWriteTokens = int64(res.Usage.CacheWriteTokens)
	}
	_, audioBytes := modelrepo.MessagesAudioBytes(messages)
	s.chargeTurn(r.Context(), key, claims, turnUsage{
		backend: meta.BackendID, model: model,
		prompt: promptTokens, completion: completionTokens,
		thinking:  thinkingTokens,
		cacheRead: cacheReadTokens, cacheWrite: cacheWriteTokens,
		images: int64(modelrepo.MessagesImageCount(messages)), audioBytes: int64(audioBytes),
		durationMs: time.Since(start).Milliseconds(), finishReason: res.FinishReason,
	})

	if generate {
		resp := GenerateResponse{
			Model:           model,
			CreatedAt:       time.Now().UTC(),
			Response:        res.Message.Content,
			Thinking:        res.Message.Thinking,
			Done:            true,
			DoneReason:      res.FinishReason,
			PromptEvalCount: int(promptTokens),
			EvalCount:       int(completionTokens),
		}
		_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response gateway.GenerateResponse
		return
	}

	resp := ChatResponse{
		Model:           model,
		CreatedAt:       time.Now().UTC(),
		Message:         ChatMessage{Role: "assistant", Content: res.Message.Content, Thinking: res.Message.Thinking},
		Done:            true,
		DoneReason:      res.FinishReason,
		PromptEvalCount: int(promptTokens),
		EvalCount:       int(completionTokens),
	}
	_ = apiframework.Encode(w, r, http.StatusOK, resp) // @response gateway.ChatResponse
}

const contextLengthKey = "contenox.context_length"

func reportFacts(pull *runtimestate.ModelPullStatus) ([]string, map[string]any, string) {
	capabilities := pull.DeclaredCapabilities
	if len(capabilities) == 0 {
		capabilities = observedReportCapabilities(pull)
	}

	var info map[string]any
	if pull.ContextLength > 0 {
		info = map[string]any{contextLengthKey: pull.ContextLength}
	}
	parameters := ""
	if pull.MaxOutputTokens > 0 {
		parameters = fmt.Sprintf("num_predict %d", pull.MaxOutputTokens)
	}
	return capabilities, info, parameters
}

// observedReportCapabilities has no tools flag to read: the runtime records
// tools as chat, so only an operator's declaration can report them.
func observedReportCapabilities(pull *runtimestate.ModelPullStatus) []string {
	capabilities := make([]string, 0, 4)
	if pull.CanChat || pull.CanPrompt || pull.CanStream {
		capabilities = append(capabilities, runtimetypes.CapabilityCompletion)
	}
	if pull.CanEmbed {
		capabilities = append(capabilities, runtimetypes.CapabilityEmbedding)
	}
	if pull.CanVision {
		capabilities = append(capabilities, runtimetypes.CapabilityVision)
	}
	if pull.CanAudio {
		capabilities = append(capabilities, runtimetypes.CapabilityAudio)
	}
	if pull.CanThink {
		capabilities = append(capabilities, runtimetypes.CapabilityThinking)
	}
	if len(capabilities) == 0 {
		return nil
	}
	return capabilities
}

func modelDetailsFromRuntime(d runtimestate.ModelDetails) ModelDetails {
	return ModelDetails{
		ParentModel:       d.ParentModel,
		Format:            d.Format,
		Family:            d.Family,
		Families:          d.Families,
		ParameterSize:     d.ParameterSize,
		QuantizationLevel: d.QuantizationLevel,
	}
}

func bearerFrom(r *http.Request) string {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	token := strings.TrimPrefix(authHeader, "Bearer ")
	token = strings.TrimPrefix(token, "bearer ")
	token = strings.TrimSpace(token)

	if token == "" {
		token = strings.TrimSpace(r.URL.Query().Get("token"))
	}
	return token
}

func (s *service) authenticate(r *http.Request) (*liblicense.Claims, *runtimetypes.ProxyKey, error) {
	return s.AuthorizeBearer(r.Context(), bearerFrom(r))
}

func (s *service) AuthorizeBearer(ctx context.Context, token string) (*liblicense.Claims, *runtimetypes.ProxyKey, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, nil, fmt.Errorf("missing authentication token: expected an Authorization header or a token query parameter")
	}

	claims, err := s.verifier.Verify(token)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid or expired token: %w", err)
	}

	if s.hasher == nil || s.db == nil {
		return claims, nil, nil
	}

	keyHash, err := s.hasher.Hash(libtokenkey.PurposeProxyKey, token)
	if err != nil {
		return nil, nil, fmt.Errorf("hash presented bearer: %w", err)
	}
	key, err := s.keys.GetProxyKeyByHash(ctx, keyHash)
	if errors.Is(err, libdb.ErrNotFound) {
		return nil, nil, fmt.Errorf("bearer was not issued by this gateway")
	}
	if err != nil {
		return nil, nil, err
	}
	if key.RevokedAt != nil {
		return nil, nil, fmt.Errorf("bearer has been revoked")
	}
	now := time.Now().UTC()
	if now.Before(key.IssuedAt) || !now.Before(key.ExpiresAt) {
		return nil, nil, fmt.Errorf("bearer is outside its validity window")
	}
	if s.revocations != nil {
		if blocked, reason := s.revocations.IsBlocked(key.ClientID, key.KeyHash, "", now); blocked {
			return nil, nil, fmt.Errorf("%w: %s", ErrAllowanceExhausted, reason)
		}
	}

	return claims, key, nil
}

func (s *service) AuthorizeTurn(ctx context.Context, key *runtimetypes.ProxyKey, claims *liblicense.Claims, model string) error {
	if key == nil {
		return nil
	}
	now := time.Now().UTC()

	if s.revocations != nil {
		if blocked, reason := s.revocations.IsBlocked(key.ClientID, key.KeyHash, model, now); blocked {
			return fmt.Errorf("%w: %s", ErrAllowanceExhausted, reason)
		}
	}

	if s.usage == nil {
		return nil
	}

	if s.globalWeeklyTokens > 0 {
		global, err := s.usage.ReadUsage(ctx, runtimetypes.UsageScope{
			Scope: runtimetypes.UsageScopeGlobal, Model: model, WindowKind: runtimetypes.UsageWindowWeek,
		}, now)
		if err != nil {
			return fmt.Errorf("meter global usage: %w", err)
		}
		if global.TotalTokens >= s.globalWeeklyTokens {
			return fmt.Errorf("%w: global weekly proxy allowance exhausted for model %q", ErrGlobalCapReached, model)
		}
	}

	if claims == nil {
		return nil
	}
	outputCap := liblicense.OutputAllowanceFor(claims, model)
	inputCap := liblicense.InputAllowanceFor(claims, model)
	imageCap := liblicense.ImageAllowanceFor(claims, model)
	audioCap := liblicense.AudioAllowanceFor(claims, model)
	if outputCap <= 0 && inputCap <= 0 && imageCap <= 0 && audioCap <= 0 {
		return nil
	}

	refill := runtimetypes.UsageWindowStart(runtimetypes.UsageWindowWeek, now).AddDate(0, 0, 7)
	next := refill.Format(time.RFC3339)

	weekly, err := s.usage.ReadUsage(ctx, runtimetypes.UsageScope{
		Scope: runtimetypes.UsageScopeClient, ScopeID: key.ClientID,
		Model: model, WindowKind: runtimetypes.UsageWindowWeek,
	}, now)
	if err != nil {
		return fmt.Errorf("meter client usage: %w", err)
	}

	fiveHourCap := liblicense.FiveHourAllowanceFor(claims, model)
	if fiveHourCap > 0 {
		burst, err := s.usage.ReadUsage(ctx, runtimetypes.UsageScope{
			Scope: runtimetypes.UsageScopeClient, ScopeID: key.ClientID,
			Model: model, WindowKind: runtimetypes.UsageWindowFiveHour,
		}, now)
		if err != nil {
			return fmt.Errorf("meter 5h window: %w", err)
		}
		if burst.EffectiveTokens() >= fiveHourCap {
			next5h := runtimetypes.UsageWindowStart(runtimetypes.UsageWindowFiveHour, now).Add(5 * time.Hour).Format(time.RFC3339)
			return fmt.Errorf("%w: model %q 5-hour burst allowance (%d tokens) reached; next reset at %s", ErrAllowanceExhausted, model, fiveHourCap, next5h)
		}
	}

	monthlyBudgetUSD := liblicense.MonthlyBudgetUSDFor(claims, model)
	if monthlyBudgetUSD > 0 {
		month, err := s.usage.ReadUsage(ctx, runtimetypes.UsageScope{
			Scope: runtimetypes.UsageScopeClient, ScopeID: key.ClientID,
			Model: model, WindowKind: runtimetypes.UsageWindowMonth,
		}, now)
		if err != nil {
			return fmt.Errorf("meter monthly spend: %w", err)
		}
		if month.CostMicrodollars >= int64(monthlyBudgetUSD*1_000_000) {
			nextMonthly := runtimetypes.UsageWindowStart(runtimetypes.UsageWindowMonth, now).AddDate(0, 1, 0).Format(time.RFC3339)
			return fmt.Errorf("%w: monthly spend budget ($%.2f) reached; next reset at %s", ErrAllowanceExhausted, monthlyBudgetUSD, nextMonthly)
		}
	}

	if outputCap > 0 && weekly.EffectiveOutput >= outputCap {
		_ = s.BroadcastRevocation(ctx, ProxyControlRevocationEvent{
			ClientID:  key.ClientID,
			KeyHash:   key.KeyHash,
			Model:     model,
			Reason:    "weekly output allowance exhausted",
			ExpiresAt: refill,
		})
		return fmt.Errorf("%w: model %q weekly output allowance exhausted for this client; next refill at %s", ErrAllowanceExhausted, model, next)
	}
	if inputCap > 0 && weekly.EffectiveInput >= inputCap {
		_ = s.BroadcastRevocation(ctx, ProxyControlRevocationEvent{
			ClientID:  key.ClientID,
			KeyHash:   key.KeyHash,
			Model:     model,
			Reason:    "weekly input allowance exhausted",
			ExpiresAt: refill,
		})
		return fmt.Errorf("%w: model %q weekly input allowance exhausted for this client; next refill at %s", ErrAllowanceExhausted, model, next)
	}
	if imageCap > 0 && weekly.ImageCount >= imageCap {
		_ = s.BroadcastRevocation(ctx, ProxyControlRevocationEvent{
			ClientID:  key.ClientID,
			KeyHash:   key.KeyHash,
			Model:     model,
			Reason:    "weekly image allowance exhausted",
			ExpiresAt: refill,
		})
		return fmt.Errorf("%w: model %q weekly image allowance (%d images) exhausted for this client; next refill at %s", ErrAllowanceExhausted, model, imageCap, next)
	}
	if audioCap > 0 && weekly.AudioBytes >= audioCap*runtimetypes.Mebibyte {
		_ = s.BroadcastRevocation(ctx, ProxyControlRevocationEvent{
			ClientID:  key.ClientID,
			KeyHash:   key.KeyHash,
			Model:     model,
			Reason:    "weekly audio allowance exhausted",
			ExpiresAt: refill,
		})
		return fmt.Errorf("%w: model %q weekly audio allowance (%d MiB) exhausted for this client; next refill at %s", ErrAllowanceExhausted, model, audioCap, next)
	}
	return nil
}

func (s *service) denyTurn(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrGlobalCapReached):
		if s.trouble != nil {
			s.trouble.Record(r.Context(), "gateway", "global_cap_hit")
		}
		_ = ollamaError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, ErrAllowanceExhausted):
		if s.trouble != nil {
			s.trouble.Record(r.Context(), "gateway", "allowance_exhausted")
		}
		_ = ollamaError(w, http.StatusTooManyRequests, err.Error())
	default:
		if s.trouble != nil {
			s.trouble.Record(r.Context(), "gateway", "deny_internal")
		}
		_ = ollamaError(w, http.StatusInternalServerError, err.Error())
	}
}

func (s *service) handleProviderTurnError(w http.ResponseWriter, r *http.Request, fallback string, err error) {
	s.recordTurnFailure(r.Context(), fallback, err)
	if errors.Is(err, modelrepo.ErrModelAccessDenied) || errors.Is(err, modelrepo.ErrRateLimited) {
		w.Header().Set("Retry-After", strconv.Itoa(int(breakerCooldown.Seconds())))
	}
	switch {
	case errors.Is(err, llmrepo.ErrInvalidRequest):
		_ = ollamaError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, llmrepo.ErrResolutionOutOfBounds):
		_ = ollamaError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, llmresolver.ErrNoAvailableModels),
		errors.Is(err, llmresolver.ErrNoSatisfactoryModel),
		errors.Is(err, llmresolver.ErrPinnedModelLacksVision):
		_ = ollamaError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, modelrepo.ErrContextLengthExceeded):
		_ = ollamaError(w, http.StatusBadRequest, "request exceeds the model's context window")
	case errors.Is(err, modelrepo.ErrRateLimited):
		_ = ollamaError(w, http.StatusTooManyRequests, "upstream provider rate limit or capacity limit reached; please try again later")
	case errors.Is(err, modelrepo.ErrModelNotFoundOnBackend):
		_ = ollamaError(w, http.StatusNotFound, "upstream provider does not serve the requested model")
	case errors.Is(err, modelrepo.ErrModelAccessDenied):
		_ = ollamaError(w, http.StatusServiceUnavailable, "upstream provider authentication or access failed; please contact your administrator")
	default:
		_ = ollamaError(w, http.StatusServiceUnavailable, "upstream provider request failed; please try again later")
	}
}

func sanitizeTurnError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, modelrepo.ErrContextLengthExceeded):
		return modelrepo.ErrContextLengthExceeded
	case errors.Is(err, modelrepo.ErrRateLimited):
		return fmt.Errorf("%w: upstream provider rate limit reached", modelrepo.ErrRateLimited)
	case errors.Is(err, modelrepo.ErrModelNotFoundOnBackend):
		return fmt.Errorf("%w: upstream provider does not serve model", modelrepo.ErrModelNotFoundOnBackend)
	case errors.Is(err, modelrepo.ErrModelAccessDenied):
		return fmt.Errorf("%w: upstream provider access or authentication failed", modelrepo.ErrModelAccessDenied)
	default:
		return errors.New("upstream provider request failed")
	}
}

func (s *service) recordTurnFailure(ctx context.Context, fallback string, err error) {
	if s.trouble == nil {
		return
	}
	op := fallback
	switch {
	case errors.Is(err, modelrepo.ErrContextLengthExceeded):
		op = "context_length_exceeded"
	case errors.Is(err, modelrepo.ErrRateLimited):
		op = "upstream_rate_limited"
	case errors.Is(err, modelrepo.ErrModelAccessDenied):
		op = "upstream_access_denied"
	case errors.Is(err, modelrepo.ErrModelNotFoundOnBackend):
		op = "upstream_model_not_found"
	}
	s.trouble.Record(ctx, "gateway", op)
}

type turnUsage struct {
	backend      string
	model        string
	prompt       int64
	completion   int64
	thinking     int64
	cacheRead    int64
	cacheWrite   int64
	images       int64
	audioBytes   int64
	durationMs   int64
	finishReason string
}

func (s *service) chargeTurn(ctx context.Context, key *runtimetypes.ProxyKey, claims *liblicense.Claims, turn turnUsage) {
	if s.db == nil || key == nil {
		return
	}
	if turn.prompt <= 0 && turn.completion <= 0 && turn.images <= 0 && turn.audioBytes <= 0 {
		return
	}

	var costMicrodollars int64
	if pull := s.pulledModelOn(ctx, turn.backend, turn.model); pull != nil && pull.Pricing != nil {
		costUSD := pull.Pricing.CalculateCost(turn.prompt, turn.cacheRead, turn.cacheWrite, turn.completion, turn.images, turn.audioBytes)
		costMicrodollars = int64(costUSD * 1_000_000)
	}

	model := turn.model
	promptTokens := turn.prompt
	cacheReadTokens := turn.cacheRead
	uncachedPrompt := promptTokens - cacheReadTokens
	if uncachedPrompt < 0 {
		uncachedPrompt = 0
	}
	cacheMultiplier := 1.0
	if claims != nil {
		cacheMultiplier = liblicense.CacheDiscountMultiplierFor(claims, model)
	}
	discountedCached := int64(float64(cacheReadTokens) * cacheMultiplier)
	effectiveInput := uncachedPrompt + discountedCached
	thinkingTokens := turn.thinking
	if thinkingTokens < 0 || thinkingTokens > turn.completion {
		thinkingTokens = 0
	}
	visibleOutput := turn.completion - thinkingTokens
	discountedThinking := int64(float64(thinkingTokens) * liblicense.ThinkingDiscountMultiplierFor(claims, model))
	effectiveOutput := visibleOutput + discountedThinking

	usage := runtimetypes.ProxyUsage{
		SessionID:        llmrepo.UsageSessionFromContext(ctx),
		KeyHash:          key.KeyHash,
		ClientID:         key.ClientID,
		Model:            model,
		PromptTokens:     promptTokens,
		CompletionTokens: turn.completion,
		ThinkingTokens:   thinkingTokens,
		TotalTokens:      promptTokens + turn.completion,
		CacheReadTokens:  cacheReadTokens,
		CacheWriteTokens: turn.cacheWrite,
		EffectiveInput:   effectiveInput,
		EffectiveOutput:  effectiveOutput,
		CostMicrodollars: costMicrodollars,
		ImageCount:       turn.images,
		AudioBytes:       turn.audioBytes,
		DurationMs:       turn.durationMs,
		FinishReason:     turn.finishReason,
	}

	// One writer per deployment: the instance holding the meter lease records a
	// turn it served, and a turn served anywhere else arrives on the bus and is
	// recorded there. Both writing is how one turn becomes two charges.
	wrote, err := s.meter.Record(ctx, usage)
	if err != nil && s.trouble != nil {
		s.trouble.Record(ctx, "gateway", "meter")
	}
	if wrote || s.bus == nil {
		return
	}
	if raw, err := json.Marshal(ProxyUsageTurnEvent{Usage: usage, Origin: s.instanceID}); err == nil {
		_ = s.bus.Publish(ctx, SubjectProxyUsageTurn, raw)
	}
}

// caller is a turn's verified identity: the licence it presented and the ledger
// row that licence is. Every turn path resolves one of these first — from a
// bearer off the wire, or from the identity an in-process repo was built with —
// and then runs the same checks, so the policy has one implementation rather
// than one per transport.
type caller struct {
	claims *liblicense.Claims
	key    *runtimetypes.ProxyKey
}

func (s *service) callerFromBearer(ctx context.Context, bearer string) (caller, error) {
	claims, key, err := s.AuthorizeBearer(ctx, bearer)
	if err != nil {
		return caller{}, err
	}
	return caller{claims: claims, key: key}, nil
}

// authorizeCaller answers whether this caller may spend on this model: the model
// is in the licence, the client's own ceilings hold, the operator's global cap
// holds, and nothing serving it is out of service.
func (s *service) authorizeCaller(ctx context.Context, who caller, model string) error {
	if !s.isModelAllowed(who.claims, model) {
		return fmt.Errorf("%w: model %q is not in this license's allowed list", ErrModelNotAllowed, model)
	}
	if err := s.AuthorizeTurn(ctx, who.key, who.claims, model); err != nil {
		return err
	}
	if s.quarantined(ctx, model) {
		return fmt.Errorf("%w: model %q", quarantineError, model)
	}
	return nil
}

func (s *service) ChatTurn(ctx context.Context, bearer, model string, messages []modelrepo.Message, args ...modelrepo.ChatArgument) (modelrepo.ChatResult, error) {
	start := time.Now()
	who, err := s.callerFromBearer(ctx, bearer)
	if err != nil {
		return modelrepo.ChatResult{}, err
	}
	if err := s.authorizeCaller(ctx, who, model); err != nil {
		return modelrepo.ChatResult{}, err
	}

	resReq, health := s.chatRequest(model, sessionKeyFor(who.key, who.claims, model, llmrepo.SessionKeyFromContext(ctx), messages), messages)
	res, meta, err := s.models.Chat(ctx, resReq, messages, args...)
	health.record()
	if err != nil {
		s.recordTurnFailure(ctx, "chat", err)
		return modelrepo.ChatResult{}, sanitizeTurnError(err)
	}
	s.recordBackendSuccess(s.backendIDFor(ctx, meta.BackendID))

	var promptTokens, completionTokens, thinkingTokens, cacheReadTokens, cacheWriteTokens int64
	if res.Usage != nil {
		promptTokens = int64(res.Usage.PromptTokens)
		completionTokens = int64(res.Usage.CompletionTokens)
		thinkingTokens = int64(res.Usage.ThinkingTokens)
		cacheReadTokens = int64(res.Usage.CacheReadTokens)
		cacheWriteTokens = int64(res.Usage.CacheWriteTokens)
	}
	_, audioBytes := modelrepo.MessagesAudioBytes(messages)
	s.chargeTurn(ctx, who.key, who.claims, turnUsage{
		backend: meta.BackendID, model: model,
		prompt: promptTokens, completion: completionTokens,
		thinking:  thinkingTokens,
		cacheRead: cacheReadTokens, cacheWrite: cacheWriteTokens,
		images: int64(modelrepo.MessagesImageCount(messages)), audioBytes: int64(audioBytes),
		durationMs: time.Since(start).Milliseconds(), finishReason: res.FinishReason,
	})
	return res, nil
}

func (s *service) resolveAllowedModels(claims *liblicense.Claims) []string {
	if claims == nil {
		return nil
	}
	allowedRaw := claims.GetString("allowed_models", "")
	if allowedRaw != "" {
		var list []string
		seen := make(map[string]bool)
		for _, m := range strings.Split(allowedRaw, ",") {
			m = strings.TrimSpace(m)
			if m != "" && !seen[m] {
				seen[m] = true
				list = append(list, m)
			}
		}
		return list
	}

	def := claims.GetString("default_model", "")
	if def != "" {
		return []string{def}
	}
	return []string{"*"}
}

func (s *service) isModelAllowed(claims *liblicense.Claims, model string) bool {
	if claims == nil {
		return false
	}
	return modelAllowed(s.resolveAllowedModels(claims), model)
}

func modelAllowed(allowed []string, model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, m := range allowed {
		if m == "*" || m == model {
			return true
		}
	}
	return false
}

type optionsArg struct {
	temperature *float64
	topP        *float64
	maxTokens   *int
	seed        *int
	think       *string
	shift       *bool
	truncate    *bool
}

func (o optionsArg) Apply(cfg *modelrepo.ChatConfig) {
	if o.temperature != nil {
		cfg.Temperature = o.temperature
	}
	if o.topP != nil {
		cfg.TopP = o.topP
	}
	if o.maxTokens != nil {
		cfg.MaxTokens = o.maxTokens
	}
	if o.seed != nil {
		cfg.Seed = o.seed
	}
	if o.think != nil {
		cfg.Think = o.think
	}
	if o.shift != nil {
		cfg.Shift = o.shift
	}
	if o.truncate != nil {
		cfg.Truncate = o.truncate
	}
}

func toModelArgs(req ChatRequest) []modelrepo.ChatArgument {
	o := optionsArg{}
	if req.Options != nil {
		if v, ok := req.Options["temperature"].(float64); ok {
			o.temperature = &v
		}
		if v, ok := req.Options["top_p"].(float64); ok {
			o.topP = &v
		}
		if v, ok := req.Options["seed"].(float64); ok {
			s := int(v)
			o.seed = &s
		}
		if v, ok := req.Options["num_predict"].(float64); ok {
			n := int(v)
			o.maxTokens = &n
		}
	}
	if req.Think != nil {
		switch v := req.Think.Value.(type) {
		case string:
			o.think = &v
		case bool:
			s := "auto"
			if !v {
				s = "off"
			}
			o.think = &s
		}
	}
	o.shift = req.Shift
	o.truncate = req.Truncate
	return []modelrepo.ChatArgument{o}
}

func toModelMessages(msgs []ChatMessage) ([]modelrepo.Message, error) {
	out := make([]modelrepo.Message, 0, len(msgs))
	for _, m := range msgs {
		audio, err := toModelAudio(m.Audios)
		if err != nil {
			return nil, err
		}
		out = append(out, modelrepo.Message{
			Role:       m.Role,
			Content:    m.Content,
			Thinking:   m.Thinking,
			ToolCallID: m.ToolCallID,
			ToolCalls:  toModelToolCalls(m.ToolCalls),
			Images:     toModelImages(m.Images),
			Audio:      audio,
		})
	}
	return out, nil
}

func toModelToolCalls(calls []ToolCall) []modelrepo.ToolCall {
	out := make([]modelrepo.ToolCall, 0, len(calls))
	for _, c := range calls {
		args := ""
		if c.Function.Arguments != nil {
			if b, err := json.Marshal(c.Function.Arguments); err == nil {
				args = string(b)
			}
		}
		tc := modelrepo.ToolCall{ID: c.ID, Type: c.Type}
		tc.Function.Name = c.Function.Name
		tc.Function.Arguments = args
		out = append(out, tc)
	}
	return out
}

func toModelAudio(audios []string) ([]modelrepo.AudioPart, error) {
	if len(audios) == 0 {
		return nil, nil
	}
	out := make([]modelrepo.AudioPart, 0, len(audios))
	for _, audio := range audios {
		raw := audio
		if i := strings.Index(raw, ","); i >= 0 {
			raw = raw[i+1:]
		}
		data, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, fmt.Errorf("audios must be base64-encoded WAV bytes: %w", err)
		}
		if !modelrepo.IsWAV(data) {
			return nil, fmt.Errorf("%w: an audio attachment is not WAV", modelrepo.ErrUnsupportedAudioMime)
		}
		out = append(out, modelrepo.AudioPart{Data: data, MimeType: modelrepo.WAVMimeType})
	}
	return out, nil
}

func toModelImages(images []string) []modelrepo.ImagePart {
	out := make([]modelrepo.ImagePart, 0, len(images))
	for _, img := range images {
		raw := img
		if i := strings.Index(raw, ","); i >= 0 {
			raw = raw[i+1:] // strip a "data:<mime>;base64," prefix if present
		}
		if data, err := base64.StdEncoding.DecodeString(raw); err == nil {
			out = append(out, modelrepo.ImagePart{Data: data})
		}
	}
	return out
}

func parcelToChatResponse(model string, p *modelrepo.StreamParcel) ChatResponse {
	resp := ChatResponse{
		Model:     model,
		CreatedAt: time.Now().UTC(),
	}
	switch {
	case p.Data != "":
		resp.Message = ChatMessage{Role: "assistant", Content: p.Data}
	case p.Thinking != "":
		resp.Message = ChatMessage{Role: "assistant", Thinking: p.Thinking}
	case p.Usage != nil:
		resp.PromptEvalCount = p.Usage.PromptTokens
		resp.EvalCount = p.Usage.CompletionTokens
	case p.Terminal != nil:
		resp.Done = true
		resp.DoneReason = p.Terminal.FinishReason
		if p.Terminal.Usage != nil {
			resp.PromptEvalCount = p.Terminal.Usage.PromptTokens
			resp.EvalCount = p.Terminal.Usage.CompletionTokens
		}
	case p.Error != nil:
		resp.Done = true
		resp.DoneReason = "error"
		resp.Error = p.Error.Error()
	}
	return resp
}

func parcelToGenerateResponse(model string, p *modelrepo.StreamParcel) GenerateResponse {
	c := parcelToChatResponse(model, p)
	return GenerateResponse{
		Model:           c.Model,
		CreatedAt:       c.CreatedAt,
		Response:        c.Message.Content,
		Thinking:        c.Message.Thinking,
		Done:            c.Done,
		DoneReason:      c.DoneReason,
		Error:           c.Error,
		PromptEvalCount: c.PromptEvalCount,
		EvalCount:       c.EvalCount,
	}
}

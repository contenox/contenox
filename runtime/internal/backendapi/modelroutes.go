package backendapi

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	apiframework "github.com/contenox/runtime/apiframework"
	"github.com/contenox/runtime/runtime/stateservice"
	"github.com/contenox/runtime/runtime/statetype"
)

func AddModelRoutes(mux *http.ServeMux, stateService stateservice.Service, defaults stateservice.RuntimeDefaults) {
	s := &service{stateService: stateService, defaults: defaults}

	mux.HandleFunc("GET /openai/v1/models", s.listModels)
	mux.HandleFunc("GET /openai/{chainID}/v1/models", s.listModels)
	mux.HandleFunc("GET /models", s.listInternal)
}

type service struct {
	stateService stateservice.Service
	defaults     stateservice.RuntimeDefaults
}

type ObservedModel struct {
	ID            string `json:"id" example:"mistral:instruct"`
	Model         string `json:"model" example:"mistral:instruct"`
	ContextLength int    `json:"contextLength" example:"8192"`
	CanChat       bool   `json:"canChat" example:"true"`
	CanEmbed      bool   `json:"canEmbed" example:"false"`
	CanPrompt     bool   `json:"canPrompt" example:"true"`
	CanStream     bool   `json:"canStream" example:"true"`
}

type OpenAIModel struct {
	ID      string `json:"id" example:"mistral:latest"`
	Object  string `json:"object" example:"mistral:latest"`
	Created int64  `json:"created" example:"1717020800"`
	OwnedBy string `json:"owned_by" example:"system"`
}

type OpenAICompatibleModelList struct {
	Object string        `json:"object" example:"list"`
	Data   []OpenAIModel `json:"data"`
}

// listModels returns the models observed across the runtime's backends in the
// OpenAI-compatible model-list shape.
func (s *service) listModels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limitStr := apiframework.GetQueryParam(r, "limit", "100", "The maximum number of items to return per page.")
	_ = apiframework.GetPathParam(r, "chainID", "The ID of the chain that links to the openAI completion API. Currently unused.")
	limit, err := parseObservedModelLimit(limitStr)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	internalModels, err := s.stateService.Get(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	defaults := stateservice.ResolveRuntimeDefaults(ctx, s.stateService, s.defaults)
	openAIModels := OpenAIModelsFromObserved(ListObservedModels(internalModels), defaults.Model, time.Now().Unix())
	if limit < len(openAIModels) {
		openAIModels = openAIModels[:limit]
	}

	response := OpenAICompatibleModelList{
		Object: "list",
		Data:   openAIModels,
	}

	_ = apiframework.Encode(w, r, http.StatusOK, response) // @response backendapi.OpenAICompatibleModelList
}

// listInternal returns the models currently observed on the runtime's
// backends, with per-model capability flags.
func (s *service) listInternal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limitStr := apiframework.GetQueryParam(r, "limit", "100", "The maximum number of items to return per page.")
	limit, err := parseObservedModelLimit(limitStr)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	states, err := s.stateService.Get(ctx)
	if err != nil {
		_ = apiframework.Error(w, r, err, apiframework.ListOperation)
		return
	}

	models := ListObservedModels(states)
	if limit < len(models) {
		models = models[:limit]
	}

	_ = apiframework.Encode(w, r, http.StatusOK, models) // @response []backendapi.ObservedModel
}

func parseObservedModelLimit(limitStr string) (int, error) {
	limit := 100
	if limitStr == "" {
		return limit, nil
	}

	i, err := strconv.Atoi(limitStr)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid limit format, expected integer", apiframework.ErrUnprocessableEntity)
	}
	if i < 1 {
		return 0, fmt.Errorf("%w: limit must be positive", apiframework.ErrUnprocessableEntity)
	}
	return i, nil
}

func ListObservedModels(states []statetype.BackendRuntimeState) []ObservedModel {
	byName := map[string]ObservedModel{}

	for _, state := range sanitizeRuntimeStates(states) {
		for _, pulled := range state.PulledModels {
			name := strings.TrimSpace(pulled.Model)
			if name == "" {
				name = strings.TrimSpace(pulled.Name)
			}
			if name == "" {
				continue
			}

			model := byName[name]
			if model.ID == "" {
				model = ObservedModel{
					ID:    name,
					Model: name,
				}
			}

			if pulled.ContextLength > model.ContextLength {
				model.ContextLength = pulled.ContextLength
			}
			model.CanChat = model.CanChat || pulled.CanChat
			model.CanEmbed = model.CanEmbed || pulled.CanEmbed
			model.CanPrompt = model.CanPrompt || pulled.CanPrompt
			model.CanStream = model.CanStream || pulled.CanStream

			byName[name] = model
		}
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	models := make([]ObservedModel, 0, len(names))
	for _, name := range names {
		models = append(models, byName[name])
	}
	return models
}

func OpenAIModelsFromObserved(observed []ObservedModel, defaultModel string, created int64) []OpenAIModel {
	seen := map[string]struct{}{}
	models := make([]OpenAIModel, 0, len(observed)+2)
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		models = append(models, OpenAIModel{
			ID:      id,
			Object:  "model",
			Created: created,
			OwnedBy: "runtime",
		})
	}

	if strings.TrimSpace(defaultModel) != "" {
		add("default")
		add(defaultModel)
	}
	for _, model := range observed {
		add(model.Model)
	}
	return models
}

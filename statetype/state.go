package statetype

import (
	"time"

	"github.com/contenox/runtime/runtimetypes"
	"github.com/ollama/ollama/api"
)

// LLMState represents the observed state of a single LLM backend.
type LLMState struct {
	ID           string               `json:"id" example:"backend1"`
	Name         string               `json:"name" example:"Backend Name"`
	Models       []string             `json:"models"`
	PulledModels []ListModelResponse  `json:"pulledModels" oapiinclude:"statetype.ListModelResponse"`
	Backend      runtimetypes.Backend `json:"backend"`
	// Error stores a description of the last encountered error when
	// interacting with or reconciling this backend's state, if any.
	Error string `json:"error,omitempty"`
	// APIKey stores the API key used for authentication with the backend.
	apiKey string
}

type ListModelResponse struct {
	Name          string       `json:"name"`
	Model         string       `json:"model"`
	ModifiedAt    time.Time    `json:"modifiedAt"`
	Size          int64        `json:"size"`
	Digest        string       `json:"digest"`
	Details       ModelDetails `json:"details" oapiinclude:"statetype.ModelDetails"`
	ContextLength int          `json:"contextLength"`
	CanChat       bool         `json:"canChat"`
	CanEmbed      bool         `json:"canEmbed"`
	CanPrompt     bool         `json:"canPrompt"`
	CanStream     bool         `json:"canStream"`
}

type ModelDetails struct {
	ParentModel       string   `json:"parentModel"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameterSize"`
	QuantizationLevel string   `json:"quantizationLevel"`
}

func (s *LLMState) GetAPIKey() string {
	return s.apiKey
}

func (s *LLMState) SetAPIKey(key string) {
	s.apiKey = key
}

func ConvertOllamaModelResponse(model *api.ListModelResponse) *ListModelResponse {
	list := &ListModelResponse{
		Name:       model.Name,
		Model:      model.Model,
		ModifiedAt: model.ModifiedAt,
		Size:       model.Size,
		Digest:     model.Digest,
		Details: ModelDetails{
			ParentModel:       model.Details.ParentModel,
			Format:            model.Details.Format,
			Family:            model.Details.Family,
			Families:          model.Details.Families,
			ParameterSize:     model.Details.ParameterSize,
			QuantizationLevel: model.Details.QuantizationLevel,
		},
	}
	return list
}

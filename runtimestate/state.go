// runtimestate implements the core logic for reconciling the declared state
// of LLM backends (from dbInstance) with their actual observed state.
// It provides the functionality for synchronizing models and processing downloads,
// intended to be executed repeatedly within background tasks managed externally.
package runtimestate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	libbus "github.com/contenox/bus"
	libdb "github.com/contenox/dbexec"
	"github.com/contenox/runtime/runtimetypes"
	"github.com/ollama/ollama/api"
)

// LLMState represents the observed state of a single LLM backend.
type LLMState struct {
	ID           string               `json:"id" example:"backend1"`
	Name         string               `json:"name" example:"Backend Name"`
	Models       []string             `json:"models"`
	PulledModels []ListModelResponse  `json:"pulledModels" oapiinclude:"runtimestate.ListModelResponse"`
	Backend      runtimetypes.Backend `json:"backend"`
	// Error stores a description of the last encountered error when
	// interacting with or reconciling this backend's state, if any.
	Error string `json:"error,omitempty"`
	// APIKey stores the API key used for authentication with the backend.
	apiKey string
}

type ListModelResponse struct {
	Name       string       `json:"name"`
	Model      string       `json:"model"`
	ModifiedAt time.Time    `json:"modified_at"`
	Size       int64        `json:"size"`
	Digest     string       `json:"digest"`
	Details    ModelDetails `json:"details,omitempty" oapiinclude:"runtimestate.ModelDetails"`
}

type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

func (s *LLMState) GetAPIKey() string {
	return s.apiKey
}

func convertOllamaModelResponse(model *api.ListModelResponse) *ListModelResponse {
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

func convertOllamaListModelResponse(models []api.ListModelResponse) []ListModelResponse {
	list := make([]ListModelResponse, len(models))
	for i, model := range models {
		list[i] = *convertOllamaModelResponse(&model)
	}
	return list
}

// State manages the overall runtime status of multiple LLM backends.
// It orchestrates the synchronization between the desired configuration
// and the actual state of the backends, including providing the mechanism
// for model downloads via the dwqueue component.
type State struct {
	dbInstance libdb.DBManager
	state      sync.Map
	psInstance libbus.Messenger
	dwQueue    dwqueue
	withPools  bool
}

type Option func(*State)

func WithPools() Option {
	return func(s *State) {
		s.withPools = true
	}
}

// New creates and initializes a new State manager.
// It requires a database manager (dbInstance) to load the desired configurations
// and a messenger instance (psInstance) for event handling and progress updates.
// Options allow enabling experimental features like pool-based reconciliation.
// Returns an initialized State ready for use.
func New(ctx context.Context, dbInstance libdb.DBManager, psInstance libbus.Messenger, options ...Option) (*State, error) {
	s := &State{
		dbInstance: dbInstance,
		state:      sync.Map{},
		dwQueue:    dwqueue{dbInstance: dbInstance},
		psInstance: psInstance,
	}
	if psInstance == nil {
		return nil, errors.New("psInstance cannot be nil")
	}
	if dbInstance == nil {
		return nil, errors.New("dbInstance cannot be nil")
	}
	// Apply options to configure the State instance
	for _, option := range options {
		option(s)
	}
	return s, nil
}

// RunBackendCycle performs a single reconciliation check for all configured LLM backends.
// It compares the desired state (from configuration) with the observed state
// (by communicating with the backends) and schedules necessary actions,
// such as queuing model downloads or removals, to align them.
// This method should be called periodically in a background process.
// DESIGN NOTE: This method executes one complete reconciliation cycle and then returns.
// It does not manage its own background execution (e.g., via internal goroutines or timers).
// This deliberate design choice delegates execution management (scheduling, concurrency control,
// lifecycle via context, error handling, circuit breaking, etc.) entirely to the caller.
//
// Consequently, this method should be called periodically by an external process
// responsible for its scheduling and lifecycle.
// When the pool feature is enabled via WithPools option, it uses pool-aware reconciliation.
func (s *State) RunBackendCycle(ctx context.Context) error {
	if s.withPools {
		return s.syncBackendsWithPools(ctx)
	}
	return s.syncBackends(ctx)
}

// RunDownloadCycle processes a single pending model download operation, if one exists.
// It retrieves the next download task, executes the download while providing
// progress updates, and handles potential cancellation requests.
// If no download tasks are queued, it returns nil immediately.
// This method should be called periodically in a background process to
// drain the download queue.
// DESIGN NOTE: this method performs one unit of work
// and returns. The caller is responsible for the execution loop, allowing
// flexible integration with task management strategies.
//
// This method should be called periodically by an external process to
// drain the download queue.
func (s *State) RunDownloadCycle(ctx context.Context) error {
	item, err := s.dwQueue.pop(ctx)
	if err != nil {
		if err == libdb.ErrNotFound {
			return nil
		}
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel() // clean up the context when done

	done := make(chan struct{})

	ch := make(chan []byte, 16)
	sub, err := s.psInstance.Stream(ctx, "queue_cancel", ch)
	if err != nil {
		// log.Println("Error subscribing to queue_cancel:", err)
		return nil
	}
	go func() {
		defer func() {
			sub.Unsubscribe()
			close(done)
		}()
		for {
			select {
			case data, ok := <-ch:
				if !ok {
					return
				}
				var queueItem runtimetypes.Job
				if err := json.Unmarshal(data, &queueItem); err != nil {
					// log.Println("Error unmarshalling cancel message:", err)
					continue
				}
				// Check if the cancellation request matches the current download task.
				// Rationale: Matching logic based on URL to target a specific backend
				// or Model ID to purge a model from all backends, if it is currently downloading.
				if queueItem.ID == item.URL || queueItem.ID == item.Model {
					cancel()
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// log.Printf("Processing download job: %+v", item)
	err = s.dwQueue.downloadModel(ctx, *item, func(status runtimetypes.Status) error {
		// //log.Printf("Download progress for model %s: %+v", item.Model, status)
		message, _ := json.Marshal(status)
		return s.psInstance.Publish(ctx, "model_download", message)
	})
	if err != nil {
		return fmt.Errorf("failed downloading model %s: %w", item.Model, err)
	}

	cancel()
	<-done

	return nil
}

// Get returns a copy of the current observed state for all backends.
// This provides a safe snapshot for reading state without risking modification
// of the internal structures.
func (s *State) Get(ctx context.Context) map[string]LLMState {
	state := map[string]LLMState{}
	s.state.Range(func(key, value any) bool {
		backend, ok := value.(*LLMState)
		if !ok {
			// log.Printf("invalid type in state: %T", value)
			return true
		}
		var backendCopy LLMState
		raw, err := json.Marshal(backend)
		if err != nil {
			// log.Printf("failed to marshal backend: %v", err)
		}
		err = json.Unmarshal(raw, &backendCopy)
		if err != nil {
			// log.Printf("failed to unmarshal backend: %v", err)
		}
		backendCopy.apiKey = backend.apiKey
		state[backend.ID] = backendCopy
		return true
	})
	return state
}

// cleanupStaleBackends removes state entries for backends not present in currentIDs.
// It performs type checking on state keys and logs errors for invalid key types.
// This centralizes the state cleanup logic used by all reconciliation flows.
func (s *State) cleanupStaleBackends(currentIDs map[string]struct{}) error {
	var err error
	s.state.Range(func(key, value any) bool {
		id, ok := key.(string)
		if !ok {
			err = fmt.Errorf("BUG: invalid key type: %T %v", key, key)
			// log.Printf("BUG: %v", err)
			return true
		}
		if _, exists := currentIDs[id]; !exists {
			s.state.Delete(id)
		}
		return true
	})
	return err
}

// syncBackendsWithPools is the pool-aware reconciliation logic called by RunBackendCycle.
// It:
//  1. Fetches all configured pools from the database.
//  2. For each pool:
//     a. Retrieves its associated backends and models.
//     b. Aggregates models for each backend, collecting a unique set of all models
//     that a backend should have based on all pools it belongs to.
//     c. Tracks all active backend IDs encountered.
//  3. After processing all pools and aggregating models:
//     a. For each unique backend, processes it once with its complete aggregated set of models.
//  4. Performs global cleanup of state entries for backends not found in any pool (those not
//     associated with any pool).
//
// This fixed version aggregates backend IDs across all pools before cleanup to prevent
// premature deletion of valid cross-pool backends.
func (s *State) syncBackendsWithPools(ctx context.Context) error {
	tx := s.dbInstance.WithoutTransaction()
	dbStore := runtimetypes.New(tx)

	allPools, err := dbStore.ListAllPools(ctx)
	if err != nil {
		return fmt.Errorf("fetching pools: %v", err)
	}

	allBackendObjects := make(map[string]*runtimetypes.Backend)
	backendToAggregatedModels := make(map[string]map[string]*runtimetypes.Model)
	activeBackendIDs := make(map[string]struct{})

	for _, pool := range allPools {
		poolBackends, err := dbStore.ListBackendsForPool(ctx, pool.ID)
		if err != nil {
			return fmt.Errorf("fetching backends for pool %s: %v", pool.ID, err)
		}

		poolModels, err := dbStore.ListModelsForPool(ctx, pool.ID)
		if err != nil {
			return fmt.Errorf("fetching models for pool %s: %v", pool.ID, err)
		}

		for _, backend := range poolBackends {
			activeBackendIDs[backend.ID] = struct{}{}
			if _, exists := allBackendObjects[backend.ID]; !exists {
				allBackendObjects[backend.ID] = backend
			}
			if _, exists := backendToAggregatedModels[backend.ID]; !exists {
				backendToAggregatedModels[backend.ID] = make(map[string]*runtimetypes.Model)
			}
			for _, model := range poolModels {
				backendToAggregatedModels[backend.ID][model.Model] = model
			}
		}
	}

	// Now, process each unique backend once with its fully aggregated list of models.
	for backendID, backendObj := range allBackendObjects {
		modelsForThisBackend := make([]*runtimetypes.Model, 0, len(backendToAggregatedModels[backendID]))
		for _, model := range backendToAggregatedModels[backendID] {
			modelsForThisBackend = append(modelsForThisBackend, model)
		}
		s.processBackend(ctx, backendObj, modelsForThisBackend)
	}

	return s.cleanupStaleBackends(activeBackendIDs)
}

// syncBackends is the global reconciliation logic called by RunBackendCycle.
// It:
// 1. Fetches all configured backends from the database
// 2. Retrieves all models regardless of pool association
// 3. Processes each backend with the full model list
// 4. Cleans up state entries for backends no longer present in the database
// This version uses the shared helper methods but maintains its original non-pool
// behavior by operating on the global backend/model lists.
func (s *State) syncBackends(ctx context.Context) error {
	tx := s.dbInstance.WithoutTransaction()
	storeInstance := runtimetypes.New(tx)

	backends, err := storeInstance.ListAllBackends(ctx)
	if err != nil {
		return fmt.Errorf("fetching backends: %v", err)
	}

	// Paginate through all models
	var allModels []*runtimetypes.Model
	var cursor *time.Time
	limit := 100 // Use a reasonable page size
	for {
		models, err := storeInstance.ListModels(ctx, cursor, limit)
		if err != nil {
			return fmt.Errorf("fetching paginated models: %v", err)
		}
		allModels = append(allModels, models...)

		// Break the loop if this is the last page
		if len(models) < limit {
			break
		}

		// Update the cursor for the next page
		lastModel := models[len(models)-1]
		cursor = &lastModel.CreatedAt
	}

	currentIDs := make(map[string]struct{})
	s.processBackends(ctx, backends, allModels, currentIDs)
	return s.cleanupStaleBackends(currentIDs)
}

// Helper method to process backends and collect their IDs
func (s *State) processBackends(ctx context.Context, backends []*runtimetypes.Backend, models []*runtimetypes.Model, currentIDs map[string]struct{}) {
	for _, backend := range backends {
		currentIDs[backend.ID] = struct{}{}
		s.processBackend(ctx, backend, models)
	}
}

// processBackend routes the backend processing logic based on the backend's Type.
// It acts as a dispatcher to type-specific handling functions (e.g., for Ollama).
// It updates the internal state map with the results of the processing,
// including any errors encountered for unsupported types.
// Helper method to process backends and collect their IDs
func (s *State) processBackend(ctx context.Context, backend *runtimetypes.Backend, declaredModels []*runtimetypes.Model) {
	switch strings.ToLower(backend.Type) {
	case "ollama":
		s.processOllamaBackend(ctx, backend, declaredModels)
	case "vllm":
		s.processVLLMBackend(ctx, backend, declaredModels)
	case "gemini":
		s.processGeminiBackend(ctx, backend, declaredModels)
	case "openai":
		s.processOpenAIBackend(ctx, backend, declaredModels)
	default:
		brokenService := &LLMState{
			ID:      backend.ID,
			Name:    backend.Name,
			Models:  []string{},
			Backend: *backend,
			Error:   "Unsupported backend type: " + backend.Type,
		}
		s.state.Store(backend.ID, brokenService)
	}
}

// processOllamaBackend handles the state reconciliation for a single Ollama backend.
// It connects to the Ollama API, compares the set of declared models for this backend
// with the models actually present on the Ollama instance, and takes corrective actions:
// - Queues downloads for declared models that are missing.
// - Initiates deletion for models present on the instance but not declared in the config.
// Finally, it updates the internal state map with the latest observed list of pulled models
// and any communication errors encountered.
func (s *State) processOllamaBackend(ctx context.Context, backend *runtimetypes.Backend, declaredOllamaModels []*runtimetypes.Model) {
	// log.Printf("Processing Ollama backend for ID %s with declared models: %+v", backend.ID, declaredOllamaModels)

	models := []string{}
	for _, model := range declaredOllamaModels {
		models = append(models, model.Model)
	}
	// log.Printf("Extracted model names for backend %s: %v", backend.ID, models)

	backendURL, err := url.Parse(backend.BaseURL)
	if err != nil {
		// log.Printf("Error parsing URL for backend %s: %v", backend.ID, err)
		stateservice := &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       models,
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        "Invalid URL: " + err.Error(),
		}
		s.state.Store(backend.ID, stateservice)
		return
	}
	// log.Printf("Parsed URL for backend %s: %s", backend.ID, backendURL.String())

	client := api.NewClient(backendURL, http.DefaultClient)
	existingModels, err := client.List(ctx)
	if err != nil {
		// log.Printf("Error listing models for backend %s: %v", backend.ID, err)
		stateservice := &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       models,
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        err.Error(),
		}
		s.state.Store(backend.ID, stateservice)
		return
	}
	// log.Printf("Existing models from backend %s: %+v", backend.ID, existingModels.Models)

	declaredModelSet := make(map[string]struct{})
	for _, declaredModel := range declaredOllamaModels {
		declaredModelSet[declaredModel.Model] = struct{}{}
	}
	// log.Printf("Declared model set for backend %s: %v", backend.ID, declaredModelSet)

	existingModelSet := make(map[string]struct{})
	for _, existingModel := range existingModels.Models {
		existingModelSet[existingModel.Model] = struct{}{}
	}
	// log.Printf("Existing model set for backend %s: %v", backend.ID, existingModelSet)

	// For each declared model missing from the backend, add a download job.
	for declaredModel := range declaredModelSet {
		if _, ok := existingModelSet[declaredModel]; !ok {
			// log.Printf("Model %s is declared but missing in backend %s. Adding to download queue.", declaredModel, backend.ID)
			// RATIONALE: Using the backend URL as the Job ID in the queue prevents
			// queueing multiple downloads for the same backend simultaneously,
			// acting as a simple lock at the queue level.
			// Download flow:
			// 1. The sync cycle re-evaluates the full desired vs. actual state
			//    periodically. It will re-detect *all* currently missing models on each run.
			// 2. Therefore, the queue doesn't need to store a "TODO" list of all
			//    pending downloads for a backend. A single job per backend URL acts as
			//    a sufficient signal that *a* download action is required.
			// 3. The specific model placed in this job's payload reflects one missing model
			//    identified during the *most recent* sync cycle run.
			// 4. When this model is downloaded, the *next* sync cycle will identify the
			//    *next* missing model (if any) and trigger the queue again, eventually
			//    leading to all models being downloaded over successive cycles.
			// 5. If the backeend dies while downloading this mechanism will ensure that
			//    the downloadjob will be readded to the queue.
			err := s.dwQueue.add(ctx, *backendURL, declaredModel)
			if err != nil {
				// log.Printf("Error adding model %s to download queue: %v", declaredModel, err)
			}
		}
	}

	// For each model in the backend that is not declared, trigger deletion.
	// NOTE: We have to delete otherwise we have keep track of not desired model in each backend to
	// ensure some backend-nodes don't just run out of space.
	for existingModel := range existingModelSet {
		if _, ok := declaredModelSet[existingModel]; !ok {
			// log.Printf("Model %s exists in backend %s but is not declared. Triggering deletion.", existingModel, backend.ID)
			err := client.Delete(ctx, &api.DeleteRequest{
				Model: existingModel,
			})
			if err != nil {
				// log.Printf("Error deleting model %s for backend %s: %v", existingModel, backend.ID, err)
			} else {
				// log.Printf("Successfully deleted model %s for backend %s", existingModel, backend.ID)
			}
		}
	}

	modelResp, err := client.List(ctx)
	if err != nil {
		// log.Printf("Error listing running models for backend %s after deletion: %v", backend.ID, err)
		stateservice := &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       models,
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        err.Error(),
		}
		s.state.Store(backend.ID, stateservice)
		return
	}
	// log.Printf("Updated model list for backend %s: %+v", backend.ID, modelResp.Models)

	stateservice := &LLMState{
		ID:           backend.ID,
		Name:         backend.Name,
		Models:       models,
		PulledModels: convertOllamaListModelResponse(modelResp.Models),
		Backend:      *backend,
	}
	s.state.Store(backend.ID, stateservice)
	// log.Printf("Stored updated state for backend %s", backend.ID)
}

// processVLLMBackend handles the state reconciliation for a single vLLM backend.
// Since vLLM instances typically serve a single model, we verify that the running model
// matches one of the models assigned to the backend through its pools.
func (s *State) processVLLMBackend(ctx context.Context, backend *runtimetypes.Backend, _ []*runtimetypes.Model) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Build models endpoint URL
	modelsURL := strings.TrimSuffix(backend.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", modelsURL, nil)
	if err != nil {
		s.state.Store(backend.ID, &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       []string{},
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        fmt.Sprintf("Failed to create request: %v", err),
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		s.state.Store(backend.ID, &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       []string{},
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        fmt.Sprintf("HTTP request failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		if readErr != nil {
			bodyStr = fmt.Sprintf("<failed to read body: %v>", readErr)
		}
		s.state.Store(backend.ID, &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       []string{},
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        fmt.Sprintf("Unexpected status: %d %s", resp.StatusCode, bodyStr),
		})
		return
	}

	var modelResp struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modelResp); err != nil {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		if readErr != nil {
			bodyStr = fmt.Sprintf("<failed to read body: %v>", readErr)
		}
		s.state.Store(backend.ID, &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       []string{},
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        fmt.Sprintf("Failed to decode response: %v | Raw response: %s", err, bodyStr),
		})
		return
	}

	if len(modelResp.Data) == 0 {
		s.state.Store(backend.ID, &LLMState{
			ID:           backend.ID,
			Name:         backend.Name,
			Models:       []string{},
			PulledModels: []ListModelResponse{},
			Backend:      *backend,
			Error:        "No models found in response",
		})
		return
	}

	servedModel := modelResp.Data[0].ID
	// Create mock PulledModels for state reporting
	pulledModels := []api.ListModelResponse{
		{
			Model: servedModel,
		},
	}

	s.state.Store(backend.ID, &LLMState{
		ID:           backend.ID,
		Name:         backend.Name,
		Models:       []string{servedModel},
		PulledModels: convertOllamaListModelResponse(pulledModels),
		Backend:      *backend,
	})
}

func (s *State) processGeminiBackend(ctx context.Context, backend *runtimetypes.Backend, _ []*runtimetypes.Model) {
	stateInstance := &LLMState{
		ID:           backend.ID,
		Name:         backend.Name,
		Backend:      *backend,
		PulledModels: []ListModelResponse{},
		apiKey:       "",
	}

	// Retrieve API key configuration
	cfg := ProviderConfig{}
	storeInstance := runtimetypes.New(s.dbInstance.WithoutTransaction())
	if err := storeInstance.GetKV(ctx, GeminiKey, &cfg); err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			stateInstance.Error = "API key not configured"
		} else {
			stateInstance.Error = fmt.Sprintf("Failed to retrieve API key configuration: %v", err)
		}
		s.state.Store(backend.ID, stateInstance)
		return
	}

	// Prepare HTTP request
	client := &http.Client{Timeout: 10 * time.Second}
	reqURL := fmt.Sprintf("%s/v1beta/models",
		backend.BaseURL,
	)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		stateInstance.Error = fmt.Sprintf("Request creation failed: %v", err)
		s.state.Store(backend.ID, stateInstance)
		return
	}

	req.Header.Set("X-Goog-Api-Key", cfg.APIKey)
	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		stateInstance.Error = fmt.Sprintf("HTTP request failed: %v", err)
		s.state.Store(backend.ID, stateInstance)
		return
	}
	defer resp.Body.Close()

	// Handle non-200 responses and read body for debugging
	if resp.StatusCode != http.StatusOK {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		if readErr != nil {
			bodyStr = fmt.Sprintf("<failed to read body: %v>", readErr)
		}
		stateInstance.Error = fmt.Sprintf("API returned %d: %s", resp.StatusCode, bodyStr)
		s.state.Store(backend.ID, stateInstance)
		return
	}

	// Parse response
	var geminiResponse struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&geminiResponse); err != nil {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		bodyStr := string(bodyBytes)
		if readErr != nil {
			bodyStr = fmt.Sprintf("<failed to read body: %v>", readErr)
		}
		stateInstance.Error = fmt.Sprintf("Response parsing failed: %v | Raw response: %s", err, bodyStr)
		s.state.Store(backend.ID, stateInstance)
		return
	}

	modelNames := make([]string, 0, len(geminiResponse.Models))
	pulledModels := make([]api.ListModelResponse, 0, len(geminiResponse.Models))
	for _, m := range geminiResponse.Models {
		modelNames = append(modelNames, m.Name)
		pulledModels = append(pulledModels, api.ListModelResponse{
			Name:  m.Name,
			Model: m.Name,
		})
	}

	// Update state
	stateInstance.Models = modelNames
	stateInstance.PulledModels = convertOllamaListModelResponse(pulledModels)
	stateInstance.apiKey = cfg.APIKey
	s.state.Store(backend.ID, stateInstance)
}

func (s *State) processOpenAIBackend(ctx context.Context, backend *runtimetypes.Backend, _ []*runtimetypes.Model) {
	stateInstance := &LLMState{
		ID:           backend.ID,
		Name:         backend.Name,
		PulledModels: []ListModelResponse{},
		Backend:      *backend,
	}

	// Retrieve API key configuration
	cfg := ProviderConfig{}
	storeInstance := runtimetypes.New(s.dbInstance.WithoutTransaction())
	if err := storeInstance.GetKV(ctx, OpenaiKey, &cfg); err != nil {
		if errors.Is(err, libdb.ErrNotFound) {
			stateInstance.Error = "API key not configured"
		} else {
			stateInstance.Error = fmt.Sprintf("Failed to retrieve API key configuration: %v", err)
		}
		s.state.Store(backend.ID, stateInstance)
		return
	}

	// Prepare HTTP request
	client := &http.Client{Timeout: 10 * time.Second}
	reqURL := strings.TrimSuffix(backend.BaseURL, "/") + "/v1/models"
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		stateInstance.Error = fmt.Sprintf("Request creation failed: %v", err)
		s.state.Store(backend.ID, stateInstance)
		return
	}
	req.Header.Set("Authorization", "Bearer "+cfg.APIKey)

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		stateInstance.Error = fmt.Sprintf("HTTP request failed: %v", err)
		s.state.Store(backend.ID, stateInstance)
		return
	}
	defer resp.Body.Close()

	// Handle non-200 responses
	if resp.StatusCode != http.StatusOK {
		stateInstance.Error = fmt.Sprintf("API returned %d", resp.StatusCode)
		s.state.Store(backend.ID, stateInstance)
		return
	}

	// Parse response
	var openAIResponse struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&openAIResponse); err != nil {
		stateInstance.Error = fmt.Sprintf("Response parsing failed: %v", err)
		s.state.Store(backend.ID, stateInstance)
		return
	}

	// Extract model names
	modelNames := make([]string, 0, len(openAIResponse.Data))
	pulledModels := make([]api.ListModelResponse, 0, len(openAIResponse.Data))
	for _, m := range openAIResponse.Data {
		modelNames = append(modelNames, m.ID)
	}
	for _, model := range openAIResponse.Data {
		pulledModels = append(pulledModels, api.ListModelResponse{
			Model: model.ID,
			Name:  model.ID,
		})
	}

	// Update state
	stateInstance.Models = modelNames
	stateInstance.PulledModels = convertOllamaListModelResponse(pulledModels)
	stateInstance.apiKey = cfg.APIKey
	s.state.Store(backend.ID, stateInstance)
}

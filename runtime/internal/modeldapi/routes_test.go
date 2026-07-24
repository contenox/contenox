package modeldapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/internal/modeldprobe"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/contenox/runtime/runtime/statetype"
	"github.com/contenox/runtime/runtime/transport"
)

type fakeStatusProvider struct {
	detected     modeldprobe.Status
	daemon       transport.DaemonStatus
	statusErr    error
	statusCalled bool
	unloadGen    uint64
	unloadCalled bool
	unloadErr    error
	loadRef      modeldconn.ModelRef
	loadCfg      transport.Config
	loadGen      uint64
	loadCalled   bool
	loadActive   transport.ActiveModel
	loadErr      error
	describeRef  modeldconn.ModelRef
	describeCfg  transport.Config
	describeInfo transport.ModelInfo
	describeErr  error
}

func (f *fakeStatusProvider) Detect(context.Context) modeldprobe.Status {
	return f.detected
}

func (f *fakeStatusProvider) Status(context.Context) (transport.DaemonStatus, error) {
	f.statusCalled = true
	return f.daemon, f.statusErr
}

func (f *fakeStatusProvider) UnloadModel(_ context.Context, expectedGeneration uint64) error {
	f.unloadCalled = true
	f.unloadGen = expectedGeneration
	return f.unloadErr
}

func (f *fakeStatusProvider) LoadModel(_ context.Context, ref modeldconn.ModelRef, cfg transport.Config, expectedGeneration uint64) (transport.ActiveModel, error) {
	f.loadCalled = true
	f.loadRef = ref
	f.loadCfg = cfg
	f.loadGen = expectedGeneration
	return f.loadActive, f.loadErr
}

func (f *fakeStatusProvider) Describe(_ context.Context, ref modeldconn.ModelRef, cfg transport.Config) (transport.ModelInfo, error) {
	f.describeRef = ref
	f.describeCfg = cfg
	return f.describeInfo, f.describeErr
}

type fakeStateReader struct {
	states []statetype.BackendRuntimeState
	err    error
}

func (f fakeStateReader) Get(context.Context) ([]statetype.BackendRuntimeState, error) {
	return f.states, f.err
}

func TestStatusReturnsSanitizedActiveModel(t *testing.T) {
	provider := &fakeStatusProvider{
		detected: modeldprobe.Status{
			State:    modeldprobe.StateRunning,
			Binary:   "/bin/modeld",
			Endpoint: "127.0.0.1:4000",
			Instance: "owner-1",
			Backend:  "llama",
		},
		daemon: transport.DaemonStatus{
			OwnerInstanceID: "owner-1",
			Backend:         "llama",
			State:           transport.SlotReady,
			Active: &transport.ActiveModel{
				ModelName:  "qwen3-8b",
				Type:       "llama",
				Digest:     "sha256:abc",
				Path:       "/private/models/qwen3-8b/model.gguf",
				Config:     transport.Config{NumCtx: 8192, HotContextTokens: 4096},
				Generation: 7,
			},
		},
	}
	mux := http.NewServeMux()
	addRoutesWithProvider(mux, provider)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/status", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "/private/models") {
		t.Fatalf("status response leaked active model path: %s", rr.Body.String())
	}

	var got StatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !provider.statusCalled {
		t.Fatal("expected daemon status to be queried")
	}
	if !got.Available || got.State != "running" || got.Slot == nil || got.Slot.Active == nil {
		t.Fatalf("unexpected status response: %#v", got)
	}
	if got.Slot.Active.ModelName != "qwen3-8b" || got.Slot.Active.Generation != 7 {
		t.Fatalf("unexpected active model: %#v", got.Slot.Active)
	}
	if got.Slot.Active.Config.NumCtx != 8192 || got.Slot.Active.Config.HotContextTokens != 4096 {
		t.Fatalf("unexpected active config: %#v", got.Slot.Active.Config)
	}
}

func TestUnloadRequiresExpectedGeneration(t *testing.T) {
	provider := &fakeStatusProvider{}
	mux := http.NewServeMux()
	addRoutesWithProvider(mux, provider)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/modeld/unload", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
	if provider.unloadCalled {
		t.Fatal("unload should not be called without expectedGeneration")
	}
}

func TestUnloadCallsModeldWithExpectedGeneration(t *testing.T) {
	provider := &fakeStatusProvider{}
	mux := http.NewServeMux()
	addRoutesWithProvider(mux, provider)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/modeld/unload", strings.NewReader(`{"expectedGeneration":7}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !provider.unloadCalled || provider.unloadGen != 7 {
		t.Fatalf("unexpected unload call: called=%v gen=%d", provider.unloadCalled, provider.unloadGen)
	}
}

func TestLoadResolvesLocalModelWithAdaptersAndSanitizesResponse(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(modelDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterBytes := []byte("fake adapter")
	adapterPath := filepath.Join(modelDir, "style.gguf")
	if err := os.WriteFile(adapterPath, adapterBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "contenox-llama.json"), []byte(`{
		"runtime":{"num_ctx":4096},
		"adapters":[{"name":"style","path":"style.gguf","scale":2}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterSum := sha256.Sum256(adapterBytes)
	adapterDigest := hex.EncodeToString(adapterSum[:])
	provider := &fakeStatusProvider{
		loadActive: transport.ActiveModel{
			ModelName: "coder",
			Type:      "llama",
			Digest:    "model-digest",
			Path:      modelPath,
			Adapters: []transport.AdapterSpec{{
				Name:   "style",
				Path:   adapterPath,
				Digest: adapterDigest,
				Scale:  2,
			}},
			Config:     transport.Config{NumCtx: 4096},
			Generation: 8,
		},
	}
	state := fakeStateReader{states: []statetype.BackendRuntimeState{localRuntimeState(root, "coder", "llama")}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/modeld/load", strings.NewReader(`{"model":"llama:coder","expectedGeneration":7}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !provider.loadCalled || provider.loadGen != 7 {
		t.Fatalf("unexpected load call: called=%v gen=%d", provider.loadCalled, provider.loadGen)
	}
	if provider.loadRef.Name != "coder" || provider.loadRef.Type != "llama" || provider.loadRef.Path != modelPath {
		t.Fatalf("unexpected load ref: %#v", provider.loadRef)
	}
	if len(provider.loadRef.Adapters) != 1 {
		t.Fatalf("expected one adapter in load ref, got %#v", provider.loadRef.Adapters)
	}
	adapter := provider.loadRef.Adapters[0]
	if adapter.Name != "style" || adapter.Path != adapterPath || adapter.Digest != adapterDigest || adapter.Scale != 2 {
		t.Fatalf("unexpected adapter ref: %#v", adapter)
	}
	if provider.loadCfg.NumCtx != 4096 {
		t.Fatalf("unexpected load config: %#v", provider.loadCfg)
	}
	if strings.Contains(rr.Body.String(), root) || strings.Contains(rr.Body.String(), adapterPath) {
		t.Fatalf("load response leaked local paths: %s", rr.Body.String())
	}
	var got LoadResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Loaded || got.Active.Generation != 8 || len(got.Active.Adapters) != 1 || got.Active.Adapters[0].Digest != adapterDigest {
		t.Fatalf("unexpected load response: %#v", got)
	}
}

func TestLoadResolvesLlamaVariantAgainstBaseWeights(t *testing.T) {
	root := t.TempDir()
	// Base model with real weights the variant reuses in place.
	baseDir := filepath.Join(root, "qwen3-coder-8b")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	baseModelPath := filepath.Join(baseDir, "model.gguf")
	if err := os.WriteFile(baseModelPath, []byte("fake gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "contenox-llama.json"), []byte(`{"runtime":{"num_ctx":4096}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Variant marker in a sibling directory, no duplicated weights.
	variantDir := filepath.Join(root, "qwen3-coder-8b-acme")
	if err := os.MkdirAll(variantDir, 0o755); err != nil {
		t.Fatal(err)
	}
	adapterBytes := []byte("fake adapter")
	adapterPath := filepath.Join(variantDir, "acme.gguf")
	if err := os.WriteFile(adapterPath, adapterBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(variantDir, "contenox-variant.json"), []byte(`{
		"name":"qwen3-coder-8b-acme",
		"backend":"llama",
		"base_model":"qwen3-coder-8b",
		"adapters":[{"name":"acme","path":"acme.gguf","scale":2}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterSum := sha256.Sum256(adapterBytes)
	adapterDigest := hex.EncodeToString(adapterSum[:])

	provider := &fakeStatusProvider{
		loadActive: transport.ActiveModel{
			ModelName:  "qwen3-coder-8b-acme",
			Type:       "llama",
			Digest:     "model-digest",
			Path:       baseModelPath,
			Generation: 3,
		},
	}
	// The variant is a selectable pulled model in the runtime state.
	state := fakeStateReader{states: []statetype.BackendRuntimeState{{
		ID:   "backend-llama",
		Name: "llama",
		Backend: runtimetypes.Backend{
			ID:      "backend-llama",
			Name:    "llama",
			Type:    "llama",
			BaseURL: root,
		},
		PulledModels: []statetype.ModelPullStatus{{
			Name:    "qwen3-coder-8b-acme",
			Model:   "qwen3-coder-8b-acme",
			CanChat: true,
		}},
	}}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/modeld/load", strings.NewReader(`{"model":"llama:qwen3-coder-8b-acme"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// The variant loads the BASE model.gguf (weights reused, never duplicated)
	// with the variant's adapter attached — identity = base weights + adapter.
	if provider.loadRef.Name != "qwen3-coder-8b-acme" || provider.loadRef.Path != baseModelPath {
		t.Fatalf("variant load ref should target base weights: %#v", provider.loadRef)
	}
	if len(provider.loadRef.Adapters) != 1 {
		t.Fatalf("expected one adapter in variant load ref, got %#v", provider.loadRef.Adapters)
	}
	adapter := provider.loadRef.Adapters[0]
	if adapter.Name != "acme" || adapter.Path != adapterPath || adapter.Digest != adapterDigest || adapter.Scale != 2 {
		t.Fatalf("unexpected variant adapter ref: %#v", adapter)
	}
	// Variant inherits the base model's runtime profile (num_ctx here).
	if provider.loadCfg.NumCtx != 4096 {
		t.Fatalf("variant should inherit base runtime profile, got %#v", provider.loadCfg)
	}
}

func TestLoadResolvesOpenVINOVariantAgainstBaseIR(t *testing.T) {
	root := t.TempDir()
	// Base OpenVINO IR the variant reuses in place: entrypoint + the config files
	// openvinoIdentity content-addresses.
	baseDir := filepath.Join(root, "qwen3-8b-ov")
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"openvino_language_model.xml": "<xml/>",
		"config.json":                 `{"max_position_embeddings":32768}`,
		"tokenizer_config.json":       `{"chat_template":"{{ messages }}"}`,
		"generation_config.json":      `{"eos_token_id":1}`,
		"contenox-openvino.json":      `{"context_length":4096}`,
	} {
		if err := os.WriteFile(filepath.Join(baseDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Variant marker in a sibling directory, no duplicated IR.
	variantDir := filepath.Join(root, "qwen3-8b-ov-acme")
	if err := os.MkdirAll(variantDir, 0o755); err != nil {
		t.Fatal(err)
	}
	adapterBytes := []byte("fake safetensors adapter")
	adapterPath := filepath.Join(variantDir, "acme.safetensors")
	if err := os.WriteFile(adapterPath, adapterBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(variantDir, "contenox-variant.json"), []byte(`{
		"name":"qwen3-8b-ov-acme",
		"backend":"openvino",
		"base_model":"qwen3-8b-ov",
		"adapters":[{"name":"acme","path":"acme.safetensors","scale":2}]
	}`), 0o644); err != nil {
		t.Fatal(err)
	}
	adapterSum := sha256.Sum256(adapterBytes)
	adapterDigest := hex.EncodeToString(adapterSum[:])

	provider := &fakeStatusProvider{
		loadActive: transport.ActiveModel{
			ModelName:  "qwen3-8b-ov-acme",
			Type:       "openvino",
			Digest:     "model-digest",
			Path:       baseDir,
			Generation: 3,
		},
	}
	// The variant is a selectable pulled model in the runtime state.
	state := fakeStateReader{states: []statetype.BackendRuntimeState{{
		ID:   "backend-openvino",
		Name: "openvino",
		Backend: runtimetypes.Backend{
			ID:      "backend-openvino",
			Name:    "openvino",
			Type:    "openvino",
			BaseURL: root,
		},
		PulledModels: []statetype.ModelPullStatus{{
			Name:    "qwen3-8b-ov-acme",
			Model:   "qwen3-8b-ov-acme",
			CanChat: true,
		}},
	}}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/modeld/load", strings.NewReader(`{"model":"openvino:qwen3-8b-ov-acme"}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// The variant loads the BASE IR directory (reused, never duplicated) with the
	// variant's adapter attached — identity = base IR + adapter.
	if provider.loadRef.Name != "qwen3-8b-ov-acme" || provider.loadRef.Path != baseDir {
		t.Fatalf("variant load ref should target base IR dir: %#v", provider.loadRef)
	}
	if len(provider.loadRef.Adapters) != 1 {
		t.Fatalf("expected one adapter in variant load ref, got %#v", provider.loadRef.Adapters)
	}
	adapter := provider.loadRef.Adapters[0]
	if adapter.Name != "acme" || adapter.Path != adapterPath || adapter.Digest != adapterDigest || adapter.Scale != 2 {
		t.Fatalf("unexpected variant adapter ref: %#v", adapter)
	}
	// Variant inherits the base model's declared context.
	if provider.loadCfg.NumCtx != 4096 {
		t.Fatalf("variant should inherit base context_length, got %#v", provider.loadCfg)
	}
}

func TestModelsReturnsLocalModelsWithoutBackendPaths(t *testing.T) {
	root := t.TempDir()
	provider := &fakeStatusProvider{}
	state := fakeStateReader{states: []statetype.BackendRuntimeState{localRuntimeState(root, "qwen3-8b", "llama")}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/models", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), root) {
		t.Fatalf("models response leaked backend path: %s", rr.Body.String())
	}
	var got []LocalModel
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != 1 || got[0].ID != "llama:qwen3-8b" || got[0].BackendType != "llama" {
		t.Fatalf("unexpected models: %#v", got)
	}
}

func TestCapacityResolvesLlamaModelServerSide(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "qwen3-8b")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	modelBytes := []byte("fake gguf")
	modelPath := filepath.Join(modelDir, "model.gguf")
	if err := os.WriteFile(modelPath, modelBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "contenox-llama.json"), []byte(`{"runtime":{"num_ctx":4096,"num_batch":128,"num_gpu_layers":35}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(modelBytes)
	wantDigest := hex.EncodeToString(sum[:])
	provider := &fakeStatusProvider{
		describeInfo: transport.ModelInfo{
			ModelMaxContext:         32768,
			EffectiveContext:        4096,
			PlannerEffectiveContext: 12000,
			Reason:                  "limited by test device",
			ResolvedGpuLayers:       35,
			RuntimeName:             "llamacpp",
			SupportsGPUOffload:      true,
			DeviceKind:              "gpu",
			DeviceTotalBytes:        16 << 30,
			MemoryContextTokens:     4096,
		},
	}
	state := fakeStateReader{states: []statetype.BackendRuntimeState{localRuntimeState(root, "qwen3-8b", "llama")}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/capacity?model=llama:qwen3-8b", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if provider.describeRef.Name != "qwen3-8b" || provider.describeRef.Type != "llama" || provider.describeRef.Path != modelPath {
		t.Fatalf("unexpected describe ref: %#v", provider.describeRef)
	}
	if provider.describeRef.Digest != wantDigest {
		t.Fatalf("describe digest = %q, want %q", provider.describeRef.Digest, wantDigest)
	}
	if provider.describeCfg.NumCtx != 4096 || provider.describeCfg.NumBatch != 128 || provider.describeCfg.NumGpuLayers != 35 {
		t.Fatalf("unexpected describe config: %#v", provider.describeCfg)
	}
	if strings.Contains(rr.Body.String(), root) || strings.Contains(rr.Body.String(), modelPath) {
		t.Fatalf("capacity response leaked backend path: %s", rr.Body.String())
	}
	var got CapacityResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Model.Digest != wantDigest || got.Model.ContextLength != 12000 || got.Info.EffectiveContext != 4096 || got.Info.PlannerEffectiveContext != 12000 || got.Info.Reason == "" {
		t.Fatalf("unexpected capacity response: %#v", got)
	}
}

// Without a profile/env override, capacity and load requests must go out with
// NumCtx=0 (auto) so modeld resolves the window fresh from live memory — a
// concrete placeholder (the old 8192 fallback) silently disabled modeld's
// dynamic capacity path for these routes.
func TestCapacityUsesAutoContextWithoutProfileOverride(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "qwen3-8b")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeStatusProvider{describeInfo: transport.ModelInfo{EffectiveContext: 17198}}
	state := fakeStateReader{states: []statetype.BackendRuntimeState{localRuntimeState(root, "qwen3-8b", "llama")}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/capacity?model=llama:qwen3-8b", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if provider.describeCfg.NumCtx != 0 {
		t.Fatalf("describe config NumCtx = %d, want 0 (auto) without a profile override", provider.describeCfg.NumCtx)
	}
}

func TestLoadUsesAutoContextWithoutProfileOverride(t *testing.T) {
	root := t.TempDir()
	modelDir := filepath.Join(root, "coder")
	if err := os.MkdirAll(modelDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modelDir, "model.gguf"), []byte("fake gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	provider := &fakeStatusProvider{
		loadActive: transport.ActiveModel{ModelName: "coder", Type: "llama", Generation: 8},
	}
	state := fakeStateReader{states: []statetype.BackendRuntimeState{localRuntimeState(root, "coder", "llama")}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/modeld/load", strings.NewReader(`{"model":"llama:coder","expectedGeneration":7}`))
	req.Header.Set("Content-Type", "application/json")
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !provider.loadCalled {
		t.Fatal("expected LoadModel to be called")
	}
	if provider.loadCfg.NumCtx != 0 {
		t.Fatalf("load config NumCtx = %d, want 0 (auto) without a profile override", provider.loadCfg.NumCtx)
	}
}

func TestCapacityRejectsAmbiguousLocalModelName(t *testing.T) {
	root := t.TempDir()
	provider := &fakeStatusProvider{}
	state := fakeStateReader{states: []statetype.BackendRuntimeState{
		localRuntimeState(root, "shared", "llama"),
		localRuntimeState(root, "shared", "openvino"),
	}}
	mux := http.NewServeMux()
	addRoutesForTest(mux, provider, state)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/capacity?model=shared", nil))

	if rr.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func localRuntimeState(root, model, backendType string) statetype.BackendRuntimeState {
	return statetype.BackendRuntimeState{
		ID:   "backend-" + backendType,
		Name: backendType,
		Backend: runtimetypes.Backend{
			ID:      "backend-" + backendType,
			Name:    backendType,
			Type:    backendType,
			BaseURL: root,
		},
		PulledModels: []statetype.ModelPullStatus{{
			Name:          model,
			Model:         model,
			ContextLength: 8192,
			CanChat:       true,
			CanPrompt:     true,
			CanStream:     true,
		}},
	}
}

func TestStatusDoesNotDialWhenModeldIsNotRunning(t *testing.T) {
	provider := &fakeStatusProvider{
		detected: modeldprobe.Status{
			State:  modeldprobe.StateNotRunning,
			Binary: "/bin/modeld",
		},
		statusErr: errors.New("must not be called"),
	}
	mux := http.NewServeMux()
	addRoutesWithProvider(mux, provider)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/modeld/status", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if provider.statusCalled {
		t.Fatal("did not expect daemon status to be queried")
	}

	var got StatusResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Available || got.State != "not-running" || got.Error == "" || got.Slot != nil {
		t.Fatalf("unexpected status response: %#v", got)
	}
}

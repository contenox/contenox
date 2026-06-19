package modelregistry

import (
	"context"
	"errors"
	"fmt"
	"strings"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/modelregistryservice"
	"github.com/contenox/runtime/runtime/runtimetypes"
)

var ErrNotFound = errors.New("model not found in registry")

type ModelDescriptor struct {
	ID        string `json:"id,omitempty" example:"m1a2b3c4-d5e6-f7g8-h9i0-j1k2l3m4n5o6"`
	Name      string `json:"name" example:"qwen3-8b"`
	SourceURL string `json:"sourceUrl" example:"https://huggingface.co/Qwen/..."`
	SizeBytes int64  `json:"sizeBytes" example:"934000000"`
	Curated   bool   `json:"curated" example:"true"`
	// Backend is the local backend this model targets: "" or "llama" for GGUF,
	// "openvino" for an OpenVINO IR model. It selects the models/<backend>/
	// directory and the pull strategy (single GGUF file vs multi-file IR repo).
	Backend string `json:"backend,omitempty" example:"openvino"`
	// Repo is the Hugging Face repo id for multi-file pulls (OpenVINO IR). For
	// GGUF models SourceURL points at the single file and Repo is empty.
	Repo string `json:"repo,omitempty" example:"OpenVINO/Qwen3-8B-int4-ov"`
	// ToolProtocol is the model-native tool-call output protocol (e.g. "qwen").
	// Set for curated models certified for tool calls; `model pull` writes it into
	// the model's profile so the local provider enables tool calls out of the box.
	ToolProtocol string `json:"toolProtocol,omitempty" example:"qwen"`
}

// BackendType returns the local backend this model targets, defaulting empty to
// "llama" for GGUF descriptors.
func (d ModelDescriptor) BackendType() string {
	if d.Backend == "" {
		return "llama"
	}
	return d.Backend
}

type Registry interface {
	// Resolve returns the descriptor for name from curated or user-added entries.
	Resolve(ctx context.Context, name string) (*ModelDescriptor, error)
	// List returns all known descriptors (curated + user-added). User entries override curated SourceURL.
	List(ctx context.Context) ([]ModelDescriptor, error)
	// OptimalFor returns the best registry name for an arbitrary model name string.
	// Exact match → family mapping → fallback.
	OptimalFor(ctx context.Context, modelName string) (string, error)
}

type registryImpl struct {
	svc modelregistryservice.Service
}

func New(svc modelregistryservice.Service) Registry {
	return &registryImpl{svc: svc}
}

func (r *registryImpl) Resolve(ctx context.Context, name string) (*ModelDescriptor, error) {
	// User DB entry takes precedence over curated.
	if r.svc != nil {
		if e, err := r.svc.GetByName(ctx, name); err == nil {
			d := mergeUserEntry(e)
			return &d, nil
		} else if !errors.Is(err, libdb.ErrNotFound) {
			return nil, err
		}
	}
	if d, ok := curatedModels[name]; ok {
		return &d, nil
	}
	return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
}

func (r *registryImpl) List(ctx context.Context) ([]ModelDescriptor, error) {
	merged := make(map[string]ModelDescriptor, len(curatedModels))
	for k, v := range curatedModels {
		merged[k] = v
	}
	if r.svc != nil {
		entries, err := r.svc.List(ctx, nil, 1000)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			merged[e.Name] = mergeUserEntry(e)
		}
	}
	out := make([]ModelDescriptor, 0, len(merged))
	for _, v := range merged {
		out = append(out, v)
	}
	return out, nil
}

func (r *registryImpl) OptimalFor(ctx context.Context, modelName string) (string, error) {
	lower := strings.ToLower(strings.SplitN(modelName, ":", 2)[0])

	// Exact match in curated.
	if _, ok := curatedModels[lower]; ok {
		return lower, nil
	}
	// Exact match in DB.
	if r.svc != nil {
		if _, err := r.svc.GetByName(ctx, lower); err == nil {
			return lower, nil
		}
	}
	// Family substring matching.
	for _, fm := range defaultFamilies {
		for _, sub := range fm.Substrings {
			if strings.Contains(lower, sub) {
				return fm.CanonicalName, nil
			}
		}
	}
	return defaultFallback, nil
}

func mergeUserEntry(e *runtimetypes.ModelRegistryEntry) ModelDescriptor {
	if e == nil {
		return ModelDescriptor{}
	}
	d, ok := curatedModels[e.Name]
	if !ok {
		d = ModelDescriptor{Name: e.Name}
	}
	d.ID = e.ID
	d.Name = e.Name
	d.Curated = false
	if e.SourceURL != "" {
		d.SourceURL = e.SourceURL
	}
	if e.SizeBytes > 0 {
		d.SizeBytes = e.SizeBytes
	}
	return d
}

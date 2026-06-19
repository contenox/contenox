package modelregistry_test

import (
	"context"
	"testing"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCuratedOnly returns a Registry backed by curated entries only (no DB).
func newCuratedOnly() modelregistry.Registry {
	return modelregistry.New(nil)
}

func TestUnit_Registry_ResolveCuratedByExactName(t *testing.T) {
	reg := newCuratedOnly()
	d, err := reg.Resolve(context.Background(), "qwen3-8b")
	require.NoError(t, err)
	assert.Equal(t, "qwen3-8b", d.Name)
	assert.True(t, d.Curated)
	assert.NotEmpty(t, d.SourceURL)
	assert.Equal(t, "qwen", d.ToolProtocol)
}

func TestUnit_Registry_ResolveCuratedQwen3CoderGGUF(t *testing.T) {
	reg := newCuratedOnly()
	d, err := reg.Resolve(context.Background(), "qwen3-coder-30b-a3b")
	require.NoError(t, err)

	assert.Equal(t, "qwen3-coder-30b-a3b", d.Name)
	assert.Equal(t, "llama", d.BackendType())
	assert.Equal(t, int64(18_556_689_568), d.SizeBytes)
	assert.Contains(t, d.SourceURL, "unsloth/Qwen3-Coder-30B-A3B-Instruct-GGUF")
	assert.Empty(t, d.Repo)
	assert.Equal(t, "qwen", d.ToolProtocol)
	assert.True(t, d.Curated)
}

func TestUnit_Registry_ResolveCuratedQwen3CoderOpenVINO(t *testing.T) {
	reg := newCuratedOnly()
	d, err := reg.Resolve(context.Background(), "qwen3-coder-30b-a3b-ov")
	require.NoError(t, err)

	assert.Equal(t, "qwen3-coder-30b-a3b-ov", d.Name)
	assert.Equal(t, "openvino", d.BackendType())
	assert.Equal(t, "OpenVINO/Qwen3-Coder-30B-A3B-Instruct-int4-ov", d.Repo)
	assert.Equal(t, int64(16_344_057_522), d.SizeBytes)
	assert.Equal(t, "https://huggingface.co/OpenVINO/Qwen3-Coder-30B-A3B-Instruct-int4-ov", d.SourceURL)
	assert.Equal(t, "qwen", d.ToolProtocol)
	assert.True(t, d.Curated)
}

func TestUnit_Registry_ResolveCuratedGPTOSSOpenVINO(t *testing.T) {
	reg := newCuratedOnly()
	d, err := reg.Resolve(context.Background(), "gpt-oss-20b-ov")
	require.NoError(t, err)

	assert.Equal(t, "gpt-oss-20b-ov", d.Name)
	assert.Equal(t, "openvino", d.BackendType())
	assert.Equal(t, "OpenVINO/gpt-oss-20b-int4-ov", d.Repo)
	assert.Equal(t, int64(12_623_951_367), d.SizeBytes)
	assert.True(t, d.Curated)
}

func TestUnit_Registry_ListIncludesCurated(t *testing.T) {
	reg := newCuratedOnly()
	entries, err := reg.List(context.Background())
	require.NoError(t, err)

	names := make(map[string]bool)
	for _, e := range entries {
		names[e.Name] = true
	}
	assert.True(t, names["qwen3-8b"])
	assert.True(t, names["qwen3-coder-30b-a3b"])
	assert.True(t, names["qwen3-coder-30b-a3b-ov"])
	assert.True(t, names["gemma3-4b"])
	assert.True(t, names["deepseek-r1-0528-qwen3-8b"])
	assert.True(t, names["gpt-oss-20b"])
	assert.True(t, names["llama3.2-1b"])
	assert.True(t, names["tiny"])
}

func TestUnit_Registry_OptimalForExactCuratedMatch(t *testing.T) {
	reg := newCuratedOnly()
	name, err := reg.OptimalFor(context.Background(), "qwen3-8b")
	require.NoError(t, err)
	assert.Equal(t, "qwen3-8b", name)
}

func TestUnit_Registry_OptimalForFamilyMapping(t *testing.T) {
	reg := newCuratedOnly()
	name, err := reg.OptimalFor(context.Background(), "Qwen3-8B-Instruct")
	require.NoError(t, err)
	assert.Equal(t, "qwen3-8b", name)
}

func TestUnit_Registry_OptimalForQwen3CoderFamilyMapping(t *testing.T) {
	reg := newCuratedOnly()
	name, err := reg.OptimalFor(context.Background(), "Qwen/Qwen3-Coder-30B-A3B-Instruct-GGUF:Q4_K_M")
	require.NoError(t, err)
	assert.Equal(t, "qwen3-coder-30b-a3b", name)
}

func TestUnit_Registry_OptimalForOpenVINOQwen3CoderFamilyMapping(t *testing.T) {
	reg := newCuratedOnly()
	name, err := reg.OptimalFor(context.Background(), "OpenVINO/Qwen3-Coder-30B-A3B-Instruct-int4-ov")
	require.NoError(t, err)
	assert.Equal(t, "qwen3-coder-30b-a3b-ov", name)
}

func TestUnit_Registry_OptimalForCurrentFamilies(t *testing.T) {
	reg := newCuratedOnly()
	tests := map[string]string{
		"google/gemma-3-4b-it":            "gemma3-4b",
		"google/gemma-3-1b-it":            "gemma3-1b",
		"microsoft/Phi-4-mini-instruct":   "phi-4-mini",
		"DeepSeek-R1-0528-Qwen3-8B":       "deepseek-r1-0528-qwen3-8b",
		"bartowski/openai_gpt-oss-20b":    "gpt-oss-20b",
		"OpenVINO/gpt-oss-20b-int4-ov":    "gpt-oss-20b-ov",
		"OpenVINO/gemma-3-12b-it-int4-ov": "gemma3-12b-ov",
	}
	for input, want := range tests {
		got, err := reg.OptimalFor(context.Background(), input)
		require.NoError(t, err)
		assert.Equal(t, want, got, input)
	}
}

func TestUnit_Registry_OpenVINOCrossSyncWithCuratedLlamaModels(t *testing.T) {
	reg := newCuratedOnly()
	pairs := map[string]string{
		"gemma3-4b":                   "gemma3-4b-ov",
		"gemma3-12b":                  "gemma3-12b-ov",
		"phi-4-mini":                  "phi-4-mini-ov",
		"qwen3-4b":                    "qwen3-4b-ov",
		"qwen3-8b":                    "qwen3-8b-ov",
		"qwen3-14b":                   "qwen3-14b-ov",
		"qwen3-30b":                   "qwen3-30b-ov",
		"qwen3-coder-30b-a3b":         "qwen3-coder-30b-a3b-ov",
		"deepseek-r1-distill-qwen-7b": "deepseek-r1-distill-qwen-7b-ov",
		"gpt-oss-20b":                 "gpt-oss-20b-ov",
	}

	for llamaName, openvinoName := range pairs {
		llamaModel, err := reg.Resolve(context.Background(), llamaName)
		require.NoError(t, err, llamaName)
		assert.Equal(t, "llama", llamaModel.BackendType(), llamaName)
		assert.Empty(t, llamaModel.Repo, llamaName)

		openvinoModel, err := reg.Resolve(context.Background(), openvinoName)
		require.NoError(t, err, openvinoName)
		assert.Equal(t, "openvino", openvinoModel.BackendType(), openvinoName)
		assert.NotEmpty(t, openvinoModel.Repo, openvinoName)
		assert.NotEmpty(t, openvinoModel.SourceURL, openvinoName)
		assert.True(t, openvinoModel.Curated, openvinoName)
		assert.Equal(t, llamaModel.ToolProtocol, openvinoModel.ToolProtocol, openvinoName)
	}
}

func TestUnit_Registry_UserEntryKeepsCuratedBackendAndToolProtocol(t *testing.T) {
	svc := fakeModelRegistryService{
		byName: map[string]*runtimetypes.ModelRegistryEntry{
			"qwen3-8b": {
				ID:        "user-qwen3-8b",
				Name:      "qwen3-8b",
				SourceURL: "https://example.com/custom-qwen3-8b.gguf",
			},
		},
	}
	reg := modelregistry.New(svc)

	d, err := reg.Resolve(context.Background(), "qwen3-8b")
	require.NoError(t, err)
	assert.Equal(t, "user-qwen3-8b", d.ID)
	assert.Equal(t, "https://example.com/custom-qwen3-8b.gguf", d.SourceURL)
	assert.Equal(t, int64(5_027_783_488), d.SizeBytes)
	assert.Equal(t, "llama", d.BackendType())
	assert.Equal(t, "qwen", d.ToolProtocol)
	assert.False(t, d.Curated)

	all, err := reg.List(context.Background())
	require.NoError(t, err)
	var listed *modelregistry.ModelDescriptor
	for i := range all {
		if all[i].Name == "qwen3-8b" {
			listed = &all[i]
			break
		}
	}
	require.NotNil(t, listed)
	assert.Equal(t, "qwen", listed.ToolProtocol)
	assert.Equal(t, int64(5_027_783_488), listed.SizeBytes)
	assert.False(t, listed.Curated)
}

func TestUnit_Registry_OptimalForFallbackOnUnknown(t *testing.T) {
	reg := newCuratedOnly()
	name, err := reg.OptimalFor(context.Background(), "totally-unknown-model-xyz")
	require.NoError(t, err)
	assert.Equal(t, "tiny", name) // defaultFallback
}

func TestUnit_Registry_ResolveNotFoundReturnsError(t *testing.T) {
	reg := newCuratedOnly()
	_, err := reg.Resolve(context.Background(), "does-not-exist")
	require.Error(t, err)
	require.ErrorIs(t, err, modelregistry.ErrNotFound)
}

type fakeModelRegistryService struct {
	byName map[string]*runtimetypes.ModelRegistryEntry
}

func (s fakeModelRegistryService) Create(context.Context, *runtimetypes.ModelRegistryEntry) error {
	return nil
}

func (s fakeModelRegistryService) Get(context.Context, string) (*runtimetypes.ModelRegistryEntry, error) {
	return nil, libdb.ErrNotFound
}

func (s fakeModelRegistryService) GetByName(_ context.Context, name string) (*runtimetypes.ModelRegistryEntry, error) {
	if e, ok := s.byName[name]; ok {
		return e, nil
	}
	return nil, libdb.ErrNotFound
}

func (s fakeModelRegistryService) Update(context.Context, *runtimetypes.ModelRegistryEntry) error {
	return nil
}

func (s fakeModelRegistryService) Delete(context.Context, string) error {
	return nil
}

func (s fakeModelRegistryService) List(context.Context, *time.Time, int) ([]*runtimetypes.ModelRegistryEntry, error) {
	out := make([]*runtimetypes.ModelRegistryEntry, 0, len(s.byName))
	for _, e := range s.byName {
		out = append(out, e)
	}
	return out, nil
}

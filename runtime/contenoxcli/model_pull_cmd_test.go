package contenoxcli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUnit_WriteToolProfileWritesLlamaProfile(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeToolProfile("llama", dir, "llama:common_chat_tool_parser"))

	body, err := os.ReadFile(filepath.Join(dir, "contenox-llama.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"tool_calls":{"protocol":"llama:common_chat_tool_parser"}}`, string(body))
}

func TestUnit_WriteToolProfileWritesOpenVINOProfile(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeToolProfile("openvino", dir, "openvino:llama3_json_tool_parser"))

	body, err := os.ReadFile(filepath.Join(dir, "contenox-openvino.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"tool_calls":{"protocol":"openvino:llama3_json_tool_parser"}}`, string(body))
}

func TestUnit_WriteLocalModelProfileWritesReasoningProtocol(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeLocalModelProfile("llama", dir, "", "llama:common_chat_reasoning_parser", "deepseek"))

	body, err := os.ReadFile(filepath.Join(dir, "contenox-llama.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"reasoning":{"protocol":"llama:common_chat_reasoning_parser","format":"deepseek"}}`, string(body))
}

func TestUnit_WriteLocalModelProfileWritesToolAndReasoningProtocols(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, writeLocalModelProfile("llama", dir, "llama:common_chat_tool_parser", "llama:common_chat_reasoning_parser", "deepseek"))

	body, err := os.ReadFile(filepath.Join(dir, "contenox-llama.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"tool_calls":{"protocol":"llama:common_chat_tool_parser"},"reasoning":{"protocol":"llama:common_chat_reasoning_parser","format":"deepseek"}}`, string(body))
}

func TestUnit_WriteToolProfileKeepsExistingProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "contenox-llama.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"profile_id":"user"}`), 0o644))

	require.NoError(t, writeToolProfile("local", dir, "llama:common_chat_tool_parser"))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"profile_id":"user"}`, string(body))
}

func TestUnit_LegacyLlamaModelDirFindsFlatExistingModel(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".contenox", "models", "qwen3-8b")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.gguf"), []byte("gguf"), 0o644))

	got, ok := legacyLlamaModelDir(home, "qwen3-8b")

	require.True(t, ok)
	assert.Equal(t, dir, got)
}

func TestUnit_LocalModelPresentChecksBackendMarkers(t *testing.T) {
	dir := t.TempDir()
	assert.False(t, localModelPresent("llama", dir))
	assert.False(t, localModelPresent("openvino", dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.gguf"), []byte("gguf"), 0o644))
	assert.True(t, localModelPresent("llama", dir))
	assert.False(t, localModelPresent("openvino", dir))

	require.NoError(t, os.WriteFile(filepath.Join(dir, "openvino_model.xml"), []byte("<xml/>"), 0o644))
	assert.True(t, localModelPresent("openvino", dir))

	langDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(langDir, "openvino_language_model.xml"), []byte("<xml/>"), 0o644))
	assert.True(t, localModelPresent("openvino", langDir))
}

// One pull action installs every artifact a llama vision model needs: the
// model GGUF and its multimodal projector as mmproj.gguf.
func TestUnit_PullLlamaArtifactsFetchesModelAndMMProj(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/model.gguf":
			_, _ = io.WriteString(w, "gguf-model-bytes")
		case "/mmproj.gguf":
			_, _ = io.WriteString(w, "gguf-mmproj-bytes")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()

	err := pullLlamaArtifacts("vlm", srv.URL+"/model.gguf", srv.URL+"/mmproj.gguf", dir, io.Discard)
	require.NoError(t, err)

	model, err := os.ReadFile(filepath.Join(dir, "model.gguf"))
	require.NoError(t, err)
	assert.Equal(t, "gguf-model-bytes", string(model))
	mmproj, err := os.ReadFile(filepath.Join(dir, "mmproj.gguf"))
	require.NoError(t, err)
	assert.Equal(t, "gguf-mmproj-bytes", string(mmproj))
}

// A model pulled before it was curated for vision upgrades in place: the
// existing model.gguf is kept and only the missing projector is fetched.
func TestUnit_PullLlamaArtifactsFetchesMissingMMProjForExistingModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/mmproj.gguf" {
			_, _ = io.WriteString(w, "gguf-mmproj-bytes")
			return
		}
		t.Errorf("unexpected download of %s for an already-installed model", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.gguf"), []byte("existing"), 0o644))

	err := pullLlamaArtifacts("vlm", srv.URL+"/model.gguf", srv.URL+"/mmproj.gguf", dir, io.Discard)
	require.NoError(t, err)

	model, err := os.ReadFile(filepath.Join(dir, "model.gguf"))
	require.NoError(t, err)
	assert.Equal(t, "existing", string(model))
	mmproj, err := os.ReadFile(filepath.Join(dir, "mmproj.gguf"))
	require.NoError(t, err)
	assert.Equal(t, "gguf-mmproj-bytes", string(mmproj))
}

// Refuse-don't-spill at install time: a vision entry whose projector cannot be
// fetched fails with an actionable error and leaves no partial mmproj behind —
// never a silently text-only model.
func TestUnit_PullLlamaArtifactsFailsLoudlyWhenMMProjMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/model.gguf" {
			_, _ = io.WriteString(w, "gguf-model-bytes")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()

	err := pullLlamaArtifacts("gemma4-e4b", srv.URL+"/model.gguf", srv.URL+"/mmproj.gguf", dir, io.Discard)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "vision projector")
	assert.Contains(t, err.Error(), "mmproj.gguf")
	assert.Contains(t, err.Error(), "contenox model pull gemma4-e4b")
	_, statErr := os.Stat(filepath.Join(dir, "mmproj.gguf"))
	assert.True(t, os.IsNotExist(statErr), "failed projector download must not leave a partial mmproj.gguf")
}

// Text entries keep the single-file pull: no projector URL means no second
// download.
func TestUnit_PullLlamaArtifactsTextModelSkipsMMProj(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/model.gguf" {
			_, _ = io.WriteString(w, "gguf-model-bytes")
			return
		}
		t.Errorf("unexpected request %s for a text model", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()
	dir := t.TempDir()

	require.NoError(t, pullLlamaArtifacts("text", srv.URL+"/model.gguf", "", dir, io.Discard))
	_, statErr := os.Stat(filepath.Join(dir, "mmproj.gguf"))
	assert.True(t, os.IsNotExist(statErr))
}

func TestUnit_ModelPullLongIncludesRegistryGeneratedCuratedModels(t *testing.T) {
	got := modelPullLong()

	for _, want := range []string{
		"llama:",
		"openvino:",
		"vram",
		"use",
		"qwen2.5-coder-7b",
		"devstral-small-2507",
		"codestral-22b",
		"qwen3-coder-30b-a3b",
		"qwen3-coder-30b-a3b-ov",
		"qwen2.5-coder-0.5b-ov",
		"coding default",
		"gemma4-26b-a4b",
		"gpt-oss-20b-ov",
		"fastest smoke test",
		"chat+vision",
		"gemma4-e4b-ov",
	} {
		assert.Contains(t, got, want)
	}
	assert.NotContains(t, got, "~19 GB")
	assert.Equal(t, 1, strings.Count(got, "qwen3-coder-30b-a3b-ov"))
}

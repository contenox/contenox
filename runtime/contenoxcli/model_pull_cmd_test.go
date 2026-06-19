package contenoxcli

import (
	"os"
	"path/filepath"
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
}

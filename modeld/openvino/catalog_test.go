package openvino

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/contenox/runtime/modeld/openvino/ovsession"
)

func TestUnit_OpenVINOCatalog_ListModels_RespectsNativeAvailability(t *testing.T) {
	dir := t.TempDir()
	modelDir := filepath.Join(dir, "qwen-coder")
	require.NoError(t, os.MkdirAll(modelDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(modelDir, "openvino_model.xml"), []byte("<xml/>"), 0644))

	models, err := (&catalogProvider{dir: dir}).ListModels(context.Background())
	require.NoError(t, err)
	if !ovsession.Available {
		require.Empty(t, models)
		return
	}

	require.Len(t, models, 1)
	require.Equal(t, "qwen-coder", models[0].Name)
	require.Equal(t, ovsession.GenAIAvailable, models[0].CanChat)
	require.Equal(t, ovsession.GenAIAvailable, models[0].CanPrompt)
	require.Equal(t, ovsession.GenAIAvailable, models[0].CanStream)
	require.Equal(t, "openvino-ir", models[0].Meta["format"])
}

func TestUnit_OpenVINOCatalog_ListModels_SkipsEntriesWithoutIR(t *testing.T) {
	if !ovsession.Available {
		t.Skip("default build advertises no OpenVINO models")
	}

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "nomodel"), 0755))
	modelDir := filepath.Join(dir, "hasmodel")
	require.NoError(t, os.MkdirAll(modelDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(modelDir, "openvino_model.xml"), []byte("<xml/>"), 0644))

	models, err := (&catalogProvider{dir: dir}).ListModels(context.Background())
	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "hasmodel", models[0].Name)
}

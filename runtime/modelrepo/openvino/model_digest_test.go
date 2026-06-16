package openvino

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnit_OpenVINOModelDirDigest_ChangesWithIRAndTemplate(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "openvino_model.xml"), []byte("<xml/>"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "openvino_model.bin"), []byte("weights-v1"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tokenizer_config.json"), []byte(`{"chat_template":"a"}`), 0644))

	base, err := modelDirDigest(dir)
	require.NoError(t, err)
	require.NotEmpty(t, base)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "tokenizer_config.json"), []byte(`{"chat_template":"b"}`), 0644))
	templateChanged, err := modelDirDigest(dir)
	require.NoError(t, err)
	require.NotEqual(t, base, templateChanged)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "openvino_model.bin"), []byte("weights-v2"), 0644))
	weightsChanged, err := modelDirDigest(dir)
	require.NoError(t, err)
	require.NotEqual(t, templateChanged, weightsChanged)
}

func TestUnit_OpenVINOModelDirDigest_RequiresIdentityFiles(t *testing.T) {
	_, err := modelDirDigest(t.TempDir())
	require.ErrorContains(t, err, "no model identity files")
}

package contenoxcli

import (
	"os"
	"path/filepath"
)

var openVINOModelEntrypoints = []string{
	"openvino_model.xml",
	"openvino_language_model.xml",
}

// llamaMMProjFileName is the multimodal projector expected beside model.gguf.
// The name is fixed by modeld's llama backend (llama.MMProjFilename), which
// resolves the projector from the model path — installing it under this name
// is what makes a curated vision model actually serve image input.
const llamaMMProjFileName = "mmproj.gguf"

func openVINOModelEntrypointPath(dir string) (string, bool) {
	for _, name := range openVINOModelEntrypoints {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

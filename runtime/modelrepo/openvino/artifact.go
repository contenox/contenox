package openvino

import (
	"os"
	"path/filepath"
)

var modelEntrypointNames = []string{
	"openvino_model.xml",
	"openvino_language_model.xml",
}

func modelEntrypointPath(modelDir string) (string, bool) {
	for _, name := range modelEntrypointNames {
		path := filepath.Join(modelDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

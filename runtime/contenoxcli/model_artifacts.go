package contenoxcli

import (
	"os"
	"path/filepath"
)

var openVINOModelEntrypoints = []string{
	"openvino_model.xml",
	"openvino_language_model.xml",
}

func openVINOModelEntrypointPath(dir string) (string, bool) {
	for _, name := range openVINOModelEntrypoints {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

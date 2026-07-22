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

// visionEncoderModelName is the vision encoder an OpenVINO VLM snapshot ships
// beside the language model. Its presence is the offline best-effort vision
// signal for the catalog while modeld cannot answer Describe; a live
// Describe's ModelInfo.SupportsVision stays the authoritative truth.
const visionEncoderModelName = "openvino_vision_embeddings_model.xml"

func visionEncoderPresent(modelDir string) bool {
	_, err := os.Stat(filepath.Join(modelDir, visionEncoderModelName))
	return err == nil
}

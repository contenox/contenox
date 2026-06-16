package openvino

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
)

var modelDigestFiles = []string{
	"openvino_model.xml",
	"openvino_model.bin",
	"openvino_tokenizer.xml",
	"openvino_tokenizer.bin",
	"openvino_detokenizer.xml",
	"openvino_detokenizer.bin",
	"tokenizer.json",
	"tokenizer_config.json",
	"special_tokens_map.json",
	"config.json",
	"generation_config.json",
	"chat_template.jinja",
}

// modelDirDigest hashes the IR, tokenizer, and chat-template artifacts that
// define cache compatibility for an OpenVINO GenAI session. The digest is part
// of the session-pool key, so replacing a model in place cannot reuse stale KV.
func modelDirDigest(modelDir string) (string, error) {
	if strings.TrimSpace(modelDir) == "" {
		return "", fmt.Errorf("openvino model digest: model directory is required")
	}

	h := sha256.New()
	found := false
	for _, rel := range modelDigestFiles {
		ok, err := hashModelDigestFile(h, modelDir, rel)
		if err != nil {
			return "", err
		}
		found = found || ok
	}
	if !found {
		return "", fmt.Errorf("openvino model digest: no model identity files found in %s", modelDir)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashModelDigestFile(h hash.Hash, modelDir, rel string) (bool, error) {
	path := filepath.Join(modelDir, rel)
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("openvino model digest %s: %w", rel, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return false, fmt.Errorf("openvino model digest %s: %w", rel, err)
	}
	if info.IsDir() {
		return false, nil
	}

	_, _ = h.Write([]byte(rel))
	_, _ = h.Write([]byte{0})
	if _, err := io.Copy(h, f); err != nil {
		return false, fmt.Errorf("openvino model digest %s: %w", rel, err)
	}
	_, _ = h.Write([]byte{0})
	return true, nil
}

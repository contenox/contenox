package openvino

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/contenox/runtime/runtime/transport"
)

func resolveProfileAdapters(profileDir string, adapters []adapterProfile) ([]transport.AdapterSpec, error) {
	if len(adapters) == 0 {
		return nil, nil
	}
	out := make([]transport.AdapterSpec, 0, len(adapters))
	for i, adapter := range adapters {
		path := strings.TrimSpace(adapter.Path)
		if path == "" {
			return nil, fmt.Errorf("openvino profile adapter[%d]: path is required", i)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(profileDir, path)
		}
		digest := strings.TrimSpace(adapter.Digest)
		if digest == "" {
			var err error
			digest, err = fileSHA256(path)
			if err != nil {
				return nil, fmt.Errorf("openvino profile adapter[%d]: %w", i, err)
			}
		}
		scale := float32(1)
		if adapter.Scale != nil {
			scale = *adapter.Scale
		}
		out = append(out, transport.AdapterSpec{
			Name:   strings.TrimSpace(adapter.Name),
			Path:   path,
			Digest: digest,
			Scale:  scale,
		})
	}
	return out, nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

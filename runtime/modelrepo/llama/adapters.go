package llama

import (
	"fmt"
	"path/filepath"
	"strings"
)

func resolveProfileAdapters(profileDir string, adapters []adapterProfile) ([]AdapterSpec, error) {
	if len(adapters) == 0 {
		return nil, nil
	}
	out := make([]AdapterSpec, 0, len(adapters))
	for i, adapter := range adapters {
		path := strings.TrimSpace(adapter.Path)
		if path == "" {
			return nil, fmt.Errorf("llama profile adapter[%d]: path is required", i)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(profileDir, path)
		}
		digest := strings.TrimSpace(adapter.Digest)
		if digest == "" {
			var err error
			digest, err = modelFileDigest(path)
			if err != nil {
				return nil, fmt.Errorf("llama profile adapter[%d]: %w", i, err)
			}
		}
		scale := float32(1)
		if adapter.Scale != nil {
			scale = *adapter.Scale
		}
		out = append(out, AdapterSpec{
			Name:   strings.TrimSpace(adapter.Name),
			Path:   path,
			Digest: digest,
			Scale:  scale,
		})
	}
	return out, nil
}

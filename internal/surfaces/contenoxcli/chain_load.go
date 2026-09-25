package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/contenox/contenox/internal/kernel/taskengine"
	"github.com/contenox/contenox/internal/services/vfs"
)

func loadChainFromFile(ctx context.Context, path string) (*taskengine.TaskChainDefinition, error) {
	view, vErr := vfs.OpenPrivilegedView(filepath.Dir(path))
	if vErr != nil {
		return nil, fmt.Errorf("failed to read chain file %q: %w", path, vErr)
	}
	data, err := view.ReadFile(ctx, filepath.Base(path))
	if err != nil {
		return nil, fmt.Errorf("failed to read chain file %q: %w", path, err)
	}
	var chain taskengine.TaskChainDefinition
	if err := json.Unmarshal(data, &chain); err != nil {
		return nil, fmt.Errorf("failed to parse chain JSON %q: %w", path, err)
	}
	return &chain, nil
}

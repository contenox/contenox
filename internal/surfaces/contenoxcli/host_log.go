package contenoxcli

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/contenox/contenox/liblog"
	"github.com/spf13/cobra"
)

const hostLogDirName = "logs"

func openHostLog(cmd *cobra.Command, name string) (*liblog.Writer, error) {
	dir, _ := cmd.Flags().GetString("log-dir")
	if strings.TrimSpace(dir) == "" {
		base, err := globalContenoxDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(base, hostLogDirName)
	}
	return liblog.Open(liblog.Config{Dir: dir, Name: name})
}

// applyStoredLogSettings moves a live host log onto the operator's configured
// bounds; anything unset stays on the default it booted with.
func applyStoredLogSettings(ctx context.Context, store runtimetypes.Store, w *liblog.Writer) {
	if w == nil {
		return
	}
	w.Reconfigure(logSettingsFromConfig(ctx, store))
}

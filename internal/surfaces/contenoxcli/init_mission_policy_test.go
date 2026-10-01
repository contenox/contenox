package contenoxcli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/contenox/contenox/internal/services/clikv"
	"github.com/contenox/contenox/internal/store/runtimetypes"
	"github.com/stretchr/testify/require"
)

func TestUnit_InitMissionPolicy(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(map[bool]string{false: "init", true: "global"}[global], func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			ctx := context.Background()
			var out, errOut bytes.Buffer
			init := func() {
				if global {
					require.NoError(t, RunGlobalInit(ctx, &out))
				} else {
					require.NoError(t, RunInit(&out, &errOut, false, false, "openai", filepath.Join(home, "workspace", ".contenox"), ""))
				}
			}
			init()
			path, err := globalDBPath()
			require.NoError(t, err)
			db, err := OpenDBAt(ctx, path)
			require.NoError(t, err)
			defer db.Close()
			store := runtimetypes.New(db.WithoutTransaction())
			require.Equal(t, "hitl-policy-default.json", clikv.Read(ctx, store, "default-mission-policy"))
			require.NoError(t, clikv.WriteConfig(ctx, store, "", "default-mission-policy", "hitl-policy-custom.json"))
			init()
			require.Equal(t, "hitl-policy-custom.json", clikv.Read(ctx, store, "default-mission-policy"))
		})
	}
}

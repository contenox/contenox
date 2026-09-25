//go:build llamanode && llamacpp_direct

package slot

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/contenox/contenox/internal/modeld/contextasm"
	"github.com/contenox/contenox/internal/modeld/llama"
	_ "github.com/contenox/contenox/internal/modeld/llama/llamasession"
	"github.com/contenox/contenox/internal/transport"
	"github.com/stretchr/testify/require"
)

func TestSystem_SnapshotNativeLifecycle(t *testing.T) {
	model := os.Getenv("CONTENOX_LLAMA_TINY_GGUF")
	if model == "" {
		t.Skip("CONTENOX_LLAMA_TINY_GGUF is not set")
	}
	_, err := os.Stat(model)
	require.NoError(t, err)
	t.Setenv("CONTENOX_LLAMA_GPU_LAYERS", "0")
	t.Setenv("CONTENOX_WARM_SNAPSHOT_DISABLE", "")
	t.Setenv("CONTENOX_WARM_SNAPSHOT_DIR", "")
	for _, event := range []string{"unload", "shutdown", "corrupt", "decoded-unload"} {
		t.Run(event, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			root := t.TempDir()
			newService := func() *Service {
				return New(llama.NewService(), WithDataRoot(root), WithBackend("llama"), WithIdleTTL(time.Minute))
			}
			svc := newService()
			req := transport.OpenSessionRequest{ModelName: "native", Type: "llama", Path: model, Config: transport.Config{NumCtx: 512, NumBatch: 64, NumThreads: 2, DisableBOS: true}}
			stable, suffix := "system\nYou are helpful.\n", "user\nHello\nassistant\n"
			manifest := contextasm.ContextManifest{Backend: "llamacpp", ProfileID: "native-snapshot", StableByteHash: contextasm.HashString(stable), StableBytes: len(stable), TotalBytes: len(stable) + len(suffix)}
			expectRestored := false
			turn := func() (string, transport.PrefixStatus) {
				sess, err := svc.OpenSession(ctx, req)
				require.NoError(t, err)
				defer sess.Close()
				if expectRestored {
					require.Positive(t, sess.ExplainContext().ResidentTokens)
				}
				prefix, err := sess.EnsurePrefix(ctx, transport.PrefixInput{Text: stable, Manifest: manifest})
				require.NoError(t, err)
				_, err = sess.PrefillSuffix(ctx, transport.SuffixInput{Text: suffix, Manifest: manifest})
				require.NoError(t, err)
				temp, seed := 0.0, 7
				chunks, err := sess.Decode(ctx, transport.DecodeConfig{MaxTokens: 4, Temperature: &temp, Seed: &seed})
				require.NoError(t, err)
				text := ""
				for chunk := range chunks {
					require.NoError(t, chunk.Error)
					text += chunk.Text
				}
				require.NotEmpty(t, text)
				return text, prefix
			}
			want, cold := turn()
			require.Zero(t, cold.ReusedTokens)
			if event != "decoded-unload" {
				seed, err := svc.OpenSession(ctx, req)
				require.NoError(t, err)
				_, err = seed.EnsurePrefix(ctx, transport.PrefixInput{Text: stable, Manifest: manifest})
				require.NoError(t, err)
				require.NoError(t, seed.Close())
			}
			if event == "shutdown" {
				require.NoError(t, svc.Shutdown(ctx))
				svc = newService()
			} else {
				require.NoError(t, svc.UnloadModel(ctx, transport.UnloadModelRequest{}))
			}
			defer svc.Shutdown(context.Background())
			files, err := os.ReadDir(svc.snapshotDir())
			require.NoError(t, err)
			require.Len(t, files, 1)
			if event == "corrupt" {
				require.NoError(t, os.WriteFile(svc.snapshotDir()+"/"+files[0].Name(), []byte("corrupt"), 0600))
			}
			expectRestored = event != "corrupt"
			got, restored := turn()
			require.Equal(t, want, got)
			if event == "corrupt" {
				require.Zero(t, restored.ReusedTokens)
			} else if event != "decoded-unload" {
				require.Positive(t, restored.ReusedTokens)
				require.Equal(t, restored.PrefixTokens, restored.ReusedTokens)
			}
			t.Logf("%s: cold=%d restored=%d prefix=%d continuation=%q", event, cold.ReusedTokens, restored.ReusedTokens, restored.PrefixTokens, got)
		})
	}
}

package contenoxcli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var modelPullCmd = &cobra.Command{
	Use:   "pull <name>",
	Short: "Download a GGUF model for local inference.",
	Long: `Download a GGUF model from HuggingFace and store it under ~/.contenox/models/<name>/.

Curated models — run 'contenox model registry-list' to see full list with sizes.
  By GPU size (approximate Q4_K_M VRAM needed):
  ~1 GB   tiny            FastThink 0.5B (testing only)
  ~1 GB   llama3.2-1b     Llama 3.2 1B
  ~1-2 GB granite-3.2-2b  IBM Granite 3.2 2B
  ~1 GB   qwen2.5-1.5b    Qwen 2.5 1.5B
  ~3 GB   qwen3-4b        Qwen 3 4B
  ~3 GB   gemma4-e2b      Gemma 4 E2B
  ~3 GB   phi-4-mini      Phi-4 Mini
  ~5 GB   gemma4-e4b      Gemma 4 E4B
  ~5 GB   granite-3.2-8b  IBM Granite 3.2 8B
  ~5 GB   qwen2.5-7b      Qwen 2.5 7B
  ~9 GB   qwen3-14b       Qwen 3 14B
  ~19 GB  qwen3-30b       Qwen 3 30B (MoE, fast)
  ~30 GB  kimi-linear     Kimi Linear 48B (MoE)
  ~68 GB  llama4-scout    Llama 4 Scout 17Bx16E (multi-GPU)

Or provide an explicit URL:
  contenox model pull my-model --url https://huggingface.co/.../model.gguf

After downloading, the model is ready to use immediately. The llama backend is
registered by 'contenox init' and the first pulled model becomes the default:
  contenox model list
  contenox "hello, what can you do?"`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(context.Background())
		rawURL, _ := cmd.Flags().GetString("url")

		// Registry is the single source of truth for curated model URLs.
		reg := modelregistry.New(nil)

		var name, downloadURL string
		switch {
		case rawURL != "" && len(args) == 1:
			name = args[0]
			downloadURL = rawURL
		case rawURL != "" && len(args) == 0:
			return fmt.Errorf("provide a model name when using --url: contenox model pull <name> --url <url>")
		case len(args) == 1:
			name = args[0]
			d, err := reg.Resolve(ctx, name)
			if err != nil {
				all, _ := reg.List(ctx)
				names := make([]string, 0, len(all))
				for _, e := range all {
					names = append(names, e.Name)
				}
				sort.Strings(names)
				return fmt.Errorf("unknown model %q\n\nRun 'contenox model registry-list' to see all curated models.\nOr specify --url to download any GGUF file.", name)
			}
			downloadURL = d.SourceURL
		default:
			return cmd.Help()
		}

		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve home directory: %w", err)
		}
		modelDir := filepath.Join(homeDir, ".contenox", "models", name)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			return fmt.Errorf("create model directory: %w", err)
		}

		destPath := filepath.Join(modelDir, "model.gguf")
		alreadyPresent := false
		if _, err := os.Stat(destPath); err == nil {
			fmt.Fprintf(cmd.OutOrStdout(), "Model %q already downloaded at %s\n", name, destPath)
			alreadyPresent = true
		}

		if !alreadyPresent {
			fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s...\n  → %s\n", name, destPath)
			if err := downloadGGUF(downloadURL, destPath, cmd.OutOrStdout()); err != nil {
				_ = os.Remove(destPath)
				return fmt.Errorf("download failed: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "\nDone.")
		}

		// Persist to local model registry and, on a fresh install, claim
		// default-model so the user can use the model immediately.
		if db, svc, _, dbErr := openModelRegistryDB(cmd); dbErr == nil {
			defer db.Close()
			_ = svc.Create(ctx, &runtimetypes.ModelRegistryEntry{
				ID:        uuid.NewString(),
				Name:      name,
				SourceURL: downloadURL,
			})
			store := runtimetypes.New(db.WithoutTransaction())
			if cur, _ := getConfigKV(ctx, store, "default-model"); cur == "" {
				contenoxDir, _ := ResolveContenoxDir(cmd)
				workspaceID := ResolveWorkspaceID(contenoxDir)
				if err := clikv.WriteConfig(ctx, store, workspaceID, "default-model", name); err == nil {
					fmt.Fprintf(cmd.OutOrStdout(), "✓  default-model = %s\n", name)
				}
			}
		}
		return nil
	},
}

func downloadGGUF(url, destPath string, out io.Writer) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var written int64
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if total > 0 {
				pct := written * 100 / total
				fmt.Fprintf(out, "\r  %d MB / %d MB (%d%%)", written/1024/1024, total/1024/1024, pct)
			} else {
				fmt.Fprintf(out, "\r  %d MB downloaded", written/1024/1024)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	fmt.Fprintln(out)
	return f.Sync()
}

func init() {
	modelPullCmd.Flags().String("url", "", "Direct GGUF download URL (use with a model name as first argument)")
	modelCmd.AddCommand(modelPullCmd)
}

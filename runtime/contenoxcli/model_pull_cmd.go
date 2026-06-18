package contenoxcli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/backendservice"
	"github.com/contenox/runtime/runtime/internal/clikv"
	"github.com/contenox/runtime/runtime/modelregistry"
	"github.com/contenox/runtime/runtime/modelrepo"
	"github.com/contenox/runtime/runtime/runtimetypes"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

var modelPullCmd = &cobra.Command{
	Use:   "pull <name>",
	Short: "Download a model (GGUF or OpenVINO IR) for local inference.",
	Long: `Download a model from HuggingFace for local inference. GGUF models are stored
under ~/.contenox/models/llama/<name>/model.gguf; OpenVINO IR models (curated
names ending in -ov) are fetched as a multi-file repo into
~/.contenox/models/openvino/<name>/.

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

		var name, downloadURL, repo, toolProtocol string
		modelBackend := "llama" // GGUF single-file by default; --url pulls are GGUF
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
			modelBackend = d.BackendType()
			repo = d.Repo
			toolProtocol = d.ToolProtocol
		default:
			return cmd.Help()
		}

		// Deposit into the registered backend's models directory so `model pull`
		// and the catalog scanner agree — this honors a custom `backend add --url`
		// dir and the legacy flat "local" backend, not just the default per-type
		// layout (models/<type>/). Falls back to the default if none is registered.
		modelDir := localBackendModelDir(ctx, modelBackend, name)
		if err := os.MkdirAll(modelDir, 0755); err != nil {
			return fmt.Errorf("create model directory: %w", err)
		}

		if modelBackend == "openvino" {
			if repo == "" {
				return fmt.Errorf("openvino model %q has no source repo in the registry", name)
			}
			if _, err := os.Stat(filepath.Join(modelDir, "openvino_model.xml")); err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Model %q already downloaded at %s\n", name, modelDir)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Downloading OpenVINO IR %s (repo %s)...\n  → %s\n", name, repo, modelDir)
				if err := downloadOpenVINOIR(ctx, repo, modelDir, cmd.OutOrStdout()); err != nil {
					return fmt.Errorf("download failed: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Done.")
			}
			// Certified curated models declare their tool-call protocol; write it
			// into the model's profile so the local provider enables model-native
			// tool calls out of the box. Never overwrite a user-edited profile.
			if toolProtocol != "" {
				if err := writeOpenVINOToolProfile(modelDir, toolProtocol); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not write tool-call profile: %v\n", err)
				}
			}
		} else {
			destPath := filepath.Join(modelDir, "model.gguf")
			if _, err := os.Stat(destPath); err == nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Model %q already downloaded at %s\n", name, destPath)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Downloading %s...\n  → %s\n", name, destPath)
				if err := downloadGGUF(downloadURL, destPath, cmd.OutOrStdout()); err != nil {
					_ = os.Remove(destPath)
					return fmt.Errorf("download failed: %w", err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "\nDone.")
			}
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

// hfModelInfo is the subset of the Hugging Face Hub model-info API we need: the
// list of files in the repo.
type hfModelInfo struct {
	Siblings []struct {
		RFilename string `json:"rfilename"`
	} `json:"siblings"`
}

// downloadOpenVINOIR fetches every file of an OpenVINO IR repo from the Hugging
// Face Hub HTTP API (no Python, no git-lfs) into destDir, mirroring the repo
// layout, then verifies the IR entrypoint so the openvino catalog scanner finds
// <destDir>/openvino_model.xml.
func downloadOpenVINOIR(ctx context.Context, repo, destDir string, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://huggingface.co/api/models/"+repo, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HF model info HTTP %s for %s", resp.Status, repo)
	}
	var info hfModelInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("decode HF model info: %w", err)
	}
	if len(info.Siblings) == 0 {
		return fmt.Errorf("no files listed for repo %s", repo)
	}
	for _, s := range info.Siblings {
		if s.RFilename == "" {
			continue
		}
		dest := filepath.Join(destDir, filepath.FromSlash(s.RFilename))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		fmt.Fprintf(out, "  %s\n", s.RFilename)
		if err := downloadFile(ctx, "https://huggingface.co/"+repo+"/resolve/main/"+s.RFilename, dest); err != nil {
			return fmt.Errorf("download %s: %w", s.RFilename, err)
		}
	}
	if _, err := os.Stat(filepath.Join(destDir, "openvino_model.xml")); err != nil {
		return fmt.Errorf("repo %s did not yield openvino_model.xml (not an OpenVINO IR model?)", repo)
	}
	return nil
}

// writeOpenVINOToolProfile writes a minimal contenox-openvino.json declaring the
// model-native tool-call protocol, so the openvino provider enables tool calls.
// It does not overwrite an existing profile (a user may have customized it).
func writeOpenVINOToolProfile(modelDir, protocol string) error {
	path := filepath.Join(modelDir, "contenox-openvino.json")
	if _, err := os.Stat(path); err == nil {
		return nil // keep an existing (possibly user-edited) profile
	}
	body := fmt.Sprintf("{\n  \"tool_calls\": { \"protocol\": %q }\n}\n", protocol)
	return os.WriteFile(path, []byte(body), 0o644)
}

// downloadFile streams url to dest.
func downloadFile(ctx context.Context, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %s", resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return f.Sync()
}

// localBackendModelDir returns where to deposit a pulled model of the given local
// backend type so the catalog scanner finds it: the BaseURL of the first
// registered backend whose canonical type matches (covers the legacy flat "local"
// backend and custom --url dirs), else the default ~/.contenox/models/<type>/.
func localBackendModelDir(ctx context.Context, modelBackend, name string) string {
	want := modelrepo.CanonicalBackendType(modelBackend)
	if dbPath, err := globalDBPath(); err == nil {
		if db, err := OpenDBAt(ctx, dbPath); err == nil {
			defer db.Close()
			if backends, err := backendservice.New(db).List(ctx, nil, 1000); err == nil {
				for _, b := range backends {
					if b.BaseURL != "" && modelrepo.CanonicalBackendType(b.Type) == want {
						return filepath.Join(b.BaseURL, name)
					}
				}
			}
		}
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".contenox", "models", modelBackend, name)
}

func init() {
	modelPullCmd.Flags().String("url", "", "Direct GGUF download URL (use with a model name as first argument)")
	modelCmd.AddCommand(modelPullCmd)
}

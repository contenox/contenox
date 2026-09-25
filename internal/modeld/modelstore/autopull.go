package modelstore

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/contenox/contenox/internal/archiveutil"
	"github.com/contenox/contenox/internal/modeld/llama"
	"github.com/contenox/contenox/internal/models/modelregistry"
	"github.com/contenox/contenox/libtracker"
)

var (
	pullMu       sync.Mutex
	pullInFlight = map[string]chan struct{}{}
)

// AutoPullOptions carries options for EnsureModelAvailable.
type AutoPullOptions struct {
	DataRoot    string
	ProgressOut io.Writer
	Tracker     libtracker.ActivityTracker
}

// EnsureModelAvailable checks if a model exists locally under dataRoot/models/<modelName>.
// If missing, it resolves the model from the modelregistry.Registry (curated or custom)
// and automatically downloads it into the local model store.
func EnsureModelAvailable(ctx context.Context, reg modelregistry.Registry, modelName string, opts AutoPullOptions) error {
	if err := validateModelName(modelName); err != nil {
		return err
	}

	dataRoot := opts.DataRoot
	if dataRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dataRoot = filepath.Join(home, ".contenox")
	}
	modelsDir := Dir(dataRoot, "")

	// 1. Check if already present on disk
	if _, err := Resolve(modelsDir, modelName, "llama", ""); err == nil {
		return nil
	}
	if _, err := Resolve(modelsDir, modelName, "openvino", ""); err == nil {
		return nil
	}

	// 2. Concurrency synchronization on the model name
	pullMu.Lock()
	if ch, busy := pullInFlight[modelName]; busy {
		pullMu.Unlock()
		select {
		case <-ch:
			// Done downloading; check if model is now resolved
			if _, err := Resolve(modelsDir, modelName, "llama", ""); err == nil {
				return nil
			}
			if _, err := Resolve(modelsDir, modelName, "openvino", ""); err == nil {
				return nil
			}
			return fmt.Errorf("%w: %s was downloaded by another process but could not be resolved", ErrModelNotFound, modelName)
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	doneCh := make(chan struct{})
	pullInFlight[modelName] = doneCh
	pullMu.Unlock()

	defer func() {
		pullMu.Lock()
		delete(pullInFlight, modelName)
		close(doneCh)
		pullMu.Unlock()
	}()

	// 3. Resolve descriptor from registry
	if reg == nil {
		reg = modelregistry.New(nil)
	}
	desc, err := reg.Resolve(ctx, modelName)
	if err != nil {
		optimal, optErr := reg.OptimalFor(ctx, modelName)
		if optErr == nil && optimal != "" {
			desc, err = reg.Resolve(ctx, optimal)
		}
	}
	if err != nil {
		return fmt.Errorf("%w: %s (not found in curated or local registry)", ErrModelNotFound, modelName)
	}

	// 4. Check disk free space before download
	admin := NewAdmin(modelsDir)
	if stats, statErr := admin.DiskStats(ctx); statErr == nil && stats.FreeBytes > 0 {
		required := desc.SizeBytes
		if desc.MMProjSizeBytes > 0 {
			required += desc.MMProjSizeBytes
		}
		if required > 0 && stats.FreeBytes < required+(500*1024*1024) {
			return fmt.Errorf("insufficient disk space: need %d bytes, have %d free bytes", required, stats.FreeBytes)
		}
	}

	// 5. Download model
	out := opts.ProgressOut
	if out == nil {
		out = os.Stderr
	}

	destDir := filepath.Join(modelsDir, modelName)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create model directory: %w", err)
	}

	modelBackend := desc.BackendType()
	if modelBackend == "openvino" {
		if desc.Repo == "" {
			return fmt.Errorf("openvino model %q has no source repo in registry", modelName)
		}
		fmt.Fprintf(out, "JIT Auto-pulling OpenVINO IR model %q (repo %s)...\n", modelName, desc.Repo)
		if err := downloadOpenVINOIRRepo(ctx, desc.Repo, destDir, out); err != nil {
			return fmt.Errorf("auto-pull openvino model %q: %w", modelName, err)
		}
	} else {
		destFile := filepath.Join(destDir, "model.gguf")
		fmt.Fprintf(out, "JIT Auto-pulling model %q from %s...\n", modelName, desc.SourceURL)
		if err := DownloadFile(ctx, desc.SourceURL, destFile, out); err != nil {
			_ = os.Remove(destFile)
			return fmt.Errorf("auto-pull model %q: %w", modelName, err)
		}

		if desc.Vision && desc.MMProjURL != "" {
			destMMProj := filepath.Join(destDir, "mmproj.gguf")
			fmt.Fprintf(out, "\nJIT Auto-pulling multimodal vision projector from %s...\n", desc.MMProjURL)
			if err := DownloadFile(ctx, desc.MMProjURL, destMMProj, out); err != nil {
				fmt.Fprintf(out, "\nwarning: failed to download multimodal projector: %v\n", err)
			}
		}

		// The template is not optional the way the projector is: a model without
		// one renders its tools through llama.cpp's ChatML fallback and answers
		// in prose, which reads to a caller as a model that cannot call tools.
		if desc.ChatTemplateURL != "" {
			destTemplate := filepath.Join(destDir, llama.ChatTemplateFilename)
			fmt.Fprintf(out, "\nPulling chat template from %s...\n", desc.ChatTemplateURL)
			if err := downloadChatTemplate(ctx, desc.ChatTemplateURL, destTemplate, out); err != nil {
				_ = os.Remove(destFile)
				return fmt.Errorf("auto-pull model %q: chat template: %w", modelName, err)
			}
		}
	}

	// Write profile if certified
	if desc.ToolProtocol != "" || desc.ReasoningProtocol != "" {
		_ = WriteModelProfile(modelBackend, destDir, desc.ToolProtocol, desc.ReasoningProtocol, desc.ReasoningFormat)
	}

	fmt.Fprintf(out, "\n✓ Model %q ready.\n", modelName)
	return nil
}

// downloadChatTemplate fetches a model's chat template. Publishers ship it
// either as a template file or inside tokenizer_config.json, and both have to
// land as the template llama.cpp reads: a JSON body written verbatim would be
// handed to the Jinja engine as its program.
func downloadChatTemplate(ctx context.Context, srcURL, destPath string, progressW io.Writer) error {
	staged := destPath + ".part"
	if err := DownloadFile(ctx, srcURL, staged, progressW); err != nil {
		return err
	}
	defer func() { _ = os.Remove(staged) }()
	raw, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	template := extractChatTemplate(raw)
	if strings.TrimSpace(template) == "" {
		return fmt.Errorf("no chat template in the fetched %s", srcURL)
	}
	return os.WriteFile(destPath, []byte(template), 0o644)
}

// extractChatTemplate returns the template a fetched body carries: the body
// itself when it is a template, or the chat_template field of a tokenizer
// config. A config may also key templates by name, whose first entry is the one
// the model renders with.
func extractChatTemplate(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return ""
	}
	// A template may itself open with a brace (`{{ bos_token }}`), so this is a
	// JSON question only when the body parses as a tokenizer config object; a
	// body that does not is the template.
	var probe map[string]json.RawMessage
	if strings.HasPrefix(trimmed, "{") && json.Unmarshal(raw, &probe) == nil {
		return templateFromTokenizerConfig(raw)
	}
	return trimmed
}

func templateFromTokenizerConfig(raw []byte) string {
	var config struct {
		ChatTemplate json.RawMessage `json:"chat_template"`
	}
	if err := json.Unmarshal(raw, &config); err != nil || len(config.ChatTemplate) == 0 {
		return ""
	}
	var one string
	if err := json.Unmarshal(config.ChatTemplate, &one); err == nil {
		return one
	}
	var many map[string]string
	if err := json.Unmarshal(config.ChatTemplate, &many); err == nil {
		names := make([]string, 0, len(many))
		for name := range many {
			names = append(names, name)
		}
		sort.Strings(names)
		if len(names) > 0 {
			return many[names[0]]
		}
	}
	return ""
}

// DownloadFile downloads a file from srcURL to destPath using a temporary staging file.
func DownloadFile(ctx context.Context, srcURL, destPath string, progressW io.Writer) error {
	resp, err := modelDownload(ctx, srcURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	partPath := destPath + ".part"
	f, err := os.OpenFile(partPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(partPath)
	}()

	pw := &DownloadProgressWriter{
		Total: resp.ContentLength,
		Out:   progressW,
	}

	tr := io.TeeReader(resp.Body, pw)
	if _, err := io.Copy(f, tr); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(partPath, destPath)
}

// DownloadProgressWriter writes download progress to Out.
type DownloadProgressWriter struct {
	Total      int64
	Downloaded int64
	LastPrint  time.Time
	Out        io.Writer
}

func (pw *DownloadProgressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.Downloaded += int64(n)
	now := time.Now()
	if now.Sub(pw.LastPrint) >= 250*time.Millisecond || pw.Downloaded == pw.Total {
		pw.LastPrint = now
		if pw.Out != nil {
			if pw.Total > 0 {
				pct := float64(pw.Downloaded) / float64(pw.Total) * 100
				fmt.Fprintf(pw.Out, "\rDownloading: %.1f%% (%s / %s)...", pct, FormatSize(pw.Downloaded), FormatSize(pw.Total))
			} else {
				fmt.Fprintf(pw.Out, "\rDownloading: %s...", FormatSize(pw.Downloaded))
			}
		}
	}
	return n, nil
}

// FormatSize formats a byte count into a human-readable string.
func FormatSize(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for n >= div*unit && exp < 3 {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGT"[exp])
}

// WriteModelProfile writes local parsing protocols to a fresh profile JSON.
func WriteModelProfile(modelBackend, modelDir, toolProtocol, reasoningProtocol, reasoningFormat string) error {
	var fileName string
	switch strings.ToLower(strings.TrimSpace(modelBackend)) {
	case "llama":
		fileName = "contenox-llama.json"
	case "openvino":
		fileName = "contenox-openvino.json"
	default:
		return nil
	}

	path := filepath.Join(modelDir, fileName)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	profile := map[string]any{}
	if toolProtocol != "" {
		profile["tool_calls"] = map[string]any{"protocol": toolProtocol}
	}
	if reasoningProtocol != "" {
		reasoning := map[string]any{"protocol": reasoningProtocol}
		if reasoningFormat != "" {
			reasoning["format"] = reasoningFormat
		}
		profile["reasoning"] = reasoning
	}
	body, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

type hfModelInfo struct {
	Siblings []struct {
		RFilename string `json:"rfilename"`
	} `json:"siblings"`
}

func downloadOpenVINOIRRepo(ctx context.Context, repo, destDir string, out io.Writer) error {
	resp, err := modelDownload(ctx, huggingFaceOrigin+"/api/models/"+repo)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

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
		dest, err := archiveutil.SafeJoin(destDir, s.RFilename)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if out != nil {
			fmt.Fprintf(out, "  %s\n", s.RFilename)
		}
		if err := DownloadFile(ctx, "https://huggingface.co/"+repo+"/resolve/main/"+s.RFilename, dest, out); err != nil {
			return fmt.Errorf("download %s: %w", s.RFilename, err)
		}
	}
	return nil
}

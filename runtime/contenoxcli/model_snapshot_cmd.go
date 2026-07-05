package contenoxcli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	libdb "github.com/contenox/runtime/libdbexec"
	"github.com/contenox/runtime/libtracker"
	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	"github.com/spf13/cobra"
)

const modelSnapshotSchema = 1

var modelSnapshotCmd = &cobra.Command{
	Use:   "snapshot",
	Short: "Capture and restore local modeld session snapshots.",
	Long: `Capture and restore local modeld session snapshots.

This is a diagnostic surface for end-to-end tests. It drives the same
runtime-to-modeld transport methods used by the warm session cache, but stores
the snapshot in an explicit file so tests can run save/restore as separate CLI
invocations.

Examples:
  contenox model snapshot save qwen --type llama --out /tmp/qwen.snap.json --prefix @prompt.txt
  contenox model snapshot restore qwen --type llama --in /tmp/qwen.snap.json --expect-reused 1`,
}

var modelSnapshotSaveCmd = &cobra.Command{
	Use:   "save <model>",
	Short: "Open a modeld session, prefill text, and write a snapshot file.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(commandContext(cmd))
		opts, err := snapshotOptionsFromFlags(cmd, args[0], "")
		if err != nil {
			return err
		}
		if opts.outPath == "" {
			return fmt.Errorf("--out is required")
		}
		prefix, err := resolveInputFlagValue("--prefix", opts.prefix)
		if err != nil {
			return err
		}
		if prefix == "" {
			return fmt.Errorf("--prefix is required")
		}
		suffix, err := resolveInputFlagValue("--suffix", opts.suffix)
		if err != nil {
			return err
		}

		db, cleanup, err := snapshotDBForResolve(cmd, opts.path)
		if err != nil {
			return err
		}
		defer cleanup()

		ref, cfg, err := resolveSnapshotModel(ctx, db, opts)
		if err != nil {
			return err
		}
		manifest, err := buildSnapshotManifest(opts.backend, ref, cfg, prefix, suffix)
		if err != nil {
			return err
		}
		sess, err := modeldconn.OpenSession(ctx, ref, cfg)
		if err != nil {
			return fmt.Errorf("open modeld session: %w", err)
		}
		defer sess.Close()

		prefixStatus, err := sess.EnsurePrefix(ctx, transport.PrefixInput{Text: prefix, Manifest: manifest})
		if err != nil {
			return fmt.Errorf("ensure prefix: %w", err)
		}
		var suffixStatus *transport.SuffixStatus
		if suffix != "" {
			st, err := sess.PrefillSuffix(ctx, transport.SuffixInput{Text: suffix, Manifest: manifest})
			if err != nil {
				return fmt.Errorf("prefill suffix: %w", err)
			}
			suffixStatus = &st
		}
		snap, err := sess.Snapshot(ctx)
		if err != nil {
			return fmt.Errorf("snapshot: %w", err)
		}
		file := modelSnapshotFile{
			Schema:    modelSnapshotSchema,
			CreatedAt: time.Now().UTC(),
			Backend:   opts.backend,
			Model:     ref.Name,
			Path:      ref.Path,
			Digest:    ref.Digest,
			Config:    cfg,
			Prefix:    prefix,
			Suffix:    suffix,
			Snapshot:  snap,
		}
		if err := writeModelSnapshotFile(opts.outPath, file); err != nil {
			return err
		}
		report := snapshotReport("save", opts.outPath, file, snap, sess.ExplainContext())
		report.Prefix = &prefixStatus
		report.Suffix = suffixStatus
		return printSnapshotReport(cmd, report)
	},
}

var modelSnapshotRestoreCmd = &cobra.Command{
	Use:   "restore [model]",
	Short: "Restore a modeld session from a snapshot file.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := libtracker.WithNewRequestID(commandContext(cmd))
		inPath, _ := cmd.Flags().GetString("in")
		if inPath == "" {
			return fmt.Errorf("--in is required")
		}
		file, err := readModelSnapshotFile(inPath)
		if err != nil {
			return err
		}
		modelName := file.Model
		if len(args) > 0 {
			modelName = args[0]
		}
		opts, err := snapshotOptionsFromFlags(cmd, modelName, file.Backend)
		if err != nil {
			return err
		}
		opts.inPath = inPath
		if opts.path == "" {
			opts.path = file.Path
		}
		if opts.model == "" {
			return fmt.Errorf("model is required either as an argument or in the snapshot file")
		}

		db, cleanup, err := snapshotDBForResolve(cmd, opts.path)
		if err != nil {
			return err
		}
		defer cleanup()

		ref, cfg, err := resolveSnapshotModel(ctx, db, opts)
		if err != nil {
			return err
		}
		if cfgIsZero(cfg) {
			cfg = file.Config
		}
		if opts.contextTokens == 0 && file.Snapshot.NumCtx > 0 {
			cfg.NumCtx = file.Snapshot.NumCtx
		}
		if err := validateSnapshotForRef(file, ref, opts.backend); err != nil {
			return err
		}
		file.Config = cfg

		sess, err := modeldconn.OpenSession(ctx, ref, cfg)
		if err != nil {
			return fmt.Errorf("open modeld session: %w", err)
		}
		defer sess.Close()
		if err := sess.Restore(ctx, file.Snapshot); err != nil {
			return fmt.Errorf("restore snapshot: %w", err)
		}

		report := snapshotReport("restore", inPath, file, file.Snapshot, sess.ExplainContext())
		prefix := file.Prefix
		if opts.prefix != "" {
			prefix, err = resolveInputFlagValue("--prefix", opts.prefix)
			if err != nil {
				return err
			}
		}
		if prefix != "" {
			manifest := file.Snapshot.Manifest
			if opts.prefix != "" {
				suffixForManifest := file.Suffix
				if opts.suffix != "" {
					suffixForManifest, err = resolveInputFlagValue("--suffix", opts.suffix)
					if err != nil {
						return err
					}
				}
				manifest, err = buildSnapshotManifest(opts.backend, ref, cfg, prefix, suffixForManifest)
				if err != nil {
					return err
				}
			}
			st, err := sess.EnsurePrefix(ctx, transport.PrefixInput{Text: prefix, Manifest: manifest})
			if err != nil {
				return fmt.Errorf("verify prefix after restore: %w", err)
			}
			report.Prefix = &st
			expect, _ := cmd.Flags().GetInt("expect-reused")
			if expect > 0 && st.ReusedTokens < expect {
				return fmt.Errorf("restored prefix reused %d tokens, expected at least %d", st.ReusedTokens, expect)
			}
		}
		suffix := ""
		if opts.suffix != "" {
			suffix, err = resolveInputFlagValue("--suffix", opts.suffix)
			if err != nil {
				return err
			}
		}
		if suffix != "" {
			st, err := sess.PrefillSuffix(ctx, transport.SuffixInput{Text: suffix, Manifest: file.Snapshot.Manifest})
			if err != nil {
				return fmt.Errorf("prefill suffix after restore: %w", err)
			}
			report.Suffix = &st
		}
		report.Explain = sess.ExplainContext()
		return printSnapshotReport(cmd, report)
	},
}

type snapshotCommandOptions struct {
	model         string
	backend       string
	path          string
	prefix        string
	suffix        string
	outPath       string
	inPath        string
	contextTokens int
}

type modelSnapshotFile struct {
	Schema    int                       `json:"schema"`
	CreatedAt time.Time                 `json:"created_at"`
	Backend   string                    `json:"backend"`
	Model     string                    `json:"model"`
	Path      string                    `json:"path,omitempty"`
	Digest    string                    `json:"digest,omitempty"`
	Config    transport.Config          `json:"config"`
	Prefix    string                    `json:"prefix,omitempty"`
	Suffix    string                    `json:"suffix,omitempty"`
	Snapshot  transport.SessionSnapshot `json:"snapshot"`
}

type modelSnapshotReport struct {
	Operation      string                  `json:"operation"`
	File           string                  `json:"file"`
	Backend        string                  `json:"backend"`
	Model          string                  `json:"model"`
	Path           string                  `json:"path,omitempty"`
	Digest         string                  `json:"digest,omitempty"`
	Config         transport.Config        `json:"config"`
	SnapshotBytes  int                     `json:"snapshot_bytes"`
	StateBytes     int                     `json:"state_bytes"`
	ColdKVBlocks   int                     `json:"cold_kv_blocks"`
	ResidentTokens int                     `json:"resident_tokens"`
	PrefixTokens   int                     `json:"prefix_tokens"`
	ManifestDigest string                  `json:"manifest_digest,omitempty"`
	Explain        transport.ContextReport `json:"explain"`
	Prefix         *transport.PrefixStatus `json:"prefix,omitempty"`
	Suffix         *transport.SuffixStatus `json:"suffix,omitempty"`
}

func snapshotOptionsFromFlags(cmd *cobra.Command, modelName, fallbackBackend string) (snapshotCommandOptions, error) {
	backend, _ := cmd.Flags().GetString("type")
	backend = strings.TrimSpace(backend)
	if backend == "" {
		backend = fallbackBackend
	}
	if backend == "" {
		backend = modeldconn.Backend()
	}
	backend = strings.ToLower(backend)
	if backend != "llama" && backend != "openvino" {
		return snapshotCommandOptions{}, fmt.Errorf("--type must be llama or openvino")
	}
	path, _ := cmd.Flags().GetString("path")
	prefix, _ := cmd.Flags().GetString("prefix")
	suffix, _ := cmd.Flags().GetString("suffix")
	outPath, _ := cmd.Flags().GetString("out")
	contextTokens, err := snapshotContextFlag(cmd)
	if err != nil {
		return snapshotCommandOptions{}, err
	}
	return snapshotCommandOptions{
		model:         strings.TrimSpace(modelName),
		backend:       backend,
		path:          strings.TrimSpace(path),
		prefix:        prefix,
		suffix:        suffix,
		outPath:       strings.TrimSpace(outPath),
		contextTokens: contextTokens,
	}, nil
}

func snapshotContextFlag(cmd *cobra.Command) (int, error) {
	raw, _ := cmd.Flags().GetString("context")
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	n, err := parseContextSize(raw)
	if err != nil {
		return 0, fmt.Errorf("--context: %w", err)
	}
	return n, nil
}

func snapshotDBForResolve(cmd *cobra.Command, path string) (libdb.DBManager, func(), error) {
	if strings.TrimSpace(path) != "" {
		return nil, func() {}, nil
	}
	db, _, err := openBackendDB(cmd)
	if err != nil {
		return nil, nil, err
	}
	return db, func() { db.Close() }, nil
}

func resolveSnapshotModel(ctx context.Context, db libdb.DBManager, opts snapshotCommandOptions) (modeldconn.ModelRef, transport.Config, error) {
	path := opts.path
	if path == "" {
		if db == nil {
			return modeldconn.ModelRef{}, transport.Config{}, fmt.Errorf("--path is required when no backend DB is available")
		}
		resolved, err := resolveSnapshotModelPathFromInventory(ctx, db, opts.backend, opts.model)
		if err != nil {
			return modeldconn.ModelRef{}, transport.Config{}, err
		}
		path = resolved
	}
	switch opts.backend {
	case "llama":
		return resolveSnapshotLlama(opts.model, path, opts.contextTokens)
	case "openvino":
		return resolveSnapshotOpenVINO(opts.model, path, opts.contextTokens)
	default:
		return modeldconn.ModelRef{}, transport.Config{}, fmt.Errorf("unsupported snapshot backend %q", opts.backend)
	}
}

func resolveSnapshotModelPathFromInventory(ctx context.Context, db libdb.DBManager, backend, modelName string) (string, error) {
	if modelName == "" {
		return "", fmt.Errorf("model is required when --path is not set")
	}
	entries, err := localModelInventory(ctx, db)
	if err != nil {
		return "", err
	}
	var matches []localModelInventoryEntry
	for _, e := range entries {
		if e.Type == backend && e.Model == modelName {
			matches = append(matches, e)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("local %s model %q not found; pass --path or run 'contenox model local'", backend, modelName)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("local %s model %q is ambiguous; pass --path", backend, modelName)
	}
	return matches[0].Path, nil
}

func resolveSnapshotLlama(name, rawPath string, contextTokens int) (modeldconn.ModelRef, transport.Config, error) {
	modelPath, dir, err := normalizeLlamaSnapshotPath(rawPath)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, err
	}
	profile, err := readLlamaSnapshotProfile(dir)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, err
	}
	cfg := transport.Config{
		NumCtx:               profile.Runtime.NumCtx,
		NumBatch:             profile.Runtime.NumBatch,
		NumThreads:           profile.Runtime.NumThreads,
		NumGpuLayers:         profile.Runtime.NumGpuLayers,
		TensorSplit:          profile.Runtime.TensorSplit,
		FlashAttn:            profile.Runtime.FlashAttention,
		KVCacheType:          profile.Runtime.KVCacheType,
		PromptFormat:         firstSnapshotNonEmptyString(profile.Prompt.Format, "chatml"),
		PromptTemplateDigest: profile.Prompt.TemplateDigest,
		ReasoningFormat:      profile.Reasoning.Format,
	}
	if profile.Prompt.AddBOS != nil {
		cfg.DisableBOS = !*profile.Prompt.AddBOS
	}
	if cfg.NumBatch <= 0 {
		cfg.NumBatch = 512
	}
	if cfg.PromptTemplateDigest == "" {
		cfg.PromptTemplateDigest = llamaPromptTemplateDigest(cfg.PromptFormat)
	}
	if contextTokens > 0 {
		cfg.NumCtx = contextTokens
	}
	digest := strings.TrimSpace(profile.ModelDigest)
	if digest == "" {
		digest, err = snapshotFileSHA256(modelPath)
		if err != nil {
			return modeldconn.ModelRef{}, transport.Config{}, err
		}
	}
	adapters, err := resolveSnapshotAdapters(dir, profile.Adapters)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(dir)
	}
	return modeldconn.ModelRef{Name: name, Type: "llama", Digest: digest, Path: modelPath, Adapters: adapters}, cfg, nil
}

func resolveSnapshotOpenVINO(name, rawPath string, contextTokens int) (modeldconn.ModelRef, transport.Config, error) {
	dir, err := filepath.Abs(rawPath)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, err
	}
	if _, ok := openVINOModelEntrypointPath(dir); !ok {
		return modeldconn.ModelRef{}, transport.Config{}, fmt.Errorf("openvino model entrypoint not found in %s", dir)
	}
	profile, err := readOpenVINOSnapshotProfile(dir)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, err
	}
	digest, templateDigest := openVINOSnapshotIdentity(dir)
	cfg := transport.Config{
		NumCtx:               profile.ContextLength,
		PromptFormat:         "openvino-chat-template",
		PromptTemplateDigest: templateDigest,
	}
	if contextTokens > 0 {
		cfg.NumCtx = contextTokens
	}
	adapters, err := resolveSnapshotAdapters(dir, profile.Adapters)
	if err != nil {
		return modeldconn.ModelRef{}, transport.Config{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = filepath.Base(dir)
	}
	return modeldconn.ModelRef{Name: name, Type: "openvino", Digest: digest, Path: dir, Adapters: adapters}, cfg, nil
}

func normalizeLlamaSnapshotPath(rawPath string) (modelPath, dir string, err error) {
	path, err := filepath.Abs(rawPath)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", "", fmt.Errorf("llama model path %s: %w", path, err)
	}
	if info.IsDir() {
		dir = path
		modelPath = filepath.Join(path, "model.gguf")
	} else {
		modelPath = path
		dir = filepath.Dir(path)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return "", "", fmt.Errorf("llama model file %s: %w", modelPath, err)
	}
	return modelPath, dir, nil
}

type snapshotAdapterProfile struct {
	Name   string   `json:"name,omitempty"`
	Path   string   `json:"path,omitempty"`
	Digest string   `json:"digest,omitempty"`
	Scale  *float32 `json:"scale,omitempty"`
}

type llamaSnapshotProfile struct {
	ModelDigest string                   `json:"model_digest,omitempty"`
	Adapters    []snapshotAdapterProfile `json:"adapters,omitempty"`
	Prompt      struct {
		Format         string `json:"format,omitempty"`
		TemplateDigest string `json:"template_digest,omitempty"`
		AddBOS         *bool  `json:"add_bos,omitempty"`
	} `json:"prompt,omitempty"`
	Runtime struct {
		NumCtx         int       `json:"num_ctx,omitempty"`
		NumBatch       int       `json:"num_batch,omitempty"`
		NumThreads     int       `json:"num_threads,omitempty"`
		NumGpuLayers   int       `json:"num_gpu_layers,omitempty"`
		TensorSplit    []float32 `json:"tensor_split,omitempty"`
		FlashAttention bool      `json:"flash_attention,omitempty"`
		KVCacheType    string    `json:"kv_cache_type,omitempty"`
	} `json:"runtime,omitempty"`
	Reasoning struct {
		Format string `json:"format,omitempty"`
	} `json:"reasoning,omitempty"`
}

type openVINOSnapshotProfile struct {
	ContextLength int                      `json:"context_length,omitempty"`
	Adapters      []snapshotAdapterProfile `json:"adapters,omitempty"`
}

func readLlamaSnapshotProfile(dir string) (llamaSnapshotProfile, error) {
	var profile llamaSnapshotProfile
	if err := readOptionalJSON(filepath.Join(dir, "contenox-llama.json"), &profile); err != nil {
		return llamaSnapshotProfile{}, err
	}
	return profile, nil
}

func readOpenVINOSnapshotProfile(dir string) (openVINOSnapshotProfile, error) {
	var profile openVINOSnapshotProfile
	if err := readOptionalJSON(filepath.Join(dir, "contenox-openvino.json"), &profile); err != nil {
		return openVINOSnapshotProfile{}, err
	}
	return profile, nil
}

func readOptionalJSON(path string, dst any) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	if err := dec.Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

func resolveSnapshotAdapters(profileDir string, adapters []snapshotAdapterProfile) ([]transport.AdapterSpec, error) {
	if len(adapters) == 0 {
		return nil, nil
	}
	out := make([]transport.AdapterSpec, 0, len(adapters))
	for i, adapter := range adapters {
		path := strings.TrimSpace(adapter.Path)
		if path == "" {
			return nil, fmt.Errorf("adapter[%d] is missing path", i)
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(profileDir, path)
		}
		digest := strings.TrimSpace(adapter.Digest)
		if digest == "" {
			var err error
			digest, err = snapshotFileSHA256(path)
			if err != nil {
				return nil, err
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

func buildSnapshotManifest(backend string, ref modeldconn.ModelRef, cfg transport.Config, prefix, suffix string) (transport.ContextManifest, error) {
	if prefix == "" {
		return transport.ContextManifest{}, fmt.Errorf("snapshot prefix must not be empty")
	}
	cfg = normalizeSnapshotConfig(backend, cfg)
	return contextasm.BuildSplitManifest(prefix, suffix, nil, contextasm.ManifestIdentity{
		ProfileID:            ref.Name,
		Backend:              backend,
		ModelDigest:          ref.Digest,
		PromptFormat:         cfg.PromptFormat,
		PromptTemplateDigest: cfg.PromptTemplateDigest,
		RuntimeDigest:        snapshotRuntimeDigest(backend, cfg, ref.Adapters),
		AddBOS:               !cfg.DisableBOS,
	})
}

func normalizeSnapshotConfig(backend string, cfg transport.Config) transport.Config {
	switch backend {
	case "llama":
		if cfg.NumBatch <= 0 {
			cfg.NumBatch = 512
		}
		if cfg.PromptFormat == "" {
			cfg.PromptFormat = "chatml"
		}
		if cfg.PromptTemplateDigest == "" {
			cfg.PromptTemplateDigest = llamaPromptTemplateDigest(cfg.PromptFormat)
		}
	case "openvino":
		if cfg.PromptFormat == "" {
			cfg.PromptFormat = "openvino-chat-template"
		}
	}
	return cfg
}

func snapshotRuntimeDigest(backend string, cfg transport.Config, adapters []transport.AdapterSpec) string {
	cfg = normalizeSnapshotConfig(backend, cfg)
	type adapterIdentity struct {
		Digest string  `json:"digest,omitempty"`
		Scale  float32 `json:"scale,omitempty"`
	}
	ids := make([]adapterIdentity, 0, len(adapters))
	for _, a := range adapters {
		ids = append(ids, adapterIdentity{Digest: a.Digest, Scale: a.Scale})
	}
	var payload any
	switch backend {
	case "llama":
		payload = struct {
			NumCtx                  int               `json:"num_ctx"`
			PlannerEffectiveContext int               `json:"planner_effective_context,omitempty"`
			NumBatch                int               `json:"num_batch"`
			NumThreads              int               `json:"num_threads"`
			NumGpuLayers            int               `json:"num_gpu_layers"`
			TensorSplit             []float32         `json:"tensor_split,omitempty"`
			FlashAttn               bool              `json:"flash_attention"`
			KVCacheType             string            `json:"kv_cache_type,omitempty"`
			Reasoning               string            `json:"reasoning,omitempty"`
			Adapters                []adapterIdentity `json:"adapters,omitempty"`
		}{cfg.NumCtx, cfg.PlannerEffectiveContext, cfg.NumBatch, cfg.NumThreads, cfg.NumGpuLayers, cfg.TensorSplit, cfg.FlashAttn, cfg.KVCacheType, cfg.ReasoningFormat, ids}
	default:
		payload = struct {
			NumCtx                  int               `json:"num_ctx"`
			PlannerEffectiveContext int               `json:"planner_effective_context,omitempty"`
			Format                  string            `json:"prompt_format"`
			Adapters                []adapterIdentity `json:"adapters,omitempty"`
		}{cfg.NumCtx, cfg.PlannerEffectiveContext, cfg.PromptFormat, ids}
	}
	b, _ := json.Marshal(payload)
	return contextasm.HashBytes(b)
}

func llamaPromptTemplateDigest(format string) string {
	switch format {
	case "", "chatml":
		return contextasm.HashString("llama-runtime-prompt-metadata:chatml:v1")
	case "llama3":
		return contextasm.HashString("llama-runtime-prompt-metadata:llama3:v1")
	default:
		return ""
	}
}

func openVINOSnapshotIdentity(modelDir string) (modelDigest, templateDigest string) {
	h := sha256.New()
	for _, name := range []string{"config.json", "tokenizer_config.json", "generation_config.json"} {
		if b, err := os.ReadFile(filepath.Join(modelDir, name)); err == nil {
			h.Write(b)
		}
	}
	modelDigest = hex.EncodeToString(h.Sum(nil))
	if b, err := os.ReadFile(filepath.Join(modelDir, "tokenizer_config.json")); err == nil {
		var cfg struct {
			ChatTemplate json.RawMessage `json:"chat_template"`
		}
		if json.Unmarshal(b, &cfg) == nil && len(cfg.ChatTemplate) > 0 {
			templateDigest = contextasm.HashString(string(cfg.ChatTemplate))
		}
	}
	return modelDigest, templateDigest
}

func snapshotFileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash %s: %w", path, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeModelSnapshotFile(path string, file modelSnapshotFile) error {
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot file: %w", err)
	}
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create snapshot directory: %w", err)
		}
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write snapshot file %s: %w", path, err)
	}
	return nil
}

func readModelSnapshotFile(path string) (modelSnapshotFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return modelSnapshotFile{}, fmt.Errorf("read snapshot file %s: %w", path, err)
	}
	var file modelSnapshotFile
	if err := json.Unmarshal(data, &file); err != nil {
		return modelSnapshotFile{}, fmt.Errorf("decode snapshot file %s: %w", path, err)
	}
	if file.Schema != modelSnapshotSchema {
		return modelSnapshotFile{}, fmt.Errorf("snapshot file schema = %d, want %d", file.Schema, modelSnapshotSchema)
	}
	if file.Snapshot.Manifest.IsZero() {
		return modelSnapshotFile{}, fmt.Errorf("snapshot file contains no manifest")
	}
	return file, nil
}

func validateSnapshotForRef(file modelSnapshotFile, ref modeldconn.ModelRef, backend string) error {
	if file.Backend != "" && file.Backend != backend {
		return fmt.Errorf("snapshot backend %q does not match requested backend %q", file.Backend, backend)
	}
	if file.Snapshot.Manifest.Backend != "" && file.Snapshot.Manifest.Backend != backend {
		return fmt.Errorf("snapshot manifest backend %q does not match requested backend %q", file.Snapshot.Manifest.Backend, backend)
	}
	if file.Digest != "" && ref.Digest != "" && file.Digest != ref.Digest {
		return fmt.Errorf("snapshot digest %q does not match current model digest %q", file.Digest, ref.Digest)
	}
	if file.Snapshot.Manifest.ModelDigest != "" && ref.Digest != "" && file.Snapshot.Manifest.ModelDigest != ref.Digest {
		return fmt.Errorf("snapshot manifest model digest %q does not match current model digest %q", file.Snapshot.Manifest.ModelDigest, ref.Digest)
	}
	return nil
}

func snapshotReport(op, path string, file modelSnapshotFile, snap transport.SessionSnapshot, explain transport.ContextReport) modelSnapshotReport {
	blob, _ := json.Marshal(snap)
	return modelSnapshotReport{
		Operation:      op,
		File:           path,
		Backend:        file.Backend,
		Model:          file.Model,
		Path:           file.Path,
		Digest:         file.Digest,
		Config:         file.Config,
		SnapshotBytes:  len(blob),
		StateBytes:     len(snap.State),
		ColdKVBlocks:   len(snap.ColdKVBlocks),
		ResidentTokens: snap.ResidentTokens,
		PrefixTokens:   snap.PrefixTokens,
		ManifestDigest: snap.Manifest.Digest(),
		Explain:        explain,
	}
}

func printSnapshotReport(cmd *cobra.Command, report modelSnapshotReport) error {
	asJSON, _ := cmd.Flags().GetBool("json")
	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(report)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s snapshot: %s\n", report.Operation, report.File)
	fmt.Fprintf(cmd.OutOrStdout(), "model: %s/%s\n", report.Backend, report.Model)
	fmt.Fprintf(cmd.OutOrStdout(), "resident_tokens: %d\n", report.ResidentTokens)
	fmt.Fprintf(cmd.OutOrStdout(), "prefix_tokens: %d\n", report.PrefixTokens)
	fmt.Fprintf(cmd.OutOrStdout(), "state_bytes: %d\n", report.StateBytes)
	fmt.Fprintf(cmd.OutOrStdout(), "cold_kv_blocks: %d\n", report.ColdKVBlocks)
	if report.Prefix != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "prefix_reused: %d\n", report.Prefix.ReusedTokens)
		fmt.Fprintf(cmd.OutOrStdout(), "prefix_prefilled: %d\n", report.Prefix.PrefilledTokens)
	}
	if report.Suffix != nil {
		fmt.Fprintf(cmd.OutOrStdout(), "suffix_tokens: %d\n", report.Suffix.SuffixTokens)
	}
	return nil
}

func commandContext(cmd *cobra.Command) context.Context {
	if ctx := cmd.Context(); ctx != nil {
		return ctx
	}
	return context.Background()
}

func cfgIsZero(cfg transport.Config) bool {
	return cfg.NumCtx == 0 &&
		cfg.HotContextTokens == 0 &&
		cfg.PlannerEffectiveContext == 0 &&
		cfg.NumBatch == 0 &&
		cfg.NumThreads == 0 &&
		cfg.NumGpuLayers == 0 &&
		len(cfg.TensorSplit) == 0 &&
		!cfg.FlashAttn &&
		cfg.KVCacheType == "" &&
		cfg.PromptFormat == "" &&
		cfg.PromptTemplateDigest == "" &&
		!cfg.DisableBOS &&
		cfg.ReasoningFormat == ""
}

func firstSnapshotNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func init() {
	for _, cmd := range []*cobra.Command{modelSnapshotSaveCmd, modelSnapshotRestoreCmd} {
		cmd.Flags().String("type", "", "Local modeld backend type: llama or openvino (default: running modeld backend)")
		cmd.Flags().String("path", "", "Resolved local model path (llama GGUF file/model dir, or OpenVINO IR dir)")
		cmd.Flags().String("prefix", "", "Stable prefix text, or @file")
		cmd.Flags().String("suffix", "", "Optional volatile suffix text, or @file")
		cmd.Flags().String("context", "", "Context window to request: bare int or shorthand (12k, 128k)")
		cmd.Flags().Bool("json", false, "Print machine-readable JSON")
	}
	modelSnapshotSaveCmd.Flags().String("out", "", "Snapshot file to write")
	modelSnapshotRestoreCmd.Flags().String("in", "", "Snapshot file to restore")
	modelSnapshotRestoreCmd.Flags().Int("expect-reused", 0, "Fail unless restore verification reuses at least this many prefix tokens")
	modelSnapshotCmd.AddCommand(modelSnapshotSaveCmd, modelSnapshotRestoreCmd)
	modelCmd.AddCommand(modelSnapshotCmd)
}

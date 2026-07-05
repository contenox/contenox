package contenoxcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
	"github.com/spf13/cobra"
)

func TestUnit_ModelSnapshotManifestAndFileRoundTrip(t *testing.T) {
	ref := modeldconn.ModelRef{Name: "tiny", Type: "llama", Digest: "digest-a", Path: "/models/tiny/model.gguf"}
	cfg := transport.Config{NumCtx: 128, NumBatch: 512, PromptFormat: "chatml", PromptTemplateDigest: llamaPromptTemplateDigest("chatml")}
	manifest, err := buildSnapshotManifest("llama", ref, cfg, "hello", " world")
	if err != nil {
		t.Fatalf("buildSnapshotManifest: %v", err)
	}
	if manifest.Backend != "llama" || manifest.ModelDigest != "digest-a" || manifest.StableByteHash != contextasm.HashString("hello") {
		t.Fatalf("manifest identity = %+v", manifest)
	}
	if manifest.StableBytes != len("hello") || manifest.TotalBytes != len("hello world") {
		t.Fatalf("manifest byte counts = stable %d total %d", manifest.StableBytes, manifest.TotalBytes)
	}

	file := modelSnapshotFile{
		Schema:  modelSnapshotSchema,
		Backend: "llama",
		Model:   "tiny",
		Path:    ref.Path,
		Digest:  ref.Digest,
		Config:  cfg,
		Prefix:  "hello",
		Suffix:  " world",
		Snapshot: transport.SessionSnapshot{
			ResidentTokens: 5,
			PrefixTokens:   5,
			NumCtx:         128,
			Manifest:       manifest,
		},
	}
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := writeModelSnapshotFile(path, file); err != nil {
		t.Fatalf("writeModelSnapshotFile: %v", err)
	}
	got, err := readModelSnapshotFile(path)
	if err != nil {
		t.Fatalf("readModelSnapshotFile: %v", err)
	}
	if got.Model != file.Model || got.Snapshot.Manifest.Digest() != file.Snapshot.Manifest.Digest() {
		t.Fatalf("round trip mismatch: got %+v want %+v", got, file)
	}
}

func TestUnit_ModelSnapshotResolveLlamaProfileAndContextOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.gguf"), []byte("model-bytes"), 0o600); err != nil {
		t.Fatalf("write model: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "adapter.bin"), []byte("adapter-bytes"), 0o600); err != nil {
		t.Fatalf("write adapter: %v", err)
	}
	profile := `{"model_digest":"profile-digest","runtime":{"num_ctx":64},"prompt":{"format":"llama3"},"adapters":[{"name":"a","path":"adapter.bin","scale":0.5}]}`
	if err := os.WriteFile(filepath.Join(dir, "contenox-llama.json"), []byte(profile), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	ref, cfg, err := resolveSnapshotLlama("tiny", dir, 32)
	if err != nil {
		t.Fatalf("resolveSnapshotLlama: %v", err)
	}
	if ref.Name != "tiny" || ref.Type != "llama" || ref.Digest != "profile-digest" || !strings.HasSuffix(ref.Path, "model.gguf") {
		t.Fatalf("ref = %+v", ref)
	}
	if cfg.NumCtx != 32 || cfg.NumBatch != 512 || cfg.PromptFormat != "llama3" || cfg.PromptTemplateDigest == "" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if len(ref.Adapters) != 1 || ref.Adapters[0].Name != "a" || ref.Adapters[0].Scale != 0.5 || ref.Adapters[0].Digest == "" {
		t.Fatalf("adapters = %+v", ref.Adapters)
	}
	if !filepath.IsAbs(ref.Adapters[0].Path) {
		t.Fatalf("adapter path should be absolute: %+v", ref.Adapters[0])
	}
}

func TestUnit_ModelSnapshotResolveOpenVINOProfileAndIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "openvino_model.xml"), []byte("<xml/>"), 0o600); err != nil {
		t.Fatalf("write entrypoint: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"model":"tiny"}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tokenizer_config.json"), []byte(`{"chat_template":"hello {{ messages }}"}`), 0o600); err != nil {
		t.Fatalf("write tokenizer config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "contenox-openvino.json"), []byte(`{"context_length":96}`), 0o600); err != nil {
		t.Fatalf("write profile: %v", err)
	}
	ref, cfg, err := resolveSnapshotOpenVINO("", dir, 48)
	if err != nil {
		t.Fatalf("resolveSnapshotOpenVINO: %v", err)
	}
	if ref.Name != filepath.Base(dir) || ref.Type != "openvino" || ref.Path != dir || ref.Digest == "" {
		t.Fatalf("ref = %+v", ref)
	}
	if cfg.NumCtx != 48 || cfg.PromptFormat != "openvino-chat-template" || cfg.PromptTemplateDigest == "" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestUnit_ModelSnapshotRejectsDigestMismatch(t *testing.T) {
	file := modelSnapshotFile{
		Schema:  modelSnapshotSchema,
		Backend: "llama",
		Digest:  "old-digest",
		Snapshot: transport.SessionSnapshot{Manifest: transport.ContextManifest{
			Backend:     "llama",
			ModelDigest: "old-digest",
		}},
	}
	ref := modeldconn.ModelRef{Name: "tiny", Type: "llama", Digest: "new-digest"}
	if err := validateSnapshotForRef(file, ref, "llama"); err == nil {
		t.Fatal("expected digest mismatch error")
	}
}

func TestUnit_ModelSnapshotRejectsBackendMismatch(t *testing.T) {
	file, ref, cfg := testSnapshotFile(t, "stable", " volatile")
	file.Backend = "openvino"
	file.Snapshot.Manifest.Backend = "openvino"
	if err := validateSnapshotForSession(file, ref, "llama", cfg); err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("expected backend mismatch, got %v", err)
	}
}

func TestUnit_ModelSnapshotRejectsMalformedSnapshotCounts(t *testing.T) {
	tests := []struct {
		name string
		edit func(*modelSnapshotFile)
		want string
	}{
		{name: "negative context", edit: func(f *modelSnapshotFile) { f.Snapshot.NumCtx = -1 }, want: "num_ctx"},
		{name: "negative resident", edit: func(f *modelSnapshotFile) { f.Snapshot.ResidentTokens = -1 }, want: "resident_tokens"},
		{name: "negative prefix", edit: func(f *modelSnapshotFile) { f.Snapshot.PrefixTokens = -1 }, want: "prefix_tokens"},
		{name: "prefix exceeds resident", edit: func(f *modelSnapshotFile) { f.Snapshot.PrefixTokens = f.Snapshot.ResidentTokens + 1 }, want: "exceeds resident_tokens"},
		{name: "resident exceeds context", edit: func(f *modelSnapshotFile) { f.Snapshot.ResidentTokens = f.Snapshot.NumCtx + 1 }, want: "exceeds num_ctx"},
		{name: "negative stable bytes", edit: func(f *modelSnapshotFile) { f.Snapshot.Manifest.StableBytes = -1 }, want: "stable_bytes"},
		{name: "negative total bytes", edit: func(f *modelSnapshotFile) { f.Snapshot.Manifest.TotalBytes = -1 }, want: "total_bytes"},
		{name: "total below stable", edit: func(f *modelSnapshotFile) { f.Snapshot.Manifest.TotalBytes = f.Snapshot.Manifest.StableBytes - 1 }, want: "less than stable_bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, ref, cfg := testSnapshotFile(t, "stable", " volatile")
			tt.edit(&file)
			err := validateSnapshotForSession(file, ref, "llama", cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateSnapshotForSession error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestUnit_ModelSnapshotRejectsContextMismatchBeforeOpen(t *testing.T) {
	file, ref, cfg := testSnapshotFile(t, "stable", "")
	cfg.NumCtx = file.Snapshot.NumCtx / 2
	err := validateSnapshotForSession(file, ref, "llama", cfg)
	if err == nil || !strings.Contains(err.Error(), "does not match requested context") {
		t.Fatalf("expected context mismatch, got %v", err)
	}
}

func TestUnit_ModelSnapshotRejectsStoredTextManifestMismatch(t *testing.T) {
	tests := []struct {
		name string
		edit func(*modelSnapshotFile)
		want string
	}{
		{name: "stable bytes", edit: func(f *modelSnapshotFile) { f.Snapshot.Manifest.StableBytes++ }, want: "stable_bytes"},
		{name: "stable hash", edit: func(f *modelSnapshotFile) { f.Prefix = "STABLE" }, want: "stable byte hash"},
		{name: "total bytes", edit: func(f *modelSnapshotFile) { f.Suffix = "changed" }, want: "total_bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, ref, cfg := testSnapshotFile(t, "stable", " volatile")
			tt.edit(&file)
			err := validateSnapshotForSession(file, ref, "llama", cfg)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validateSnapshotForSession error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestUnit_ModelSnapshotReadRejectsBadFiles(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "invalid json", data: `{`, want: "decode snapshot file"},
		{name: "bad schema", data: `{"schema":2}`, want: "schema"},
		{name: "missing manifest", data: `{"schema":1,"snapshot":{}}`, want: "contains no manifest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snap.json")
			if err := os.WriteFile(path, []byte(tt.data), 0o600); err != nil {
				t.Fatalf("write bad snapshot: %v", err)
			}
			_, err := readModelSnapshotFile(path)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("readModelSnapshotFile error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestUnit_ModelSnapshotWriteOverwritesWithPrivatePermissions(t *testing.T) {
	file, _, _ := testSnapshotFile(t, "stable", "")
	path := filepath.Join(t.TempDir(), "snap.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}
	if err := writeModelSnapshotFile(path, file); err != nil {
		t.Fatalf("writeModelSnapshotFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("snapshot permissions = %#o, want 0600", got)
	}
}

func TestUnit_ModelSnapshotContextFlagParsesShorthandAndRejectsInvalid(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("context", "", "")
	if err := cmd.Flags().Set("context", "12k"); err != nil {
		t.Fatalf("set context: %v", err)
	}
	got, err := snapshotContextFlag(cmd)
	if err != nil {
		t.Fatalf("snapshotContextFlag: %v", err)
	}
	if got != 12000 {
		t.Fatalf("context = %d, want 12000", got)
	}
	if err := cmd.Flags().Set("context", "12x"); err != nil {
		t.Fatalf("set context: %v", err)
	}
	if _, err := snapshotContextFlag(cmd); err == nil || !strings.Contains(err.Error(), "unknown suffix") {
		t.Fatalf("expected invalid suffix error, got %v", err)
	}
}

func TestUnit_ModelSnapshotInputFileFlag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prompt.txt")
	if err := os.WriteFile(path, []byte("from file"), 0o600); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	got, err := resolveInputFlagValue("--prefix", "@"+path)
	if err != nil {
		t.Fatalf("resolveInputFlagValue: %v", err)
	}
	if got != "from file" {
		t.Fatalf("got %q", got)
	}
}

func testSnapshotFile(t *testing.T, prefix, suffix string) (modelSnapshotFile, modeldconn.ModelRef, transport.Config) {
	t.Helper()
	ref := modeldconn.ModelRef{Name: "tiny", Type: "llama", Digest: "digest-a", Path: "/models/tiny/model.gguf"}
	cfg := transport.Config{NumCtx: 128, NumBatch: 512, PromptFormat: "chatml", PromptTemplateDigest: llamaPromptTemplateDigest("chatml")}
	manifest, err := buildSnapshotManifest("llama", ref, cfg, prefix, suffix)
	if err != nil {
		t.Fatalf("buildSnapshotManifest: %v", err)
	}
	resident := len([]rune(prefix + suffix))
	file := modelSnapshotFile{
		Schema:  modelSnapshotSchema,
		Backend: "llama",
		Model:   ref.Name,
		Path:    ref.Path,
		Digest:  ref.Digest,
		Config:  cfg,
		Prefix:  prefix,
		Suffix:  suffix,
		Snapshot: transport.SessionSnapshot{
			ResidentTokens: resident,
			PrefixTokens:   len([]rune(prefix)),
			NumCtx:         cfg.NumCtx,
			Manifest:       manifest,
		},
	}
	return file, ref, cfg
}

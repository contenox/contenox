package contenoxcli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/contenox/runtime/runtime/contextasm"
	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
	"github.com/contenox/runtime/runtime/transport"
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
	profile := `{"model_digest":"profile-digest","runtime":{"num_ctx":64},"prompt":{"format":"llama3"}}`
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

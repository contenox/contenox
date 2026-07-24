package llama

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/contenox/runtime/runtime/modelrepo/modeldconn"
)

// writeFile is a tiny test helper that writes content, creating parent dirs.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedBaseAndVariants lays out a base model plus two variants that reuse it, and
// returns the models root. Adapter files have distinct bytes so their content
// digests (and therefore variant identities) differ.
func seedBaseAndVariants(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "qwen3-coder-8b", "model.gguf"), "base-weights")
	writeFile(t, filepath.Join(root, "qwen3-coder-8b", profileFileName), `{"can_think": true}`)

	writeFile(t, filepath.Join(root, "qwen3-coder-8b-acme", "acme.gguf"), "adapter-acme-bytes")
	writeFile(t, filepath.Join(root, "qwen3-coder-8b-acme", variantFileName), `{
		"name": "qwen3-coder-8b-acme",
		"backend": "llama",
		"base_model": "qwen3-coder-8b",
		"adapters": [{"name": "acme-coding-style", "path": "acme.gguf"}]
	}`)

	writeFile(t, filepath.Join(root, "qwen3-coder-8b-blog", "blog.gguf"), "adapter-blog-bytes-differ")
	writeFile(t, filepath.Join(root, "qwen3-coder-8b-blog", variantFileName), `{
		"base_model": "qwen3-coder-8b",
		"adapters": [{"name": "blog-style", "path": "blog.gguf"}]
	}`)
	return root
}

// refFromSource mirrors how the provider/catalog build the ModelRef a session
// opens with: variant name for identity, the base model.gguf path, and the
// variant's adapters.
func refFromSource(t *testing.T, src modelSource) modeldconn.ModelRef {
	t.Helper()
	digest := src.profile.ModelDigest
	if digest == "" {
		var err error
		digest, err = modelFileDigest(src.modelPath)
		if err != nil {
			t.Fatal(err)
		}
	}
	return modeldconn.ModelRef{Name: src.name, Type: "llama", Digest: digest, Path: src.modelPath, Adapters: src.adapters}
}

func TestUnit_LlamaVariant_ResolvesDistinctIdentityFromBaseAndReusesWeights(t *testing.T) {
	root := seedBaseAndVariants(t)

	base, err := resolveModelSource(root, "qwen3-coder-8b")
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	acme, err := resolveModelSource(root, "qwen3-coder-8b-acme")
	if err != nil {
		t.Fatalf("resolve acme variant: %v", err)
	}
	blog, err := resolveModelSource(root, "qwen3-coder-8b-blog")
	if err != nil {
		t.Fatalf("resolve blog variant: %v", err)
	}

	if !acme.isVariant || !blog.isVariant || base.isVariant {
		t.Fatalf("isVariant flags wrong: base=%v acme=%v blog=%v", base.isVariant, acme.isVariant, blog.isVariant)
	}
	// No weight duplication: every variant reuses the base model.gguf in place.
	baseGGUF := filepath.Join(root, "qwen3-coder-8b", "model.gguf")
	for _, s := range []modelSource{base, acme, blog} {
		if s.modelPath != baseGGUF {
			t.Fatalf("model path = %q, want reused base gguf %q (no duplication)", s.modelPath, baseGGUF)
		}
	}
	if len(acme.adapters) != 1 || acme.adapters[0].Digest == "" {
		t.Fatalf("acme adapter not resolved with a digest: %+v", acme.adapters)
	}
	if acme.adapters[0].Scale != 1 {
		t.Fatalf("adapter scale = %v, want default 1.0", acme.adapters[0].Scale)
	}
	// Variant inherits the base runtime profile (can_think here).
	if !acme.profile.CanThink {
		t.Fatal("variant should inherit base profile can_think")
	}

	cfg := Config{NumCtx: 8192, NumBatch: 512}
	baseKey := sessionCacheKey(refFromSource(t, base), cfg)
	acmeKey := sessionCacheKey(refFromSource(t, acme), cfg)
	blogKey := sessionCacheKey(refFromSource(t, blog), cfg)

	// A variant, its base, and a different variant must all be cache-isolated:
	// warm KV for one must never satisfy another (adapter identity flows into the
	// session cache key). This is the load-bearing safety property.
	keys := map[string]string{"base": baseKey, "acme": acmeKey, "blog": blogKey}
	seen := map[string]string{}
	for label, key := range keys {
		if other, dup := seen[key]; dup {
			t.Fatalf("cache key collision: %q and %q share a key", label, other)
		}
		seen[key] = label
	}

	// The manifest runtime identity must diverge the same way.
	baseRD := runtimeDigest(cfg, base.adapters)
	acmeRD := runtimeDigest(cfg, acme.adapters)
	blogRD := runtimeDigest(cfg, blog.adapters)
	if baseRD == acmeRD || acmeRD == blogRD || baseRD == blogRD {
		t.Fatalf("runtimeDigest not distinct: base=%s acme=%s blog=%s", baseRD, acmeRD, blogRD)
	}
}

func TestUnit_LlamaVariant_RejectsInvalidMarkers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "base", "model.gguf"), "w")

	cases := []struct {
		name string
		json string
	}{
		{"missing_base", `{"adapters":[{"name":"a","path":"a.gguf"}]}`},
		{"no_adapters", `{"base_model":"base"}`},
		{"unknown_field", `{"base_model":"base","adapters":[{"name":"a","path":"a.gguf"}],"bogus":1}`},
		{"wrong_backend", `{"base_model":"base","backend":"openvino","adapters":[{"name":"a","path":"a.gguf"}]}`},
		{"traversal_base", `{"base_model":"../escape","adapters":[{"name":"a","path":"a.gguf"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, "v-"+tc.name)
			writeFile(t, filepath.Join(dir, "a.gguf"), "adapter")
			writeFile(t, filepath.Join(dir, variantFileName), tc.json)
			if _, err := resolveModelSource(root, "v-"+tc.name); err == nil {
				t.Fatalf("expected error for %s marker", tc.name)
			}
		})
	}
}

func TestUnit_LlamaVariant_RejectsMissingBaseModel(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "orphan")
	writeFile(t, filepath.Join(dir, "a.gguf"), "adapter")
	writeFile(t, filepath.Join(dir, variantFileName), `{"base_model":"nonexistent","adapters":[{"name":"a","path":"a.gguf"}]}`)

	_, err := resolveModelSource(root, "orphan")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("missing base model error = %v, want ErrUnsupportedFeature", err)
	}
}

func TestUnit_LlamaCatalog_EmitsVariantAsSelectableModel(t *testing.T) {
	withSessionFactory(t, func(string, Config) (Session, error) { return nil, nil })
	root := seedBaseAndVariants(t)

	models, err := (&catalogProvider{dir: root}).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	byName := map[string]bool{}
	variantFound := false
	variantCanThink := false
	variantCanEmbed := false
	for _, m := range models {
		byName[m.Name] = true
		if m.Name == "qwen3-coder-8b-acme" {
			variantFound = true
			variantCanThink = m.CanThink
			variantCanEmbed = m.CanEmbed
		}
	}
	for _, want := range []string{"qwen3-coder-8b", "qwen3-coder-8b-acme", "qwen3-coder-8b-blog"} {
		if !byName[want] {
			t.Fatalf("model %q missing from catalog: got %+v", want, byName)
		}
	}
	if !variantFound {
		t.Fatal("variant not emitted")
	}
	// Variant inherits base capabilities (can_think) but never advertises
	// embeddings (adapter-free by design).
	if !variantCanThink {
		t.Fatal("variant should inherit base can_think")
	}
	if variantCanEmbed {
		t.Fatal("variant must not advertise embeddings")
	}
}

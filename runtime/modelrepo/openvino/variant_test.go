package openvino

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

// seedBaseIR lays down the files that make dir a base OpenVINO IR: the language
// model entrypoint plus the config/tokenizer files modelIdentity hashes (so the
// base digest and template digest are non-empty).
func seedBaseIR(t *testing.T, dir, profileJSON string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "openvino_language_model.xml"), "<xml/>")
	writeFile(t, filepath.Join(dir, "config.json"), `{"max_position_embeddings":32768}`)
	writeFile(t, filepath.Join(dir, "tokenizer_config.json"), `{"chat_template":"{{ messages }}"}`)
	writeFile(t, filepath.Join(dir, "generation_config.json"), `{"eos_token_id":1}`)
	if profileJSON != "" {
		writeFile(t, filepath.Join(dir, profileFileName), profileJSON)
	}
}

// seedBaseAndVariants lays out a base OpenVINO IR plus two variants that reuse
// it, and returns the models root. Adapter files have distinct bytes so their
// content digests (and therefore variant identities) differ.
func seedBaseAndVariants(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	seedBaseIR(t, filepath.Join(root, "qwen3-8b-ov"), `{"can_think": true}`)

	writeFile(t, filepath.Join(root, "qwen3-8b-ov-acme", "acme.safetensors"), "adapter-acme-bytes")
	writeFile(t, filepath.Join(root, "qwen3-8b-ov-acme", variantFileName), `{
		"name": "qwen3-8b-ov-acme",
		"backend": "openvino",
		"base_model": "qwen3-8b-ov",
		"adapters": [{"name": "acme-coding-style", "path": "acme.safetensors"}]
	}`)

	writeFile(t, filepath.Join(root, "qwen3-8b-ov-blog", "blog.safetensors"), "adapter-blog-bytes-differ")
	writeFile(t, filepath.Join(root, "qwen3-8b-ov-blog", variantFileName), `{
		"base_model": "qwen3-8b-ov",
		"adapters": [{"name": "blog-style", "path": "blog.safetensors"}]
	}`)
	return root
}

// refFromSource mirrors how the provider/catalog build the ModelRef a session
// opens with: variant name for identity, the base IR directory path, the base
// digest, and the variant's adapters.
func refFromSource(src modelSource) modeldconn.ModelRef {
	return modeldconn.ModelRef{Name: src.name, Type: "openvino", Digest: src.modelDigest, Path: src.modelDir, Adapters: src.adapters}
}

func TestUnit_OpenVINOVariant_ResolvesDistinctIdentityFromBaseAndReusesIR(t *testing.T) {
	root := seedBaseAndVariants(t)

	base, err := resolveModelSource(root, "qwen3-8b-ov")
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	acme, err := resolveModelSource(root, "qwen3-8b-ov-acme")
	if err != nil {
		t.Fatalf("resolve acme variant: %v", err)
	}
	blog, err := resolveModelSource(root, "qwen3-8b-ov-blog")
	if err != nil {
		t.Fatalf("resolve blog variant: %v", err)
	}

	if !acme.isVariant || !blog.isVariant || base.isVariant {
		t.Fatalf("isVariant flags wrong: base=%v acme=%v blog=%v", base.isVariant, acme.isVariant, blog.isVariant)
	}
	// No IR duplication: every variant reuses the base IR directory in place.
	baseDir := filepath.Join(root, "qwen3-8b-ov")
	for _, s := range []modelSource{base, acme, blog} {
		if s.modelDir != baseDir {
			t.Fatalf("model dir = %q, want reused base IR dir %q (no duplication)", s.modelDir, baseDir)
		}
	}
	// All three content-address the SAME base IR (shared digest) — identity must
	// diverge only through the adapters, not the base weights.
	if base.modelDigest == "" || acme.modelDigest != base.modelDigest || blog.modelDigest != base.modelDigest {
		t.Fatalf("base digest not shared: base=%q acme=%q blog=%q", base.modelDigest, acme.modelDigest, blog.modelDigest)
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

	cfg := Config{NumCtx: 8192}
	baseKey := sessionCacheKey(refFromSource(base), cfg)
	acmeKey := sessionCacheKey(refFromSource(acme), cfg)
	blogKey := sessionCacheKey(refFromSource(blog), cfg)

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

func TestUnit_OpenVINOVariant_RejectsInvalidMarkers(t *testing.T) {
	root := t.TempDir()
	seedBaseIR(t, filepath.Join(root, "base"), "")

	cases := []struct {
		name string
		json string
	}{
		{"missing_base", `{"adapters":[{"name":"a","path":"a.safetensors"}]}`},
		{"no_adapters", `{"base_model":"base"}`},
		{"unknown_field", `{"base_model":"base","adapters":[{"name":"a","path":"a.safetensors"}],"bogus":1}`},
		{"wrong_backend", `{"base_model":"base","backend":"llama","adapters":[{"name":"a","path":"a.safetensors"}]}`},
		{"traversal_base", `{"base_model":"../escape","adapters":[{"name":"a","path":"a.safetensors"}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, "v-"+tc.name)
			writeFile(t, filepath.Join(dir, "a.safetensors"), "adapter")
			writeFile(t, filepath.Join(dir, variantFileName), tc.json)
			if _, err := resolveModelSource(root, "v-"+tc.name); err == nil {
				t.Fatalf("expected error for %s marker", tc.name)
			}
		})
	}
}

func TestUnit_OpenVINOVariant_RejectsMissingBaseModel(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "orphan")
	writeFile(t, filepath.Join(dir, "a.safetensors"), "adapter")
	writeFile(t, filepath.Join(dir, variantFileName), `{"base_model":"nonexistent","adapters":[{"name":"a","path":"a.safetensors"}]}`)

	_, err := resolveModelSource(root, "orphan")
	if !errors.Is(err, ErrUnsupportedFeature) {
		t.Fatalf("missing base model error = %v, want ErrUnsupportedFeature", err)
	}
}

func TestUnit_OpenVINOCatalog_EmitsVariantAsSelectableModel(t *testing.T) {
	oldFactory := sessionFactory
	sessionFactory = func(modeldconn.ModelRef, Config) (Session, error) { return nil, nil }
	t.Cleanup(func() { sessionFactory = oldFactory })

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
		if m.Name == "qwen3-8b-ov-acme" {
			variantFound = true
			variantCanThink = m.CanThink
			variantCanEmbed = m.CanEmbed
		}
	}
	for _, want := range []string{"qwen3-8b-ov", "qwen3-8b-ov-acme", "qwen3-8b-ov-blog"} {
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

// A stray marker targeting a DIFFERENT backend (both backends scan the same
// contenox-variant.json filename in different roots) must be skipped, never
// break the whole scan — the valid openvino models are still emitted.
func TestUnit_OpenVINOCatalog_SkipsForeignBackendMarker(t *testing.T) {
	oldFactory := sessionFactory
	sessionFactory = func(modeldconn.ModelRef, Config) (Session, error) { return nil, nil }
	t.Cleanup(func() { sessionFactory = oldFactory })

	root := seedBaseAndVariants(t)
	// A llama-backend variant marker dropped into the openvino root.
	writeFile(t, filepath.Join(root, "qwen3-8b-ov-foreign", "x.gguf"), "adapter")
	writeFile(t, filepath.Join(root, "qwen3-8b-ov-foreign", variantFileName), `{
		"backend": "llama",
		"base_model": "qwen3-8b-ov",
		"adapters": [{"name": "foreign", "path": "x.gguf"}]
	}`)

	models, err := (&catalogProvider{dir: root}).ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels must not fail on a foreign-backend marker: %v", err)
	}
	byName := map[string]bool{}
	for _, m := range models {
		byName[m.Name] = true
	}
	if byName["qwen3-8b-ov-foreign"] {
		t.Fatal("foreign-backend marker must not be emitted as an openvino model")
	}
	for _, want := range []string{"qwen3-8b-ov", "qwen3-8b-ov-acme", "qwen3-8b-ov-blog"} {
		if !byName[want] {
			t.Fatalf("valid model %q dropped from catalog after a foreign marker: got %+v", want, byName)
		}
	}
}

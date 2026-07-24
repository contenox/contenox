package llama

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// variantFileName is the convention marker that turns a directory under the
// models root into a selectable local model VARIANT: base model weights plus one
// or more LoRA adapters, with no duplicated base .gguf. Its presence (and the
// absence of model.gguf) is what the catalog scan recognizes — mirroring how
// model.gguf presence defines a base model. Layout:
//
//	<modelDir>/qwen3-coder-8b/model.gguf        (base model)
//	<modelDir>/qwen3-coder-8b/contenox-llama.json
//	<modelDir>/qwen3-coder-8b-acme/contenox-variant.json   (variant marker)
//	<modelDir>/qwen3-coder-8b-acme/acme.gguf               (adapter, or referenced elsewhere)
//
// Keeping the variant in a sibling directory under the same models root (rather
// than a separate variants namespace) reuses the existing scan, the existing
// ProviderFor dispatch, and needs no new root plumbing; it also keeps every
// resolved path inside the models root, honoring the control-plane isolation
// invariant.
const variantFileName = "contenox-variant.json"

// variantProfile is the on-disk variant marker (<modelDir>/<variant>/
// contenox-variant.json). It names a sibling base model — whose model.gguf is
// reused, never copied — and the adapters that make this a distinct model. The
// runtime profile (prompt template, reasoning/tool protocols, context, runtime
// knobs) is inherited from the base model's contenox-llama.json; the adapter set
// is what distinguishes the variant's identity.
type variantProfile struct {
	Name      string           `json:"name,omitempty"`
	Backend   string           `json:"backend,omitempty"`
	BaseModel string           `json:"base_model"`
	Adapters  []adapterProfile `json:"adapters,omitempty"`
}

func variantProfilePath(variantDir string) string {
	return filepath.Join(variantDir, variantFileName)
}

// loadVariantProfile reads and validates the variant marker in variantDir. The
// bool is false with a nil error when the directory holds no marker (i.e. it is
// not a variant); a present-but-invalid marker is a typed error, failing early
// per the blueprint's validation rules.
func loadVariantProfile(variantDir string) (variantProfile, bool, error) {
	path := variantProfilePath(variantDir)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return variantProfile{}, false, nil
		}
		return variantProfile{}, false, fmt.Errorf("llama variant open %s: %w", path, err)
	}
	defer f.Close()
	var v variantProfile
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return variantProfile{}, false, fmt.Errorf("llama variant decode %s: %w", path, err)
	}
	if b := strings.TrimSpace(v.Backend); b != "" && b != "llama" {
		return variantProfile{}, false, fmt.Errorf("%w: llama variant %s: backend %q is not llama", ErrUnsupportedFeature, path, v.Backend)
	}
	if strings.TrimSpace(v.BaseModel) == "" {
		return variantProfile{}, false, fmt.Errorf("%w: llama variant %s: base_model is required", ErrUnsupportedFeature, path)
	}
	if !safeModelName(v.BaseModel) {
		return variantProfile{}, false, fmt.Errorf("%w: llama variant %s: base_model %q must be a plain model name", ErrUnsupportedFeature, path, v.BaseModel)
	}
	if len(v.Adapters) == 0 {
		return variantProfile{}, false, fmt.Errorf("%w: llama variant %s: at least one adapter is required", ErrUnsupportedFeature, path)
	}
	return v, true, nil
}

// safeModelName rejects names that could escape the models root (path
// separators or traversal). Base-model directories are named by the filesystem
// scan and so are inherently safe, but a variant's base_model comes from JSON:
// it must be constrained to the same single-segment shape, because the
// control-plane isolation invariant forbids resolving arbitrary paths outside
// the models/adapters roots.
func safeModelName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}

// modelSource is the resolved on-disk backing for a selectable local model —
// either a base model (model.gguf directly in its own dir) or a variant (base
// model.gguf reused from a sibling dir, plus adapters). The catalog scan, the
// provider's session client, and the embed path all resolve through this, so a
// variant serves as a normal model against the base weights with its adapters
// attached and inherits the base's runtime profile.
type modelSource struct {
	name      string        // selectable model name (== the scanned directory name)
	baseName  string        // base model name (== name for a base model)
	baseDir   string        // directory holding model.gguf + contenox-llama.json
	modelPath string        // absolute path to the base model.gguf (reused, never copied)
	profile   modelProfile  // base model runtime profile (variants inherit it)
	adapters  []AdapterSpec // adapters for a variant (nil for a base model)
	isVariant bool
}

// resolveModelSource resolves the directory named name under modelDir into its
// backing model.gguf, runtime profile, and adapter set. A directory holding
// model.gguf is a base model (existing behavior, unchanged). A directory holding
// a contenox-variant.json marker is a variant: its base_model must be a sibling
// directory with a model.gguf, whose profile the variant inherits, and whose
// weights it reuses in place while attaching the variant's own adapters.
func resolveModelSource(modelDir, name string) (modelSource, error) {
	dir := filepath.Join(modelDir, name)
	modelPath := filepath.Join(dir, "model.gguf")
	// Dispatch on the variant marker's PRESENCE, not on model.gguf's absence: a
	// base model's local model.gguf may legitimately be absent when its profile
	// declares the digest and the daemon owns the artifact, so a base model is the
	// default and a variant is only ever the explicit marker (and only when it is
	// not shadowed by a real model.gguf).
	_, ggufErr := os.Stat(modelPath)
	if ggufErr != nil {
		if v, ok, verr := loadVariantProfile(dir); verr != nil {
			return modelSource{}, verr
		} else if ok {
			return resolveVariantSource(modelDir, dir, name, v)
		}
		// Not a variant: fall through to the base-model path, which does not
		// require model.gguf to exist locally (digest may come from the profile).
	}
	profile, err := loadModelProfile(dir)
	if err != nil {
		return modelSource{}, err
	}
	adapters, err := resolveProfileAdapters(dir, profile.Adapters)
	if err != nil {
		return modelSource{}, err
	}
	return modelSource{
		name:      name,
		baseName:  name,
		baseDir:   dir,
		modelPath: modelPath,
		profile:   profile,
		adapters:  adapters,
	}, nil
}

// resolveVariantSource resolves a validated variant marker against its base
// model under modelDir, reusing the base weights in place and attaching the
// variant's adapters.
func resolveVariantSource(modelDir, dir, name string, v variantProfile) (modelSource, error) {
	if v.Name != "" && v.Name != name {
		return modelSource{}, fmt.Errorf("%w: llama variant %s: name %q must match its directory %q", ErrUnsupportedFeature, variantProfilePath(dir), v.Name, name)
	}

	baseDir := filepath.Join(modelDir, v.BaseModel)
	baseModelPath := filepath.Join(baseDir, "model.gguf")
	if _, err := os.Stat(baseModelPath); err != nil {
		return modelSource{}, fmt.Errorf("%w: llama variant %q base model %q has no model.gguf at %s", ErrUnsupportedFeature, name, v.BaseModel, baseModelPath)
	}
	profile, err := loadModelProfile(baseDir)
	if err != nil {
		return modelSource{}, err
	}
	// Variant adapters are resolved relative to the variant directory (matching
	// the profile-adapter convention) and are authoritative: they, not any base
	// profile adapters, define the variant on top of the base weights.
	adapters, err := resolveProfileAdapters(dir, v.Adapters)
	if err != nil {
		return modelSource{}, err
	}
	return modelSource{
		name:      name,
		baseName:  v.BaseModel,
		baseDir:   baseDir,
		modelPath: baseModelPath,
		profile:   profile,
		adapters:  adapters,
		isVariant: true,
	}, nil
}

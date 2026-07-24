package openvino

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/contenox/runtime/runtime/transport"
)

// variantFileName is the convention marker that turns a directory under the
// models root into a selectable local model VARIANT: a base OpenVINO IR plus one
// or more safetensors LoRA adapters, with no duplicated base IR. Its presence
// (and the absence of an IR entrypoint) is what the catalog scan recognizes —
// mirroring how an IR entrypoint (openvino_model.xml / openvino_language_model.xml)
// defines a base model. Layout:
//
//	<modelDir>/qwen3-8b-ov/openvino_language_model.xml   (base IR entrypoint)
//	<modelDir>/qwen3-8b-ov/contenox-openvino.json        (base profile)
//	<modelDir>/qwen3-8b-ov-acme/contenox-variant.json    (variant marker)
//	<modelDir>/qwen3-8b-ov-acme/acme.safetensors         (adapter, or referenced elsewhere)
//
// Keeping the variant in a sibling directory under the same models root (rather
// than a separate variants namespace) reuses the existing scan, the existing
// ProviderFor dispatch, and needs no new root plumbing; it also keeps every
// resolved path inside the models root, honoring the control-plane isolation
// invariant. The filename is identical to the llama marker (per-backend roots
// differ); the marker's backend field selects the target backend.
const variantFileName = "contenox-variant.json"

// errVariantForeignBackend marks a contenox-variant.json whose backend targets a
// DIFFERENT backend than openvino. Both backends scan the same marker filename
// (in different roots), and the catalog scan skips such a stray foreign marker
// rather than breaking the whole scan — while resolveModelSource still surfaces
// it as an error so a misdirected marker is never silently served as a base.
var errVariantForeignBackend = errors.New("openvino: variant marker targets a different backend")

// variantProfile is the on-disk variant marker (<modelDir>/<variant>/
// contenox-variant.json). It names a sibling base model — whose OpenVINO IR is
// reused, never copied — and the adapters that make this a distinct model. The
// runtime profile (reasoning/tool protocols, context, runtime knobs) is
// inherited from the base model's contenox-openvino.json; the adapter set is
// what distinguishes the variant's identity.
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
// per the blueprint's validation rules. A marker that targets a foreign backend
// is reported with errVariantForeignBackend so the catalog can skip it.
func loadVariantProfile(variantDir string) (variantProfile, bool, error) {
	path := variantProfilePath(variantDir)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return variantProfile{}, false, nil
		}
		return variantProfile{}, false, fmt.Errorf("openvino variant open %s: %w", path, err)
	}
	defer f.Close()
	var v variantProfile
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return variantProfile{}, false, fmt.Errorf("openvino variant decode %s: %w", path, err)
	}
	if b := strings.TrimSpace(v.Backend); b != "" && b != "openvino" {
		return variantProfile{}, false, fmt.Errorf("%w: openvino variant %s: backend %q is not openvino", errVariantForeignBackend, path, v.Backend)
	}
	if strings.TrimSpace(v.BaseModel) == "" {
		return variantProfile{}, false, fmt.Errorf("%w: openvino variant %s: base_model is required", ErrUnsupportedFeature, path)
	}
	if !safeModelName(v.BaseModel) {
		return variantProfile{}, false, fmt.Errorf("%w: openvino variant %s: base_model %q must be a plain model name", ErrUnsupportedFeature, path, v.BaseModel)
	}
	if len(v.Adapters) == 0 {
		return variantProfile{}, false, fmt.Errorf("%w: openvino variant %s: at least one adapter is required", ErrUnsupportedFeature, path)
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
// either a base model (an OpenVINO IR entrypoint directly in its own dir) or a
// variant (base IR reused from a sibling dir, plus adapters). The catalog scan,
// the provider's session client, and the embed path all resolve through this,
// so a variant serves as a normal model against the base IR with its adapters
// attached and inherits the base's runtime profile and identity.
type modelSource struct {
	name           string                  // selectable model name (== the scanned directory name)
	baseName       string                  // base model name (== name for a base model)
	modelDir       string                  // directory holding the base IR (reused, never copied); ModelRef.Path
	profile        modelProfile            // base model runtime profile (variants inherit it)
	adapters       []transport.AdapterSpec // adapters for a variant (nil for a base model)
	modelDigest    string                  // content digest of the base IR
	templateDigest string                  // digest of the base model's own chat template
	isVariant      bool
}

// resolveModelSource resolves the directory named name under root into its
// backing OpenVINO IR, runtime profile, identity, and adapter set. A directory
// holding an IR entrypoint is a base model (existing behavior, unchanged). A
// directory holding a contenox-variant.json marker (and no IR of its own) is a
// variant: its base_model must be a sibling directory with an IR entrypoint,
// whose profile the variant inherits, and whose IR it reuses in place while
// attaching the variant's own adapters.
func resolveModelSource(root, name string) (modelSource, error) {
	dir := filepath.Join(root, name)
	// Dispatch on the variant marker's PRESENCE, but only when the directory has
	// no IR entrypoint of its own: a base model's IR always wins, so a variant is
	// only ever the explicit marker in a dir that is not itself a base.
	if _, ok := modelEntrypointPath(dir); !ok {
		if v, ok, verr := loadVariantProfile(dir); verr != nil {
			return modelSource{}, verr
		} else if ok {
			return resolveVariantSource(root, dir, name, v)
		}
		// Not a variant: fall through to the base-model path (identity degrades to
		// empty when the IR/config files are absent, exactly as before).
	}
	profile, err := loadModelProfile(dir)
	if err != nil {
		return modelSource{}, err
	}
	adapters, err := resolveProfileAdapters(dir, profile.Adapters)
	if err != nil {
		return modelSource{}, err
	}
	modelDigest, templateDigest := modelIdentity(dir)
	return modelSource{
		name:           name,
		baseName:       name,
		modelDir:       dir,
		profile:        profile,
		adapters:       adapters,
		modelDigest:    modelDigest,
		templateDigest: templateDigest,
	}, nil
}

// resolveVariantSource resolves a validated variant marker against its base
// model under root, reusing the base IR in place and attaching the variant's
// adapters.
func resolveVariantSource(root, dir, name string, v variantProfile) (modelSource, error) {
	if v.Name != "" && v.Name != name {
		return modelSource{}, fmt.Errorf("%w: openvino variant %s: name %q must match its directory %q", ErrUnsupportedFeature, variantProfilePath(dir), v.Name, name)
	}

	baseDir := filepath.Join(root, v.BaseModel)
	if _, ok := modelEntrypointPath(baseDir); !ok {
		return modelSource{}, fmt.Errorf("%w: openvino variant %q base model %q has no IR entrypoint in %s", ErrUnsupportedFeature, name, v.BaseModel, baseDir)
	}
	profile, err := loadModelProfile(baseDir)
	if err != nil {
		return modelSource{}, err
	}
	// Variant adapters are resolved relative to the variant directory (matching
	// the profile-adapter convention) and are authoritative: they, not any base
	// profile adapters, define the variant on top of the base IR.
	adapters, err := resolveProfileAdapters(dir, v.Adapters)
	if err != nil {
		return modelSource{}, err
	}
	modelDigest, templateDigest := modelIdentity(baseDir)
	return modelSource{
		name:           name,
		baseName:       v.BaseModel,
		modelDir:       baseDir,
		profile:        profile,
		adapters:       adapters,
		modelDigest:    modelDigest,
		templateDigest: templateDigest,
		isVariant:      true,
	}, nil
}

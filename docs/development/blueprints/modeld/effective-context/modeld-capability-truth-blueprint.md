# Blueprint: modeld Capability-Truth Boundary

Owner: runtime / modeld

Purpose: make every modeld capability surface report what the runtime can actually
serve, not only what metadata can be parsed from a model repository.

## Core Rule

Capability output must be servability output.

If a model, device, or feature cannot be opened and served by the selected backend,
`Describe`, `ModelInfo`, setup output, catalog capability, and UI state must report
that limitation explicitly.

## Affected Surfaces

- `Describe` / model resolver responses.
- `transport.ModelInfo`.
- `contenox setup`, model catalog, and curated registry output.
- Beam or any UI panel that displays modeld capacity facts.
- Benchmark manifests and certification rows.

## Capability Dimensions

Each surface distinguishes:

| Dimension | Meaning |
|---|---|
| `metadata_detected` | Facts parsed from GGUF, OpenVINO IR, tokenizer, config, or profile files. |
| `loader_supported` | The linked native runtime can load the architecture and format. |
| `pipeline_supported` | The selected backend pipeline can serve the model type, for example text CB versus VLM. |
| `device_supported` | The selected device can compile/run required operators and memory features. |
| `context_fit` | The requested hot/planner context fits the capacity policy. |
| `context_certified` | The context tier passed product-path latency, quality, and stability gates. |

Only the intersection is a supported runtime capability.

## Facts Encoded

### GGUF architecture support

A model repository can declare an architecture (for example
`general.architecture = gemma4`) that the pinned llama.cpp runtime does not
support. Numeric metadata is still parseable, so capacity output can look
valid even though the loader cannot serve the model.

Required behavior:

- Read and report the architecture string.
- Check loader support before advertising a model as servable.
- Return `unsupported_architecture` or equivalent when the linked runtime lacks support.
- Preserve the native loader reason in the modeld error.
- Treat runtime pin bumps as integration changes: smoke-test build, package,
  and model load before certifying against the new pin.

### llama vision (mtmd) support

The llama backend serves the VLM pipeline family natively via llama.cpp's
multimodal stack (libmtmd): a `model.gguf` with an `mmproj.gguf` projector in
the same model directory is a servable vision model.

Required behavior:

- `SupportsVision` is certified from the resolved projector's own metadata
  (image-input capability read from the mmproj GGUF), never inferred from a
  model or repository name.
- A model without a resolvable projector is a text model; image input against
  it fails with `ErrUnsupportedFeature`, not a silent text-only degradation.
- Projector weights and the vision-encoder compute reserve enter the capacity
  budget as fixed device overhead; `Describe` additionally reports a
  conservative per-image sequence-token estimate (`VisionTokensPerImage`)
  derived from projector metadata.
- Image-bearing requests that do not fit the hot context window are refused
  with the typed context-overflow error (refuse-don't-spill).

### OpenVINO text versus VLM

The OpenVINO backend serves exported VLM directories through a dedicated VLM
cell wrapping `ov::genai::VLMPipeline` (pinned to the stateful SDPA
implementation), separate from the text effective-context adapter's
`ContinuousBatchingPipeline`. VLM-ness is certified from the export's own IR
layout — `openvino_language_model.xml` plus
`openvino_vision_embeddings_model.xml` in the model directory — never from a
repository name; session open routes on the same layout rule.

The VLM cell is public-surface only: `VLMPipeline` hides its implementation
behind a private pimpl with no prefix-cache or KV hook, unlike the text
pipeline whose internals the effective-context adapter reaches. Vision
sessions therefore have NO prefix-cache reuse, NO cold-KV/effective-context
offload, and no snapshot/restore in v1 — every decode re-prefills the full
multimodal prompt, and that limitation is the advertised truth, not an
implementation secret.

Required behavior:

- Classify OpenVINO repositories by pipeline type (IR layout) before
  cataloging or describing them; a VLM directory reports `SupportsVision` plus
  a per-image sequence-token estimate (`VisionTokensPerImage`) read from the
  export's own `config.json` (declared per-image count, else patch-grid
  geometry; 0 = unknown).
- Vision weights already enter the capacity budget: the whole IR directory —
  vision and text-embedding IRs included — is summed as model weights.
- Image input against a text-only OpenVINO model fails with
  `ErrUnsupportedFeature`, not a silent text-only degradation; markers and
  images must pair 1:1 or the request is refused.
- VLM sessions must not advertise prefix-cache, cold-KV, or snapshot/restore
  capability; requests for structured output or parser protocols on the VLM
  path fail typed until the cell grows those bridges.

### Device feature support

The Intel NPU can enumerate, but the OpenVINO CB/PagedAttention path is unsupported on
that device. Arc iGPU driver stacks can reject XAttention.

Required behavior:

- Device enumeration is not device support.
- AUTO uses only devices certified for the selected pipeline.
- Explicit unsupported pins fail with `ErrUnsupportedFeature` and an actionable reason.
- Auto sparse/XAttention may retry dense; explicit sparse remains a hard failure.

### Context truth

Raw backend probes can accept prompt sizes that modeld does not certify. TinyLlama is
advertised at its trained ceiling unless a certified long-context model/profile exists.

Required behavior:

- `ModelMaxContext` reports the model/profile ceiling.
- `EffectiveContext` reports the hot served window used for cache identity.
- `PlannerEffectiveContext` reports the logical planner window, not a promise that
  every token remains physically hot.
- `CertifiedContext` or equivalent certification metadata must be added before long
  context is advertised as product-supported.

### Resident-session truth

Capacity questions about the identity currently resident in the slot must be
answered from the resident session's open-time resolved `ModelInfo`, not from
a hypothetical recomputation made under that session's own memory footprint.
Same-identity `Describe`, capacity panels, and `model list` must agree with
what the open session actually serves.

## Implementation Requirements

### Loader probe

For each backend:

- expose the model architecture or pipeline family.
- expose the linked runtime version and commit/digest.
- expose a cheap support check when possible.
- classify loader failure into architecture, format, dependency, memory, and unknown.

### Capability report

Add structured unsupported modes:

```json
{
  "unsupported": [
    {
      "code": "unsupported_architecture",
      "detail": "runtime llama.cpp commit does not support general.architecture=gemma4"
    }
  ]
}
```

Keep human-readable text, but make machine-readable codes the stable interface.

### Catalog gating

Curated entries include:

- backend family.
- model format.
- pipeline family: text, embedding, VLM, reranker, image, or other.
- required runtime feature set.
- certified devices and context tiers.

A catalog entry can exist before certification, but it must be labeled uncertified
for serving until product-path rows exist.

### UI and setup

UI/setup may show parseable metadata for debugging, but it must visually separate:

- detected metadata.
- supported runtime capability.
- certified product context.
- unsupported reason.

## Tests

Required unit or system coverage:

- GGUF with known architecture reports servable capability.
- GGUF with unknown/new architecture reports `unsupported_architecture` and no false
  effective-context claim.
- GGUF model with a resolvable image-capable mmproj reports `SupportsVision`;
  without one, image input fails typed and `SupportsVision` stays false.
- OpenVINO VLM repo routes to the vision session, answers an image question,
  and reports `SupportsVision` with a per-image token estimate; image input to
  a text-only OpenVINO model fails typed.
- Explicit OpenVINO NPU pin returns unsupported-feature with PagedAttention reason;
  the VLM (vision) pipeline also rejects an explicit NPU pin.
- AUTO OpenVINO selection excludes NPU for CB.
- Raw context above model ceiling does not change runtime-advertised context.
- Native loader error text is preserved through transport.

## Acceptance

A model capability row is acceptable only when:

- metadata and runtime support are separate.
- unsupported modes have stable codes.
- context values identify fit, hot served context, planner context, and certified context
  separately.
- catalog/setup/UI cannot imply that an unopenable model is servable.
- the same checks are used by benchmark certification.

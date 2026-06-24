# Blueprint: modeld LoRA adapters and local model variants

> Status: product and architecture blueprint. Scope is local inference through
> `modeld`: how LoRA-style adapters should appear in the product, how they affect
> model identity and cache safety, and what the first backend milestone should be.
> Out of scope: training adapters, hosted fine-tuning, policy enforcement, and
> making every backend support dynamic adapters on day one.

## The Product Bet

`modeld` currently serves base local models. System prompts, context assembly, and
tools can steer those models, but they do not change the model's learned priors.
LoRA adapters give us a third local customization primitive:

```text
base model + small adapter + runtime profile = local model variant
```

That makes `modeld` more than a local model runner. It becomes the local layer that
can serve private, small, per-workspace or per-team model variants without asking
users to download another full set of base weights.

The product should not lead with "LoRA" as the primary user concept. The product
surface should be **model variants** or **local custom models**:

```text
qwen3-coder-8b
qwen3-coder-8b + contenox-coding-style
qwen3-coder-8b + acme-internal-apis
```

Internally, a variant can be implemented as one or more LoRA adapters applied to a
base model at session open.

## Why Prompts Are Not Enough

Prompts are still the right tool for explicit instructions, current facts, policy,
and per-turn behavior. LoRA is useful for stable behavior that should become a
default prior:

- code style, naming conventions, formatting habits, and project idioms;
- domain vocabulary, internal API shapes, DSL patterns, error categories;
- autocomplete behavior where prompt budget is small and latency matters;
- reducing repeated prompt text so the context window is used for live repo state;
- lowering instruction fragility when long context competes with system text.

LoRA should not be used for:

- fresh repo knowledge;
- secrets, permissions, or security policy;
- current data;
- anything that must be easy to inspect, cite, or override.

The working split is:

```text
prompts       = instructions and constraints
context/RAG   = facts and current repo state
LoRA adapter  = stable behavior and domain/style priors
```

## Serving Modes

There are three related but different modes. The first milestone should implement
only the first one.

### 1. Dynamic Adapter at Inference Time

The base model remains unchanged. `modeld` loads a small adapter file and applies it
to the model context when opening a session.

```text
model.gguf + adapter.gguf --served as--> variant
```

Properties:

- no full model duplication;
- fast to distribute;
- enables multiple named variants over the same base;
- adapter identity must be part of the session/cache key;
- switching adapters is a model-variant switch and invalidates warm KV.

This is the right first implementation for llama.cpp.

### 2. Merged or Exported Model

An offline tool applies the adapter to the base model and writes a new full model
artifact.

```text
model.gguf + adapter.gguf --offline export--> merged-model.gguf
```

Properties:

- simplest serving path: `modeld` sees a normal model;
- best runtime performance;
- duplicates a full model artifact;
- export/merge can be heavy on consumer hardware;
- loses easy adapter switching.

We should support importing merged models as normal local models, but `modeld` should
not require users to export/merge as the primary path. Consumer-device export is
likely too stressful to make the default workflow.

### 3. Adapter Training

Training or fine-tuning a LoRA adapter is a separate product. It needs datasets,
evaluation, checkpointing, GPU scheduling, privacy controls, and model-specific
recipes. It should not live in the first `modeld` serving milestone.

## Backend Support Position

### llama.cpp

The pinned llama.cpp reference already exposes adapter APIs in `include/llama.h`:

- load LoRA adapter from file;
- read adapter GGUF metadata;
- free adapter;
- apply adapter to a `llama_context` with a scale;
- remove or clear adapters from a context.

The important property for our product is that applying an adapter to a context does
not modify the base model weights. That maps cleanly onto `modeld` dynamic variants:
load base model, attach adapter(s), then serve the session.

First milestone: **llama.cpp GGUF LoRA only**.

### OpenVINO

OpenVINO GenAI should not block the first milestone. Unless we verify a native,
stable dynamic-adapter API for the GenAI path, OpenVINO should support LoRA only via
merged/exported IR models treated as normal models.

Product behavior should be explicit:

- dynamic adapter requested on an OpenVINO backend returns unsupported;
- merged OpenVINO model directories remain normal OpenVINO models;
- model variants declare which backend modes they support.

### Hosted Providers

Hosted providers are out of scope. Their fine-tuning and adapter systems are
provider-specific and not served by `modeld`.

## Product Surface

The user-facing object should be a named local variant, not a raw adapter file.

Example conceptual state:

```text
Base model:
  name: qwen3-coder-8b
  backend: llama
  artifact: ~/.contenox/models/llama/qwen3-coder-8b/model.gguf

Variant:
  name: qwen3-coder-8b-acme
  base: qwen3-coder-8b
  adapters:
    - acme-coding-style
  profile:
    context, prompt template, reasoning/tool protocols, adapter scales
```

A possible CLI shape:

```bash
contenox model pull qwen3-coder-8b
contenox model adapter add acme-coding-style --base qwen3-coder-8b --file acme.gguf
contenox model variant add qwen3-coder-8b-acme --base qwen3-coder-8b --adapter acme-coding-style
contenox config set default-model qwen3-coder-8b-acme
```

The exact command names can change. The product invariant should not:

- adapters are installed artifacts;
- variants are selectable models;
- base and adapter compatibility is validated before use;
- users can tell which base and adapter a variant uses.

## Artifact Layout

The layout should avoid duplicating base weights while keeping variants portable.

One possible layout:

```text
~/.contenox/models/llama/
  qwen3-coder-8b/
    model.gguf
    contenox-llama.json

~/.contenox/adapters/llama/
  acme-coding-style/
    adapter.gguf
    adapter.json

~/.contenox/variants/llama/
  qwen3-coder-8b-acme.json
```

Variant JSON:

```json
{
  "name": "qwen3-coder-8b-acme",
  "backend": "llama",
  "base_model": "qwen3-coder-8b",
  "adapters": [
    {
      "name": "acme-coding-style",
      "path": "~/.contenox/adapters/llama/acme-coding-style/adapter.gguf",
      "digest": "sha256:...",
      "scale": 1.0
    }
  ]
}
```

Adapter metadata should include, at minimum:

- adapter digest;
- expected base model name and/or digest, when known;
- source/provenance;
- backend type;
- optional rank/format metadata if available from GGUF;
- whether the adapter is curated/certified or user-provided.

## Runtime Identity and Cache Safety

LoRA changes model behavior. Therefore a variant is not the same model as its base.
All cache and residency identity must include adapter state.

Identity must include:

- base model name;
- base model content digest;
- backend type;
- adapter list in deterministic order;
- each adapter digest;
- each adapter scale;
- prompt template digest and BOS policy;
- backend runtime version;
- context/runtime config.

This affects:

- `runtime/transport.OpenSessionRequest`;
- session cache key in `runtime/modelrepo/llama`;
- `transport.ContextManifest` digest or runtime identity fields;
- `modeld/slot` `sameIdentity`;
- model list/catalog entries;
- benchmark/report labels.

Warm KV reuse across different adapters is invalid. If a session has resident KV for
`base+A`, it must not be reused for `base+B`, even when prompt text is identical.

Switching adapter variants should be treated like switching model variants:

```text
base model same, adapter changed -> close old active session, open new variant
```

## Transport Shape

The cleanest durable API is to add adapter specs to the transport model handle or
session config. Conceptually:

```go
type AdapterSpec struct {
    Name   string
    Path   string
    Digest string
    Scale  float32
}
```

There are two placement options:

1. Add adapters to `OpenSessionRequest` and `EmbedRequest`.
2. Add adapters to `transport.Config`.

`OpenSessionRequest` is semantically cleaner: adapters are part of the model handle,
not a hardware/runtime knob. `transport.Config` is mechanically convenient because it
already participates in some identity comparisons. The blueprint preference is:

```text
OpenSessionRequest.Adapters []AdapterSpec
LoadModelRequest.Adapters []AdapterSpec
ActiveModel.Adapters []AdapterSpec
```

Then update identity code explicitly instead of smuggling adapters into runtime
configuration.

Embedding should be conservative. If the first llama dynamic adapter path does not
support embeddings over `modeld`, adapter-backed embeddings should remain unsupported.

## Capacity and Overhead

Dynamic adapters add overhead but should be much cheaper than duplicating base model
weights.

Expected overhead:

- adapter memory: roughly adapter file size plus native runtime bookkeeping;
- compute: extra low-rank math in adapted layers;
- session open: adapter load and validation;
- switching: model-variant switch, no warm KV reuse across adapters;
- context budget: KV bytes do not grow directly, but adapter weights can reduce free
  VRAM/RAM and therefore shrink effective context on tight devices.

For typical low-rank adapters the expected slowdown is usually a few percent to low
double digits. High-rank adapters or multiple adapters can be worse. The product should
not promise a fixed number. `modeld Describe` should eventually report adapter memory
and whether effective context was clamped after adapter load.

Merged models remain the faster serving shape, but export/merge is not the default
because it duplicates full weights and can be too heavy for consumer machines.

## Validation and Compatibility

A dynamic adapter should fail early with a clear error when it cannot be used with the
base model.

Validation inputs:

- backend type is `llama`;
- adapter file exists;
- adapter digest matches recorded digest if provided;
- adapter format is supported by the native runtime;
- adapter GGUF metadata can be read;
- expected base model digest/name matches, when metadata provides it;
- adapter scale is finite and within an accepted range.

When compatibility metadata is missing, the product has two choices:

- strict mode: reject unless a base digest is declared;
- permissive mode: warn and allow user-provided adapters.

For curated registry entries, use strict mode. For local user imports, permissive mode is
acceptable only if the UX clearly labels the adapter as user-provided and unverified.

## Security and Provenance

Adapters are data files, but they still affect code generation and assistant behavior.
They need provenance and trust treatment similar to model weights.

Rules:

- never execute anything from an adapter artifact;
- do not follow links or embedded instructions from adapter metadata;
- record source URL, digest, and install time;
- make user-provided vs curated status visible;
- include adapter name/digest in diagnostics and trace/benchmark reports;
- do not use LoRA as a permission or safety mechanism.

Adapter provenance matters because a malicious adapter can bias generated code in subtle
ways even if it cannot execute directly.

## Model Registry Implications

The registry should distinguish:

- base models;
- adapters;
- variants.

A variant may be curated even if it references a curated base plus a curated adapter.
The resolver should produce a normal provider for the variant, so the rest of the
runtime can use it like a model.

Example conceptual registry entries:

```json
{
  "name": "qwen3-coder-8b",
  "type": "model",
  "backend": "llama"
}
```

```json
{
  "name": "contenox-coding-style",
  "type": "adapter",
  "backend": "llama",
  "base": "qwen3-coder-8b"
}
```

```json
{
  "name": "qwen3-coder-8b-contenox",
  "type": "variant",
  "backend": "llama",
  "base": "qwen3-coder-8b",
  "adapters": ["contenox-coding-style"]
}
```

Catalog listing should show variants as selectable models while still exposing the
base/adapter relationship in detail views and diagnostics.

## Observability

Diagnostics should answer:

- which base model is active;
- which adapters are attached;
- adapter digests and scales;
- whether the adapter is curated or user-provided;
- runtime backend and version;
- effective context before/after adapter-aware capacity planning, when available;
- why an adapter failed to load.

`modeld status --json` should include active variant details once adapter support exists.
`contenox doctor` should report adapter compatibility failures in the backend row.

## Non-Goals

The first serving feature should not include:

- training LoRA adapters;
- exporting merged models on consumer hardware;
- dynamic OpenVINO adapters;
- hosted-provider fine-tuning;
- adapter hot-swapping inside an active decode;
- using adapters as safety controls;
- guessing adapter compatibility from names alone for curated entries.

## Phased Plan

### Phase 0: Product Model and Files

- Add registry concepts for adapter and variant, or add a minimal local variant layer
  without changing the curated registry yet.
- Define local artifact layout.
- Define JSON shape for variant and adapter metadata.
- Decide curated vs user-provided validation rules.

Proof point: `contenox model local` can display a base model and a variant that points
to an adapter, without serving it yet.

### Phase 1: Transport Identity

- Add adapter specs to the modeld transport requests and active model status.
- Include adapter identity in llama provider session cache keys.
- Include adapter identity in context manifest runtime identity.
- Include adapter identity in slot identity.
- Add unit tests proving `base+A` and `base+B` cannot share warm sessions or manifests.

Proof point: a fake transport session treats adapter changes as model switches.

### Phase 2: llama.cpp Dynamic LoRA

- Extend the direct llama shim with LoRA load/apply/clear/free bindings.
- Load adapter(s) during session creation after base model/context initialization.
- Apply each adapter with scale.
- Free/clear adapters on session close.
- Surface adapter load errors as typed unsupported/invalid model errors.

Proof point: a real llama.cpp build loads a tiny LoRA GGUF and produces a different
continuation than the base model under the same prompt, while cache identity remains
separate.

### Phase 3: UX and Registry

- Add CLI support for installing adapters and creating variants.
- Add setup/doctor messages for unsupported backend modes.
- Show variant details in `model list`, `model local`, and status output.
- Let defaults point to a variant name.

Proof point: user can select a named local variant as `default-model` and chat through
`modeld`.

### Phase 4: Evaluation and Quality Gates

- Add simple benchmark labels for base vs variant.
- Add smoke prompts for curated adapters.
- Add regression checks for tool-call/reasoning formats when a variant claims those
  capabilities.

Proof point: curated adapters have a documented compatibility and quality check before
they appear in the registry.

### Phase 5: Optional Export/Merge Workflow

- Support importing a merged model as a normal model.
- Document external/offline export path.
- Do not make `modeld` perform the merge on consumer hardware in the first product.

Proof point: a merged model and a dynamic variant can coexist, and diagnostics make the
difference clear.

## Open Questions

- Should variant files live under the base model directory or in a separate variants
  namespace?
- Should user-provided adapters be allowed without base digest metadata?
- Should multiple adapters be supported in the first llama milestone, or should we start
  with exactly one adapter?
- Should adapter scale be user-tunable in normal UX, or only in advanced profile JSON?
- Should adapter-backed variants inherit base model tool/reasoning certifications, or
  must they re-certify?
- How should adapter provenance appear in ACP/VS Code surfaces?

## Recommendation

Implement LoRA as **dynamic llama.cpp model variants** first.

Keep the product surface high-level:

```text
variant = base model + adapter(s) + profile
```

Do not make export/merge a required workflow. Treat export as an optional future path
for users who want maximum runtime performance and accept full model duplication.

Do not attempt OpenVINO dynamic adapter parity in the first pass. OpenVINO can support
adapter-tuned behavior through merged IR models until a native GenAI dynamic-adapter
path is proven.

The load-bearing engineering rule is cache identity: adapter digest, order, and scale
must be part of every session and manifest identity before native adapter calls ship.

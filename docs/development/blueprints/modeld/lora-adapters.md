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

The pinned llama.cpp reference already exposes the full adapter C API. Verified
present in `.llamacpp-runtime/cuda/include/llama.h` (the header the CGo shim builds
against, `modeld/llama/llamacppshim/direct.go` line 11 `#include "llama.h"`):

- `llama_adapter_lora_init(model, path_lora)` — load a GGUF adapter, bound to the
  base `llama_model` (header line 568);
- `llama_adapter_meta_val_str` / `llama_adapter_meta_count` /
  `llama_adapter_meta_key_by_index` — read adapter GGUF metadata for validation and
  provenance (lines 579–588);
- `llama_adapter_lora_free(adapter)` (line 592);
- `llama_set_adapter_lora(ctx, adapter, scale)` — apply an adapter to a
  `llama_context` with a float scale (line 602);
- `llama_rm_adapter_lora(ctx, adapter)` / `llama_clear_adapter_lora(ctx)` — remove
  one or all adapters from a context (lines 609–614);
- `llama_adapter_get_alora_*` — invocation-token API for activated LoRA (aLoRA); out
  of scope for the first milestone but available.

The load-bearing property: `llama_set_adapter_lora` mutates the **context**, not the
base `llama_model` weights. The adapter is owned by the model and freed with it. That
maps cleanly onto `modeld`'s single resident model + per-session context: load base
model once, attach adapter(s) to the session context at open, serve.

**Implemented and smoke-tested.** `direct.go` now wraps the adapter API as
`Model.LoadAdapter` (`llama_adapter_lora_init`), `Adapter.MetaValue`
(`llama_adapter_meta_val_str`), `Adapter.Free` (`llama_adapter_lora_free`),
`Context.SetAdapter` (`llama_set_adapter_lora`), and `Context.ClearAdapters`
(`llama_clear_adapter_lora`). `llamasession.NewWithAdapters` loads each
`llama.AdapterSpec` against the base model and attaches it to the session context
after `NewContext`, frees adapters before the model on close (the model frees still-
attached adapters, so freeing after `model.Close` would double-free), and `New`
delegates to it with no adapters. Unlike OpenVINO, GGUF LoRA applies the adapter to a
live `llama_context` post-load, so there is no construction-time property to thread —
and it works on quantized base models (Q8_0 verified) since the adapter math is
applied alongside the dequantized weights.

Proven by `TestSystem_LlamaSessionLoRA_AdapterChangesContinuation` (driving the shim →
`NewWithAdapters` → EnsurePrefix/PrefillSuffix/Decode path against Qwen3-0.6B-Q8_0 with
a real GGUF adapter): the log shows `set_adapter_lora: ... scale = 8.0` and the greedy
continuation changes versus base. The GGUF adapter fixture
(`testdata/make_lora_gguf.py`) reads the base model's real tensor dims so adapter
shapes satisfy llama.cpp's loader (`base.ne[0]==lora_a.ne[0]`,
`base.ne[1]==lora_b.ne[1]`, `lora_a.ne[1]==lora_b.ne[0]`), and the adapter is named
with llama.cpp GGML tensor names (`blk.N.attn_q.weight.lora_a`), not PEFT names — the
mirror-image gotcha of the OpenVINO path.

### OpenVINO

OpenVINO GenAI **is part of the first milestone**. The GenAI path supports LoRA
natively, but on a different mechanism and file format than llama, and modeld must
expose it as a stable dynamic-adapter API. The divergences that drive the design:

- **Format**: OpenVINO GenAI consumes adapters in **safetensors**, not GGUF
  (`ov::genai::Adapter(path)` / `ov::genai::AdapterConfig`). A GGUF adapter cannot
  serve the OpenVINO backend and vice-versa, so adapter artifacts and registry
  entries are backend-typed exactly like base models already are.
- **Application point**: llama attaches an adapter to a live context post-load;
  OpenVINO registers the `AdapterConfig` on the pipeline via the `ov::genai::adapters`
  property at **construction** time. modeld already passes an `ov::AnyMap properties`
  into the `ContinuousBatchingPipeline` constructor
  (`modeld/openvino/ovsession/genai.cpp` line 1436–1441) — that map is the injection
  point. The full adapter set must therefore be known at `cx_genai_session_new`
  (session open), which fits modeld's single-slot / session-open lifecycle.
- **Static vs dynamic**: `AdapterConfig` has modes `MODE_AUTO`, `MODE_DYNAMIC`,
  `MODE_STATIC_RANK`, `MODE_STATIC`, `MODE_FUSE`. Only `MODE_DYNAMIC` keeps A/B/alpha
  variable so adapters can be selected per `generate()` from the registered set
  without recompiling. modeld should register with `MODE_DYNAMIC` and select the
  active adapter(s) per request via `GenerationConfig.adapters`.
- **Scale semantics**: OpenVINO folds LoRA `alpha/rank` and any user weight into one
  effective alpha. The transport `Scale` maps to that alpha; the backend, not the
  runtime, decides rank normalization.

**Parity gate — VERIFIED (OpenVINO GenAI 2026.2, smoke-tested end to end):** the raw
`ContinuousBatchingPipeline` that modeld drives directly (not the `LLMPipeline` wrapper
the docs use) **does** honor the `ov::genai::adapters` property injected into its
construction `AnyMap`, and a registered `MODE_DYNAMIC` adapter measurably changes
generation. Proven by `TestSystem_OpenVINOGenAI_LoRAAdapterGenerates` driving the Go →
CGo → CB-pipeline path with a real safetensors adapter. Two load-bearing gotchas found
while proving it:

- **int4 is fine for dynamic LoRA.** `MODE_DYNAMIC` inserts on activations, so it
  applies to u4/u8 weight-compressed models — the same int4 exports modeld serves on
  consumer hardware. (`MODE_FUSE` is the mode that rejects low-bit weights with "Use
  f32/f16/bf16 weights only"; modeld must not use FUSE on compressed models.)
- **Adapter tensor names must be canonical PEFT**
  (`base_model.model.model.layers.N.self_attn.q_proj.lora_A.weight`). OpenVINO's prefix
  detection maps these onto model MatMul nodes; shorter/abbreviated names load but match
  **zero** nodes and silently no-op (only a "unused LoRA tensors" log). Validation must
  reject or warn on adapters that match zero layers, or a variant will silently behave
  like its base.

The low-level plumbing (C ABI `cx_genai_lora_adapter` + `cx_genai_session_config`
fields, `AdapterConfig`/`adapters` injection in `genai.cpp`, `GenAIConfig.LoRAAdapters`
in `genai.go`) is implemented and smoke-tested. What remains for OpenVINO is the layer
**above** ovsession: threading `AdapterSpec` from the transport request through the
`openvino` service into `GenAIConfig.LoRAAdapters` (Phase 1 identity work).

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

This affects (real symbols — see the Code Map for the full chain):

- `runtime/transport/session.go` — `OpenSessionRequest` / `EmbedRequest` /
  `LoadModelRequest` / `ActiveModel` (add `Adapters []AdapterSpec`);
- `runtime/transport/grpc/wire.go` + `client.go`/`server.go` — the JSON wire structs
  (`openSessionReq`, `loadModelReq`, `describeReq`) mirror those fields and must carry
  adapters too, or they are dropped on the wire;
- `runtime/modelrepo/llama/client.go` — `sessionCacheKey` (warm-reuse key) and
  `client.ref()` (the `ModelRef` it opens with);
- `runtime/modelrepo/modeldconn/modeldconn.go` — `ModelRef` and the
  `openRequest`/`loadRequest` mappers;
- `runtime/modelrepo/llama/manifest.go` — `runtimeDigest` (the manifest runtime
  identity hash);
- `modeld/slot/service.go` — `sameIdentity` (the same-model gate) and `activeModel`;
- model list/catalog entries (`runtime/modelrepo/llama/catalog.go`);
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
- multi-adapter-per-batch switching in OpenVINO continuous batching (single resident
  variant per slot is enough; revisit only if the CB spike proves it cheap);
- hosted-provider fine-tuning;
- adapter hot-swapping inside an active decode;
- using adapters as safety controls;
- guessing adapter compatibility from names alone for curated entries.

Note: dynamic OpenVINO adapters were previously a non-goal; they are now in the first
milestone (see Backend Support Position). What remains out of scope is the per-batch
multi-adapter case above, not single-variant OpenVINO LoRA.

## Code Map

This maps every abstract reference in the blueprint to the symbol that exists in the
tree today, so the phased plan is concrete. Paths are relative to repo root. Nothing
below has adapter support yet — these are the exact sites that change.

### The identity seam (transport)

| Concept | Symbol | Today | Change |
| --- | --- | --- | --- |
| Session open handle | `runtime/transport/session.go` → `OpenSessionRequest` (line ~145) | `Fence, ModelName, Type, Digest, Path, Config` | add `Adapters []AdapterSpec` |
| Embedding handle | same file → `EmbedRequest` | mirrors open req | add `Adapters` (or leave embeddings adapter-free per blueprint) |
| Explicit slot load | same file → `LoadModelRequest` | mirrors open req | add `Adapters` |
| Active slot report | same file → `ActiveModel` (line ~192) | `ModelName, Type, Digest, Path, Config, Generation` | add `Adapters` for diagnostics |
| New type | — | — | define `AdapterSpec{ Name, Path, Digest string; Scale float32 }` |
| Manifest cache key | `runtime/contextasm/manifest.go` → `ContextManifest` (aliased in `transport`) | profile/model/template/BOS hashes | adapter digests fold into the runtime-identity hash (see llama `runtimeDigest`) |

`Config` (same file, line ~38) is the alternative placement the blueprint rejects:
adapters belong on the model handle, not the hardware/runtime knob struct. Keep them on
the request types.

### Runtime → modeld wire

| Concept | Symbol | Note |
| --- | --- | --- |
| Typed handle | `runtime/modelrepo/modeldconn/modeldconn.go` → `ModelRef` (line ~124) | `Name, Type, Digest, Path`; add `Adapters` |
| Request mappers | same file → `openRequest` / `loadRequest` (line ~200/211) | copy `ModelRef.Adapters` into the transport requests |
| gRPC wire structs | `runtime/transport/grpc/wire.go` → `openSessionReq`, `loadModelReq`, `describeReq` | JSON codec (`codec.go`), so a field not added here is silently dropped over the wire |
| gRPC client/server copy | `runtime/transport/grpc/client.go` (line ~77/109/133/186), `server.go` | each field is copied by hand; add the adapter field at every copy site |

### llama provider identity (warm reuse)

| Concept | Symbol | Note |
| --- | --- | --- |
| Warm-cache key | `runtime/modelrepo/llama/client.go` → `sessionCacheKey` (line ~34) | append deterministic `adapter=<digest>@<scale>` segments; this is what stops `base+A` reusing `base+B`'s KV |
| Client handle | same file → `client.ref()` (line ~69) and `client` struct (line ~55) | carry the resolved adapter list |
| Runtime-identity hash | `runtime/modelrepo/llama/manifest.go` → `runtimeDigest` (line ~22) | add adapters to the `runtimeIdentity` struct so the `ContextManifest` differs across variants |
| Adapter file digest | `runtime/modelrepo/llama/model_identity.go` → `modelFileDigest` (line ~24) | reuse the same stat-cached sha256 helper for adapter files |
| Warm cache | same file → `warm = modelrepo.NewWarmCache` (line ~18) | unchanged; evict-before-open already handles single-slot switching (`[[warmcache-evict-before-open]]`) |

### slot identity (daemon)

| Concept | Symbol | Note |
| --- | --- | --- |
| Same-model gate | `modeld/slot/service.go` → `sameIdentity` (line ~601) | compares `ModelName/Type/Digest/Path/Config`; must also compare `Adapters` so an adapter change forces a switch |
| Active descriptor | same file → `activeModel` (line ~609) | copy adapters into `ActiveModel` |

### llama session creation (where adapters attach) — Phase 2

| Concept | Symbol | Note |
| --- | --- | --- |
| Backend-neutral contract | `modeld/llama/session.go` | `llama.Config` / `llama.Session`; `Config` gains adapter fields if adapters flow through here |
| Session open | `modeld/llama/llamasession/llama.go` → `New` (line ~163) | after `NewContext` (line ~202): `llama_adapter_lora_init` per adapter, then `llama_set_adapter_lora(ctx, adapter, scale)`; store handles on the `session` struct; free in `Close` |
| Model config | same file → `modelConfig` (line ~244) | adapters are context-level, not model-load-level; no change to `ModelConfig` |
| CGo shim | `modeld/llama/llamacppshim/direct.go` | add `AdapterLoad`/`AdapterMeta`/`AdapterFree`/`SetAdapter`/`ClearAdapters` wrapping the `llama_adapter_*` calls; mirror the `Model`/`Context` ownership pattern (line ~157/493) |

### OpenVINO session creation (where adapters attach) — Phase 2 parity

| Concept | Symbol | Note |
| --- | --- | --- |
| Session open (Go) | `modeld/openvino/ovsession/genai.go` → `NewGenAI` (line ~192) → `cx_genai_session_new` (line ~232) | thread adapter paths+scales into `GenAIConfig` and the `cx_genai_session_config` struct |
| Config struct | `modeld/openvino/ovsession/genai.h` → `cx_genai_session_config` | add adapter path/scale array fields |
| Pipeline construction (C++) | `modeld/openvino/ovsession/genai.cpp` → `properties` AnyMap (line ~1436) feeding `ContinuousBatchingPipeline(...)` (line ~1437) | inject `ov::genai::adapters(adapter_config)` (built from `ov::genai::Adapter` + `AdapterConfig::add(adapter, alpha)`, `MODE_DYNAMIC`) into that map |
| Per-request selection | `genai.go` → `Generate` (line ~243) / `genai.cpp` generate path | optionally pass `GenerationConfig.adapters` to select the active subset; verify CB support first (see Backend Support Position parity risk) |

### Identity-isolation tests to add

- `runtime/modelrepo/llama/primitives_test.go` already proves `sessionCacheKey`
  isolates config changes (`TestUnit_LocalNodeSessionCacheKey_IncludesRuntimeIdentity`,
  line ~59) — extend it so `base+A` and `base+B` produce different keys, and `base` vs
  `base+A` differ.
- `modeld/slot/service_test.go` — assert `sameIdentity` is false across an adapter
  change (the "fake transport session treats adapter changes as model switches" proof
  point).

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

- Define `transport.AdapterSpec` and add `Adapters []AdapterSpec` to
  `OpenSessionRequest`, `LoadModelRequest`, and `ActiveModel` (`session.go`).
- Thread the field through the gRPC wire layer: `wire.go` structs + every hand-copy
  site in `client.go`/`server.go` (a dropped wire field is an invisible cache-safety
  bug).
- Add `Adapters` to `modeldconn.ModelRef` and copy it in `openRequest`/`loadRequest`.
- Include adapter digests+scales in `llama` `sessionCacheKey` and `runtimeDigest`.
- Include adapters in `slot.sameIdentity` and `slot.activeModel`.
- Add unit tests proving `base+A` and `base+B` cannot share warm sessions or manifests
  (extend `primitives_test.go`; add a `slot.sameIdentity` case).

Proof point: a fake transport session treats adapter changes as model switches.

### Phase 2: llama.cpp Dynamic LoRA + OpenVINO Parity

Two backend tracks; both gated on the same Phase 1 identity work.

**llama.cpp (GGUF):**

- Extend `llamacppshim/direct.go` with `AdapterLoad`/`AdapterMeta`/`SetAdapter`/
  `ClearAdapters`/`AdapterFree` wrapping `llama_adapter_lora_init`,
  `llama_adapter_meta_val_str`, `llama_set_adapter_lora`, `llama_clear_adapter_lora`,
  `llama_adapter_lora_free`.
- In `llamasession.New`, after `NewContext`, init each adapter and
  `llama_set_adapter_lora(ctx, adapter, scale)`; store handles on `session`; free on
  `Close`.
- Validate via `llama_adapter_meta_*` (format/base metadata) and surface failures as
  typed unsupported/invalid model errors.

**OpenVINO (safetensors) — parity, runs in the same phase:**

- First, the CB-pipeline spike from Backend Support Position: confirm
  `ContinuousBatchingPipeline` honors the `ov::genai::adapters` property and
  (optionally) per-request `GenerationConfig.adapters` on the pinned OpenVINO version.
- Add adapter path/scale fields to `cx_genai_session_config` (`genai.h`) and thread
  them from `GenAIConfig`/`NewGenAI`.
- In `genai.cpp`, build `ov::genai::AdapterConfig` (`MODE_DYNAMIC`) from
  `ov::genai::Adapter(safetensors)` + `add(adapter, alpha)` and inject it into the
  existing `properties` AnyMap at pipeline construction.
- Surface load/format failures as typed errors, same as llama.

Proof point: a real llama.cpp build loads a tiny GGUF adapter and a real OpenVINO build
loads a safetensors adapter; each produces a different continuation than its base model
under the same prompt, while cache identity stays separate per variant.

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
- Does the pinned `ContinuousBatchingPipeline` honor the `ov::genai::adapters` property
  and per-request `GenerationConfig.adapters`, or only a single construction-time set?
  (Phase 2 spike; gates how much OpenVINO can match llama's per-session flexibility.)
- A variant references a backend-typed adapter file (GGUF for llama, safetensors for
  OpenVINO). Should a logical variant name be allowed to resolve to different adapter
  files per backend, or is a variant always pinned to one backend+format?
- Should adapter file digests reuse `model_identity.go`'s stat-cached sha256, or get a
  separate adapter-digest cache keyed by content + format?

## Recommendation

Implement LoRA as **dynamic local model variants** on both first-class backends:
llama.cpp (GGUF, context-level `llama_set_adapter_lora`) and OpenVINO GenAI
(safetensors, `MODE_DYNAMIC` `AdapterConfig` on the CB pipeline). Land the shared
transport identity (Phase 1) once; both backend tracks depend on it.

Keep the product surface high-level:

```text
variant = base model + adapter(s) + profile
```

Do not make export/merge a required workflow. Treat export as an optional future path
for users who want maximum runtime performance and accept full model duplication.

Reach llama/OpenVINO parity at the product surface (a variant is a variant regardless
of backend), but respect the two mechanical divergences: GGUF vs safetensors adapter
files, and context-level apply (llama) vs construction-time `AdapterConfig` property
(OpenVINO). De-risk the OpenVINO path with the `ContinuousBatchingPipeline` adapter
spike before wiring it end-to-end.

The load-bearing engineering rule is cache identity: adapter digest, order, and scale
must be part of every session and manifest identity before native adapter calls ship —
and that rule is backend-agnostic, so Phase 1 lands once and protects both backends.

# Blueprint: modeld LoRA adapters and local model variants

> Status: product and architecture blueprint; the serving plumbing is LANDED
> (2026-07-22). Scope is local inference through `modeld`: how LoRA-style adapters
> should appear in the product, how they affect model identity and cache safety,
> and what the first backend milestone should be. Out of scope: training adapters,
> hosted fine-tuning, policy enforcement, and making every backend support dynamic
> adapters on day one.
>
> Implementation state: Phases 1–2 are implemented and smoke-tested on both
> backends — transport identity, gRPC wire, provider cache keys, slot identity,
> and native attach (see Code Map). Adapters currently enter the system only via
> model-profile JSON (`adapters[]` in `contenox-llama.json` /
> `contenox-openvino.json`, or snapshot profiles); there is no CLI, API, or
> registry surface yet (Phases 0/3–5 open), no real adapter artifacts ship (test
> fixtures are synthetic), and the LoRA system tests are env-gated. Vision (VLM)
> sessions refuse adapters in v1 — see Backend Support Position.

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

The pinned llama.cpp reference exposes the full adapter C API. Verified present in
`.llamacpp-runtime/local/include/llama.h` (the header the CGo shim builds against;
line numbers drift with the pin):

- `llama_adapter_lora_init(model, path_lora)` — load a GGUF adapter, bound to the
  base `llama_model` (~line 650);
- `llama_adapter_meta_val_str` / `llama_adapter_meta_count` /
  `llama_adapter_meta_key_by_index` — read adapter GGUF metadata for validation and
  provenance (~lines 661–667);
- `llama_adapter_lora_free(adapter)` (~line 674);
- `llama_set_adapters_lora(ctx, adapters, n, scales)` — the pin's **plural**
  set-all-at-once form (~line 683); the older singular
  `llama_set_adapter_lora`/`llama_rm_adapter_lora` calls no longer exist upstream;
- `llama_adapter_get_alora_*` — invocation-token API for activated LoRA (aLoRA); out
  of scope for the first milestone but available (~lines 677–678).

The load-bearing property: `llama_set_adapters_lora` mutates the **context**, not the
base `llama_model` weights. The adapter is owned by the model and freed with it. That
maps cleanly onto `modeld`'s single resident model + per-session context: load base
model once, attach adapter(s) to the session context at open, serve.

**Implemented and smoke-tested.** `direct.go` now wraps the adapter API as
`Model.LoadAdapter` (`llama_adapter_lora_init`), `Adapter.MetaValue`
(`llama_adapter_meta_val_str`), `Adapter.Free` (`llama_adapter_lora_free`), and
`Context.SetAdapter` / `Context.ClearAdapters` (both via the pin's plural
`llama_set_adapters_lora`). `llamasession.NewWithAdapters` loads each
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
in `genai.go`) is implemented and smoke-tested. The layer **above** ovsession is also
done: `modeld/openvino/service.go` maps `req.Adapters` through `toGenAILoRA` (transport
`Scale` → OpenVINO's folded alpha, unit-proven in `lora_test.go`) into
`GenAIConfig.LoRAAdapters`, and `service_lora_system_test.go` smoke-tests the full
Service path via `req.Adapters`.

### Vision (VLM) Sessions

Vision sessions refuse adapters today, and the refusal is deliberate v1 scoping, not
an upstream limit:

- `modeld/openvino/service.go` returns a typed `ErrUnsupportedFeature` ("openvino VLM
  session does not support LoRA adapters in v1") when `req.Adapters` is non-empty on a
  VLM open; `visionsession.go` and `ovsession/vlm.cpp` carry no adapter plumbing.
- Upstream is ready at the pin: `ov::genai::adapters(...)` is a generic
  `ov::Property<AdapterConfig>` accepted through any construction `AnyMap`, and the
  `VLMPipeline` ctors take one. `vlm.cpp` already passes a construction `AnyMap` (the
  SDPA attention-backend setting) — that map is the ready-made injection point.
- Enabling it is contained: add adapter fields to the VLM C-ABI session config, build
  the `MODE_DYNAMIC` `AdapterConfig` into that map (mirroring `genai.cpp`), thread
  `req.Adapters` through the vision session, and fold adapters into VLM slot identity.
  This is lighter than the text path was: the VLM session has no
  prefix-cache/cold-KV/snapshot surface to keep identity-safe.
- llama vision (mtmd) + adapters is untested as a combination. Adapters attach to the
  context and the projector is a separate artifact, so no conflict is known — but the
  combo must not be certified until a system test proves it.

Enable VLM adapters only with the same proof standard as text: a real adapter
measurably changes generation on a VLM base under a vision prompt, with identity
isolation asserted.

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

- `runtime/transport/session.go` — `OpenSessionRequest` / `LoadModelRequest` /
  `ActiveModel` (`Adapters []AdapterSpec` — landed; embeddings stay adapter-free);
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
already participates in some identity comparisons. The blueprint preference (landed
exactly as specified) is:

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

## Code Map (implemented)

Everything below is landed and wired. Line numbers are approximate and drift with the
tree; the map documents where each piece lives and what proves it.

### Origin — where adapters enter the system today

| Piece | Symbol |
| --- | --- |
| llama profile parse | `runtime/modelrepo/llama/adapters.go` → `resolveProfileAdapters` (reads `contenox-llama.json` `adapters[]`) |
| OpenVINO profile parse | `runtime/modelrepo/openvino/adapters.go` → `resolveProfileAdapters` (`contenox-openvino.json`; profile struct in `profile.go`) |
| modeld HTTP API resolver | `runtime/internal/modeldapi/resolver.go` → `resolveAdapterProfiles` (used by both the llama and OpenVINO model resolvers) |
| Snapshot profiles | `runtime/contenoxcli/model_snapshot_cmd.go` → `resolveSnapshotAdapters` |

Shape everywhere: `{name, path, digest?, scale?}` — digest auto-computed (sha256),
scale defaults to 1.0. This profile JSON is the **only** origin: no CLI flag, no API
request field, and no registry entry produces adapters yet (that is Phases 0/3).

### Identity seam (transport) — landed

`transport.AdapterSpec{Name, Path, Digest, Scale}` (`session.go` ~:194), carried on
`OpenSessionRequest` (~:215), `LoadModelRequest` (~:298), and `ActiveModel` (~:271).
Embeddings stay adapter-free, per this blueprint. The gRPC wire carries `adapters` in
`wire.go` (~:54, :72) with hand-copies at every site in `client.go`/`server.go`;
round-trip proven by `grpc_test.go` (~:689–703).

### Provider identity / warm-KV safety — landed (both backends)

llama: `runtime/modelrepo/llama/client.go` folds deterministic
`adapters=<digest>@<scale>` segments into `sessionCacheKey` (what stops `base+A`
reusing `base+B`'s KV) and `manifest.go` hashes adapters in `runtimeDigest`.
OpenVINO: `runtime/modelrepo/openvino/client.go` + `prompt.go`
(`appendAdapterIdentity`). Daemon slot: `modeld/slot/service.go` → `sameIdentity`
compares `Adapters` (~:889–895), so an adapter change forces a model switch;
`activeModel` copies them for diagnostics, surfaced via `modeldapi`
(`sanitizeAdapters`) and the OpenAPI schema.

### Native attach — landed

llama: `llamacppshim/direct.go` → `Model.LoadAdapter` / `Adapter.MetaValue` /
`Adapter.Free` / `Context.SetAdapter` / `Context.ClearAdapters` (~:863–:925, via the
pin's plural `llama_set_adapters_lora`); `llamasession.NewWithAdapters` /
`applyAdapters` attach at session open and free adapters before the model on close.
OpenVINO: `ovsession/genai.go` → `GenAIConfig.LoRAAdapters` → C ABI
`cx_genai_lora_adapter` + `cx_genai_session_config.lora_adapters` (`genai.h`) →
`genai.cpp` builds `AdapterConfig(MODE_DYNAMIC)`, injects `ov::genai::adapters(...)`
into the CB pipeline construction `AnyMap`, and activates it in the default
`GenerationConfig` (~:1839–1879).

### Proof tests (all env-gated; they skip without real model+adapter paths)

- `modeld/llama/llamasession/lora_test.go` + `service_lora_system_test.go` — need
  `CONTENOX_LLAMA_LORA_GGUF` + `CONTENOX_LLAMA_LORA_ADAPTER`; fixture generator
  `testdata/make_lora_gguf.py` (GGML tensor names, e.g. `blk.N.attn_q.weight.lora_a`).
- `modeld/openvino/ovsession/genai_lora_test.go` +
  `modeld/openvino/service_lora_system_test.go` — need
  `CONTENOX_OPENVINO_TEST_MODEL` + `CONTENOX_OPENVINO_TEST_LORA`
  (plus `_EXPECT_DIFF=1` to hard-assert a behavior change); fixture generator
  `testdata/make_lora.py` (canonical PEFT tensor names).
- `runtime/transport/grpc/grpc_test.go` — adapters survive the wire.
- Still worth adding if not already present: a `sessionCacheKey` unit case asserting
  `base`, `base+A`, and `base+B` all produce distinct keys, and a
  `slot.sameIdentity` adapter-change case.

## Phased Plan

### Phase 0: Product Model and Files — OPEN (partially pre-empted)

The minimal non-registry origin exists: profile-JSON `adapters[]` is parsed by both
providers, the modeld API resolver, and snapshot profiles (see Code Map → Origin).
Registry types, the artifact layout, and validation rules remain undone.

- Add registry concepts for adapter and variant, or add a minimal local variant layer
  without changing the curated registry yet.
- Define local artifact layout.
- Define JSON shape for variant and adapter metadata.
- Decide curated vs user-provided validation rules.

Proof point: `contenox model local` can display a base model and a variant that points
to an adapter, without serving it yet.

### Phase 1: Transport Identity — LANDED

Everything below is implemented; see Code Map for symbols and the wire round-trip
proof test.

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

### Phase 2: llama.cpp Dynamic LoRA + OpenVINO Parity — LANDED (both backends)

Both tracks are implemented and smoke-tested; the CB-pipeline spike is done and
verified (`MODE_DYNAMIC` works on int4/u4 weight-compressed models; `MODE_FUSE`
rejects low-bit weights). See Code Map → Native attach and Proof tests.

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

### Phase 3: UX and Registry — OPEN (the current front door work)

This is now the gap that keeps adapters unusable in practice: nothing upstream of the
profile JSON produces an `adapters[]`, and no real adapter artifacts ship.

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
- ANSWERED: the pinned `ContinuousBatchingPipeline` honors the `ov::genai::adapters`
  property at construction and activation via the default `GenerationConfig`
  (verified at the 2026.2 pin; `MODE_DYNAMIC`, int4-safe). Per-request adapter-subset
  selection remains unexercised.
- Should VLM (vision) sessions gain adapter support next, given upstream readiness and
  the contained change (see Vision (VLM) Sessions)?
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
(OpenVINO). The serving path is landed and proven on both backends; the remaining work
is the product surface — registry adapter/variant types, install/create CLI, and
visibility in `model list`/status — plus a decision on enabling VLM-session adapters.

The load-bearing engineering rule is cache identity: adapter digest, order, and scale
must be part of every session and manifest identity before native adapter calls ship —
and that rule is backend-agnostic, so Phase 1 lands once and protects both backends.

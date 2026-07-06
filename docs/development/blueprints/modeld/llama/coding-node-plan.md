# Plan: Graduate Contenox Local Coding Node on llama.cpp

> **Status:** graduation blueprint. Sibling to `../openvino/coding-node-plan.md`. The current
> llama.cpp provider must stop being a toy fallback: this track turns it into the
> serious local runtime path with session state, stable-prefix reuse, explicit
> hardware/config controls, and the same context assembler used by OpenVINO. The
> OpenVINO track proved the workspace-context design (S0/S1/S2); llama.cpp
> should now implement that design quickly on accessible hardware, then the
> validated runtime shape ports back to OpenVINO. Owning both backends is
> deliberate — see "Why both".
>
> **Binary boundary:** the CGO llama.cpp work lives in the `modeld` daemon
> (`modeld/llama`). `runtime` stays pure Go and reaches it as a client over the
> `modeld` transport (`../interface-boundary.md`). The pure-Go context
> assembler (`AssembleContext`), model catalog, and backend management stay in
> `runtime`; `modeld` owns device memory, KV cache, and sessions.
>
> **Runtime path:** the permanent `llama` path is a pinned, Contenox-built
> direct llama.cpp adapter.

---

## Goals

The product goals this track serves live in `local-coding-node-goals.md` — the
north star (useful local coding on a ~1.5k EUR node), the workspace-context design goal
(reusable workspace execution state), and the T1/T2/T3 latency targets. This
blueprint is the llama.cpp plan to *reach* them; the section "Reaching the goals
on the 6 GB test bench" below maps each goal to a spike and is explicit about
what the dev bench can prove versus what the budget node must.

---

## Context

The workspace-context design is already proven on OpenVINO:

- **S0** — KV snapshot/restore round-trips (only at `KV_CACHE_PRECISION=f16`;
  default 8-bit KV is lossy). See `openvino-s2-prefix-reuse.md` / memory.
- **S2** — `ContinuousBatchingPipeline` prefix caching collapses a **2m14s cold
  prefill to 664ms warm (99.5%)** for a repeated stable prefix; cold CPU prefill
  (~350 tok/s) is the real bottleneck.
- **S2.5** — the deterministic `AssembleContext` assembler drives the cache: a
  stable-prefix **hash predicts** the warm hit (104× on a 9k-token prefix); an
  edited stable segment correctly re-prefills.

The OpenVINO track is **paused mid-S5** (constrained tool calls). We pivot to
llama.cpp to prove the **design goals** faster, because GGUF models are
ubiquitous and GPU offload (CUDA/Metal/Vulkan) is practical on developer
machines. The production path is the CGO `modeld/llama` backend, owned by the
`modeld` daemon: persistent sessions, explicit profile/config, embeddings, and
live prefix reuse. `runtime` reaches it as a client.

## Graduation target

The llama.cpp path is done only when it is a real local node, not a demo wrapper:

- **Persistent sessions:** requests reuse a model/context/session manager instead
  of allocating a fresh context for every call.
- **Live prefix reuse hot path:** the normal coding loop keeps stable KV live,
  reuses it, and only prefills the changed suffix.
- **Snapshot/restore as durability and branching:** stable workspace prefixes can
  be saved, loaded, branched, and invalidated by deterministic segment hashes,
  but snapshots are not assumed to be faster than a hot live session until the
  measurements prove it.
- **Suffix-only prefill:** edited tails are re-prefilled without replaying the
  unchanged prefix.
- **Explicit runtime config:** context size, batch/ubatch, threads, GPU layers,
  tensor split, KV cache type, and Flash Attention are configured through the
  provider/profile surface instead of fixed constants.
- **Cache manifest correctness:** cache identity includes model digest,
  tokenizer digest, chat template digest, backend version, context/RoPE settings,
  KV type, token hashes, token positions, and cache-block alignment.
- **Semantic cache policy:** system/developer prompt, tool schemas, repo
  instructions, repo map, pinned files, diffs, logs, and user turns have explicit
  admission/eviction priority instead of all competing under plain LRU.
- **Bounded failures:** over-context, over-batch, cancelled decode, missing model,
  unsupported binding capability, and invalid session state return structured
  errors. No user-facing path should rely on panic recovery as normal control
  flow.
- **Streaming and non-streaming parity:** both paths use the same session/runtime
  machinery and cancellation semantics.
- **Shared workspace-context layer:** `AssembleContext` is lifted out of OpenVINO and becomes
  the backend-independent driver for both llama.cpp and OpenVINO.
- **Bench-ready instrumentation:** cold prefill, live warm prefix reuse, suffix
  prefill, snapshot save/restore timing, first-token latency, decode tokens/sec,
  and KV snapshot size are measured at the runtime boundary even if we
  temporarily skip formal benchmark reporting.

There must be one user-visible embedded GGUF runtime: `--type llama`. The old
`local` keyword remains accepted only as a compatibility alias that canonicalizes
to `llama`; it is not a separate package or behavior. The retired `localnode`
backend type is not kept.

## Why both

The product layer of the workspace-context design — the deterministic context assembler
(`AssembleContext` in `runtime/modelrepo/openvino/segments.go`) — is already
**substrate-independent Go** with no OpenVINO imports. So owning two backends is
tractable: the differentiated layer ports with limited substrate-specific work,
while the runtime primitives differ. It also de-risks the single-vendor bet and
*validates* that the workspace-context abstraction is real (an abstraction that survives a
second backend is a real abstraction).

```text
OWN  : Contenox session/context layer  -> AssembleContext (built, backend-agnostic)
                                       + live prefix reuse + suffix-only prefill
                                       + optional KV snapshot/restore
REUSE: inference primitives         -> llama.cpp KV + sequence API
REUSE: model/IO + kernels           -> llama.cpp/ggml (GGUF, tokenizer, CPU/CUDA/Metal/Vulkan/SYCL/OpenCL where proven)
```

This is still a profile-gated bridge, not "hardware support for free." CUDA,
Metal, Vulkan, SYCL, OpenCL, Jetson, Snapdragon, and any future NPU-ish path must
each prove the same Contenox gates: sequence/state correctness, warm suffix
equals cold full prompt, KV memory placement, context length, cancellation, and
packaging on the target OS/arch. Alternative-silicon certification lives in
`../alternative-silicon.md`; this document owns the llama.cpp `llama` path.

## llama.cpp gives us the primitives — verified against `llama.h`

| Reuse capability | llama.cpp API | Notes |
|---|---|---|
| Snapshot/restore a session | `llama_state_get_data` / `set_data` for exact resume; `llama_state_seq_*` for sequence memory operations | full context state includes the last logits buffer; sequence-only state is not enough for next-token equality |
| Suffix-only re-prefill | `llama_memory_seq_rm(seq, p0, p1)` | drop the changed tail, append the new suffix |
| Copy-on-write session branch | `llama_memory_seq_cp` | fork a workspace session cheaply |
| Context shift / position ops | `llama_memory_seq_add`, `_div`, `_keep`, `_pos_max` | older docs call these `llama_kv_cache_seq_*`; the vendored version uses `llama_memory_seq_*` |
| KV quant / Flash Attn / chunked prefill | `kvCacheType` q8_0/q4_0, FA flag, `n_batch`/`n_ubatch` | already in the binding's `NewContextParams` |
| GPU offload | `NumGpuLayers`, `TensorSplit` | CUDA/Metal/Vulkan/SYCL/OpenCL where exposed by the linked build; profile-gated |

Jetson-specific note: Jetson Orin NX 16GB is CUDA-capable but shared-memory
constrained. A llama.cpp Jetson profile must measure resident model memory,
load/unload time, swap/zram pressure, and whether large infrequent models are
on-demand instead of permanently warmed. A profile that swaps during daily decode
is not certified for that workload.

Two honest deltas vs OpenVINO:

- **Likely no f16 gotcha.** llama.cpp KV save/restore is exact for the configured
  KV type, so the S0 lossy-default surprise probably does not recur — but verify
  in L0.
- **No sparse attention.** llama.cpp has no XAttention equivalent. The
  "200k-effective via sparsity" lane stays OpenVINO-only; llama.cpp reaches long
  context via KV quant + warm reuse + GPU.

## Cache manifest and correctness gates

The local node must never treat a byte hash alone as a valid KV hit. A reusable
prefix is valid only for the exact runtime profile that produced it:

```text
profile_id
backend + backend_version
model_digest
tokenizer_digest
chat_template_digest
n_ctx / RoPE settings
KV type and Flash Attention setting
segment byte_hash + token_hash
segment token_start/token_end
cache block/page size and alignment
```

Two byte-identical text segments can tokenize differently after a template,
special-token, BOS/EOS, or profile change. That is a mandatory miss. The
`contextasm` manifest should store both byte and token hashes, and llama.cpp L0+
tests must prove profile/template/tokenizer mismatch invalidates rather than
silently reusing stale KV.

Block/page alignment matters because practical KV caches share full blocks more
reliably than arbitrary token spans. The assembler should learn the backend's
cache block size and prefer stable segment boundaries that do not strand large
partial blocks at the end of reusable prefixes.

## Minimum serious runtime config

The graduated local node ships tested profiles. It does not hide behind the
retired `local` provider's magic defaults:

```text
n_ctx
n_batch
n_ubatch
n_threads
n_threads_batch
n_gpu_layers
tensor_split
flash_attn
type_k / type_v
RoPE settings when applicable
seed
sampler config
cache block/page size if exposed or measured
```

Profile changes invalidate cache manifests. Deterministic equivalence tests must
pin seed and sampler config before comparing warm output to cold output.

## The binding architecture: direct Contenox shim

`modeld/llama` uses the Contenox-owned direct llama.cpp shim in
`modeld/llama/llamacppshim`. The shim links against a pinned, generated
`.llamacpp-runtime/<profile>` build and owns the model/context lifecycle. The
old Ollama Go binding and unsafe private-layout shim are not part of the
embedded llama backend.

The direct shim preserves upstream `llama_decode` status distinctions:
- `0`: success
- `1`: could not find KV slot
- `2`: aborted; partially processed ubatches may remain in memory
- `-1`: invalid input batch
- `< -1`: fatal error

That distinction matters because a cancelled or partially decoded prefill can
poison live workspace state. The session layer must either roll back cleanly or
mark the session fatal and evict it; it must not collapse these cases into a
generic "KV full" error.

The shim exposes or must expose:

```text
llama_free(ctx)
llama_decode with exact status mapping
llama_memory_seq_rm/cp/add
llama_state_get_data/set_data
llama_state_seq_get_size/get_data/set_data
tokenization and token-to-piece helpers
model-native chat-template rendering through the pinned minja headers
minimal sampler support needed for deterministic tests
```

Snapshot file helpers (`llama_state_save_file/load_file` or
`llama_state_seq_save_file/load_file`, depending on whether logits are needed)
remain an L4
durability/benchmark addition on top of the current byte-state primitives.

See `binding-ownership-options.md` for the final decision record.

## Spike plan (mirrors S0 / S2 / S2.5)

- **L0 (kill-gate) — owned shim + state round-trip.** Build the Contenox-owned
  llama.cpp shim, then prefill a prompt, save context state, fresh
  context, `load_file`, decode, and assert identical greedy continuation. Also
  prove same prompt + same seed + same sampler config reproduces, snapshot
  save/restore bytes/ms are recorded, `llama_free(ctx)` works, decode status
  preserves aborted/no-slot/invalid/fatal distinctions, and no duplicate
  llama.cpp copy is linked.
- **L1 — production local runtime skeleton.** Create the graduated llama.cpp
  provider/session manager with explicit config parsing, model lifecycle,
  bounded context/batch validation, cancellation, and shared streaming/non-
  streaming decode. This is where the toy constants die.
- **L2 — warm prefix reuse.** Prefill a large stable prefix in a live session;
  for a new suffix, branch/copy the sequence when needed, remove the old tail
  with `llama_memory_seq_rm`, and append only the new suffix. Snapshots may be
  used as a benchmark fixture, but the hot path is live sequence reuse unless
  measurements prove restore is better. Required correctness: warm prefix +
  suffix output equals cold full prompt output under greedy decoding. Required
  curve: changed suffix sizes at 0, 256, 1k, 4k, 8k, and 16k tokens.
- **L2.5 — assembler drives the cache.** Wire the **existing** `AssembleContext`
  to the llama path; same stable segments → warm, edited stable segment → cold.
  Reuse the `segments_integration_test.go` shape verbatim after moving the
  assembler into the shared package. Add token hashes, segment token ranges, and
  profile compatibility checks to the manifest before trusting cache hits.
- **L2.7 — admission and eviction policy.** Pin system/developer prompt, tool
  schemas, and repo instructions for the workspace session; treat repo map,
  pinned files, active-task summary, diffs, test output, logs, and user turns
  according to the priority policy in `local-coding-node-goals.md`.
- **L3 — replace the toy surface.** Route product-facing GGUF inference through
  the `modeld/llama` backend; `runtime` resolves `--type llama` and dials the
  daemon. `--type llama` is the real backend type; `--type local` canonicalizes
  to `llama` for compatibility; `localnode` is retired.

## Reaching the goals on the 6 GB test bench

Goals are in `local-coding-node-goals.md`. The dev bench has a **6 GB VRAM** GPU
— small, but enough to prove the design and finally get the GPU number the
OpenVINO CPU runs could not.

**Bench config.** Use a small coding model fully GPU-offloaded so VRAM holds the
**KV cache**, not just the weights:

```text
model : Qwen2.5-Coder 1.5B-3B Q4   (weights ~1-2 GB) -> NumGpuLayers = all
KV    : q8_0 (or q4_0 for max context); Flash Attention on
prefix: a large stable "repo context" (tens of k tokens) so prefill cost is
        non-trivial even on GPU and warm reuse is visibly cheaper
note  : 7B Q4 (~4.5 GB) also loads but leaves little KV room -> not the bench's
        job; that is budget-node (16 GB) tier validation
```

**Why a small model is still a valid proof.** The reuse mechanism is
**model-size-independent** at the API level — sequence reuse, suffix prefill, and
snapshot/restore have the same correctness contract at 1.5B and 7B; only the
absolute times, KV sizes, and memory pressure change. So the bench proves the
*design*; full-size Tier numbers belong on the budget node.

| Goal (from goals doc) | Spike / milestone | Provable on the 6 GB bench? |
|---|---|---|
| KV snapshot/restore works | L0 | yes — round-trip on GPU-resident KV |
| Warm reuse collapses cold prefill | L2 | yes — cold-vs-warm ratio + GPU tok/s (the headline) |
| GPU is the answer for cold prefill | L2 | yes — directional GPU-vs-CPU delta |
| Assembler drives the cache | L2.5 | yes — same/edited stable segment -> warm/cold |
| Real local node, not a toy | L1 / L3 | yes — session mgr, explicit config, bounded failures, streaming parity |
| Own both backends behind shared context layer | shared `AssembleContext` | yes — side-by-side llama.cpp + OpenVINO |
| **T1/T2/T3 at full model + context** | **L4 (budget node)** | no — needs the 16 GB Arc/budget node; deferred, defined below |

**L4 — budget-node tier validation (deferred, defined).** On the real ~1.5k EUR
Intel node (16 GB Arc / NPU), run the same graduated runtime + `AssembleContext`
with a 7B/8B coder at 64k -> 128k and record cold prefill, live warm prefix
reuse, suffix-only prefill, snapshot save/restore timing, first-token latency,
decode tok/s, and KV snapshot size against the T1/T2/T3 goals. **The bench
proves the design; L4 proves the product target.**

The report shape is the one in `local-coding-node-goals.md`: cold full prefill,
warm same-prefix, warm changed-suffix, edited-stable-segment miss, snapshot
save/restore, decode, and failure cases. The most important single graph is TTFT
as changed suffix grows; a single "warm is faster" number is not enough.

L2/L4 prove the runtime mechanism. They do not by themselves prove 200k
effective coding context. That requires the coding-context eval gate in
`local-coding-node-goals.md`: cross-file bug localization, trace failing test to
implementation, large refactor with usage search, and repo architecture answers
with citations.

## Structure

- **CGO session work in `modeld/llama/`.** The old `local` and `localnode`
  packages are deleted after their useful behavior is absorbed. `modeld/llama`
  holds the build-tagged deep-binding subpackages (`llamasession/` now, owned
  shim next) behind a `Session` abstraction for prefix, suffix, decode,
  explain-context, and later snapshot/restore/branch. Model catalog, backend
  registration, and the `modelprovider` wrapper that bridges stateless `Chat()`
  to the daemon stay in `runtime` (pure Go), mirroring the package shape with
  `provider.go` / `catalog.go` / `client.go`.
- **Current llama state:** `EnsurePrefix`, `PrefillSuffix`, and `Decode` are
  wired as live-session primitives. Prefix/suffix inputs now carry a
  `ContextManifest` with profile ID, backend version, model digest, prompt
  format/template digest, runtime digest, BOS policy, stable byte hash, rendered
  segment byte ranges, backend-resolved stable token hash, and per-segment token
  ranges/hashes populated by the active tokenizer. Incompatible
  profile/runtime/template changes clear resident KV before token LCP reuse; a
  segment boundary that cannot be proven token-aligned is rejected as a manifest
  mismatch.
- **Tiny GGUF proof exists.** An opt-in `CONTENOX_LLAMA_TINY_GGUF` test
  refuses models over 512 MiB and verifies real llama.cpp tokenization,
  prefix/suffix prefill, manifest segment token ranges, and one-token warm/cold
  equivalence. This is a correctness fixture, not a performance benchmark.
- **Prompt formatting is profile-declared.** `contenox-llama.json` now
  accepts `prompt.format` (`chatml` or `llama3`), `prompt.template_digest`, and
  `prompt.add_bos`. Unknown formats and tool-call message history are rejected
  instead of serialized through an accidental fallback.
- **Lifecycle remains a shim gate.** The llama adapter clears KV, frees the
  owned batch, evicts fatal sessions, treats failed KV rollback/removal as fatal,
  and avoids freeing the model while an unfreed context exists. Full
  deterministic cleanup now belongs to the Contenox-owned shim exposing
  `llama_free(ctx)` and exact decode status mapping.
- **Lift `AssembleContext`** (segments.go + segments_test.go) into a shared
  pure-Go package in `runtime` (e.g. `runtime/contextasm`), imported by the
  runtime wrappers that drive both the `modeld/llama` and `modeld/openvino`
  backends. Add tokenization cache, manifest generation, profile compatibility
  checks, and `explain-context`. The assembler is non-CGO and stays in
  `runtime`; it feeds segments and tokens to `modeld` over the transport. This
  refactor is the concrete proof the workspace-context layer is
  substrate-independent.
- **`Makefile.llamacpp-direct`** owns the pinned llama.cpp source/runtime build.
  It must build exactly one linked llama.cpp copy plus `common`, and run
  tiny-model tests against the direct runtime.

## Non-goals

- Not removing or abandoning OpenVINO — paused; the validated design replicates
  back, and the shared `AssembleContext` already serves both.
- Not preserving separate `local` or `localnode` implementations. `local` is a
  compatibility keyword only; all behavior routes through `llama`.
- Not chasing 200k-via-sparse on llama.cpp (OpenVINO's lane).
- Cold prefill stays compute-bound on both engines; GPU is the answer — llama.cpp
  just makes the GPU demo faster to reach.

## Verification

- `make -f Makefile.llamacpp-direct test-shim-cpu` — direct shim build/link.
- Native `modeld/llama/llamasession` tests — snapshot round-trip + warm/cold
  when `CONTENOX_LLAMA_TINY_GGUF` is set.
- Warm suffix output equals cold full prompt output under greedy decoding.
- Profile/tokenizer/template mismatch causes a cache miss, not stale reuse.
- Cancellation during prefill/decode has a tested structured outcome.
- Shared `AssembleContext` unit tests stay green in the **default build** for both
  backends (no CGo, CI-safe).
- **Side-by-side proof:** the same `AssembleContext` segments drive warm reuse on
  *both* llama.cpp and OpenVINO — the payoff of the OpenVINO+llama.cpp strategy
  and the evidence the workspace-context abstraction is real.

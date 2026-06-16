# llama.cpp Llama Plumbing Log

Date: 2026-06-16

Status: implementation log for the merged `runtime/modelrepo/llama` package.

This log records the implementation pass after `plan-llamacpp.md`: merge the
old `local` and `localnode` paths into the feature-complete `llama` package and
turn that runtime into explicit primitives instead of a toy wrapper.

Update later the same session: the higher-priority gaps were promoted ahead of
snapshot/restore:

- lifecycle/error handling;
- model-profile prompt formatting;
- manifest/profile compatibility for warm reuse.

## Target

Move toward the local coding-node goals without pretending snapshot/restore is
solved yet:

- expose `llama` as the user-facing serious backend type;
- keep `local` only as a compatibility keyword that canonicalizes to `llama`;
- remove the old `runtime/modelrepo/local` and `runtime/modelrepo/localnode`
  packages once their behavior is absorbed;
- make the live hot path explicit: `EnsurePrefix -> PrefillSuffix -> Decode`;
- return typed errors and primitive status so later benchmark/report code can
measure warm reuse, suffix prefill, overflow, cancellation, and unsupported
surfaces.

## What Was Wired

- `runtime/modelrepo/llama/` defines the backend-neutral session contract.
- `runtime/modelrepo/llama/llamasession/` registers the llama.cpp adapter
  behind the `llamanode` build tag.
- `runtime/runtimestate/catalogimports.go` blank-imports both `llama` and
  the build-tagged llama adapter so the session factory registers when present.
- `runtime/backendservice` accepts backend type `llama`.
- `runtime/backendservice` accepts `local` as a compatibility alias for `llama`
  and rejects the retired `localnode` type.
- `runtime/runtimestate` reconciles `llama` through the local model-directory
  scanner path and canonicalizes old `local` rows/config to `llama`.
- CLI help and setup diagnostics now mention `llama` explicitly:

```sh
contenox backend add llama --type llama --url ~/.contenox/models/
# compatibility only:
contenox backend add local --type local --url ~/.contenox/models/
```

## Primitive Contract

The session API now reports more than success/failure:

```go
EnsurePrefix(ctx, PrefixInput{Text, Manifest}) -> PrefixStatus
PrefillSuffix(ctx, SuffixInput{Text, Manifest}) -> SuffixStatus
Decode(ctx, DecodeConfig) -> StreamChunk
ExplainContext() -> ContextReport
```

`PrefixStatus` reports reused, dropped, prefilled, resident, and available
tokens plus stable byte/token hashes and manifest digest. `SuffixStatus` reports
suffix tokens, prefix tokens, resident tokens, remaining context capacity, and
manifest digest. `DecodeConfig` now carries `Seed`, which the llama.cpp sampler
receives, so cold-vs-warm equivalence tests can be deterministic later.

## Prompt Profile Contract

`contenox-llama.json` now has explicit prompt/profile identity fields:

```json
{
  "profile_id": "qwen-coder-7b-llama",
  "model_digest": "sha256...",
  "prompt": {
    "format": "llama3",
    "template_digest": "sha256...",
    "add_bos": true
  },
  "runtime": {
    "num_ctx": 65536,
    "num_batch": 1024,
    "flash_attention": true,
    "kv_cache_type": "q8_0"
  }
}
```

Supported prompt formats are deliberately small and profile-declared:

- `chatml` — current fallback/default.
- `llama3` — Llama 3 header/eot format, with BOS controlled by `prompt.add_bos`.

Unknown prompt formats are rejected as `ErrUnsupportedFeature`. Tool-call
messages and tool history are also rejected until llama has a
profile-declared tool protocol/parser path. This avoids silently serializing a
tool protocol the model did not declare.

The prompt planner keeps only leading `system` messages in the stable prefix.
Once a volatile message appears, later messages stay volatile so rendering does
not reorder the conversation.

## Manifest Contract

Every llama turn now builds a `ContextManifest` from the actual rendered
bytes:

- profile ID;
- llama.cpp backend version;
- GGUF model digest (profile-supplied or cached SHA-256 over `model.gguf`);
- prompt format/template digest;
- runtime digest (`num_ctx`, batch, GPU layers, tensor split, Flash Attention,
  KV type);
- BOS policy;
- stable byte hash;
- backend-resolved stable token hash;
- rendered segment byte ranges and byte hashes;
- backend-resolved per-segment token ranges and token hashes for stable and
  volatile segments.

The session cache key includes model digest, runtime config, prompt
format/template, and BOS policy. Replacing a GGUF at the same path no longer
reuses the old loaded session key.

Warm reuse now has two gates:

1. Manifest runtime identity must be compatible. Model, backend, profile,
   prompt template, runtime config, or BOS drift clears resident KV.
2. If identity is compatible, llama.cpp still performs token-level
   longest-common-prefix reuse, so an edited stable prefix can reuse only the
   unchanged token prefix and refill the divergent tail.

## Error Contract

Added typed errors:

- `ErrSessionUnavailable`
- `ErrSessionClosed`
- `ErrContextOverflow`
- `ErrUnsupportedFeature`
- `ErrManifestMismatch`
- `ErrSessionFatal`

`ContextOverflowError` carries:

```text
stage
resident_tokens
additional_tokens
num_ctx
```

The llama adapter now returns typed overflow/closed errors instead of plain
strings. Tool calls are explicitly rejected as `ErrUnsupportedFeature` until a
profile-declared parser/protocol path exists for llama.

`ErrManifestMismatch` is returned when a suffix is paired with resident KV from a
different manifest. `ErrSessionFatal` marks a backend state that must be evicted
from the session cache rather than reused.

## Context Safety

The llama adapter now checks context capacity before prefix, suffix, and decode
growth. If suffix prefill fails or is cancelled after partial decode, it removes
the partially-written KV tail and leaves the resident-token bookkeeping at the
stable prefix. This is still live-session reuse only; state save/restore remains
blocked until the Contenox-owned llama.cpp shim exposes `llama_state_seq_*`.

Rollback is now fatal on KV-remove failure. `KvCacheSeqRm` returning false means
the in-memory KV and resident-token bookkeeping can no longer be trusted, so the
adapter closes the session, returns `ErrSessionFatal`, and the client evicts that
session from the cache.

Segment token ranges are populated through the active backend tokenizer. If a
declared segment boundary cannot be proven token-aligned under that tokenizer,
manifest population fails as `ErrManifestMismatch` instead of inventing a range.
The suffix path also validates that tokenizing `stable+suffix` as a cold full
prompt equals the warm path's stable tokens plus suffix tokens; tokenizer merges
across the split are rejected as manifest mismatches.

Lifecycle is stronger but not fully solved:

- `Close` now clears llama KV, frees the owned batch, nils resident bookkeeping,
  and reports closed state through `ExplainContext`.
- decode panics and decode-step failures mark the session fatal and evict it from
  the llama cache.
- model/context cleanup is still not fully owned because Ollama's Go binding
  exposes `llama_model_free` but not `llama_free(ctx)`. We do not call
  `FreeModel` while an unfreed context still references it. Fully deterministic
  resource release is now assigned to the Contenox-owned llama.cpp shim.

## Verification

Passed:

```sh
go test -count=1 ./runtime/modelrepo/llama/...
go test -count=1 ./runtime/backendservice ./runtime/runtimestate ./runtime/internal/setupcheck ./runtime/contenoxcli
```

The tiny GGUF test is opt-in and refuses models over 512 MiB. A previous local run used
`/home/naro/.libollama/models/tiny/model.gguf` (323 MiB). It exercised real
llama.cpp tokenization, prefix/suffix prefill, manifest token-range population,
and a one-token warm suffix continuation that matched a fresh cold session under
the same seed/sampler config.

Skipped by operator request during this pass:

```sh
go test ./runtime/modelrepo/...
```

The broader package test was intentionally stopped; focused llama and affected
consumer package tests are the current verification signal.

## Still Open

- Extend the tiny one-token warm/cold proof into a benchmark-grade multi-token
  L0/L2 report.
- Add benchmark JSON output for cold prefill, warm same-prefix, changed suffix,
  snapshot timing, decode speed, and failure cases.
- Build the Contenox-owned llama.cpp shim for both `llama_free(ctx)` lifecycle
  ownership and `llama_state_seq_*` snapshot/restore.

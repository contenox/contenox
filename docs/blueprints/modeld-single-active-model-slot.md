# Plan: modeld Single Active Model Slot

> **Status:** decision blueprint, drafted 2026-06-19.
> **Sibling docs:** `modeld-interface-boundary.md`,
> `modeld-provisioning-detection.md`,
> `local-runtime-multi-client-coordination.md`,
> `local-runtime-owner-coordination.md`.
> **Purpose:** make `modeld` the owner of one resident local model at a time,
> while runtime can still discover, select, load, unload, and switch local
> models without restarting the process.

---

## Problem Statement

The current `modeld` contract is already the right boundary for local compute:
runtime owns planning, messages, tools, model selection, and catalog state;
`modeld` owns resident local compute and the stateful
`OpenSession -> EnsurePrefix -> PrefillSuffix -> Decode` contract.

The problem is the cardinality behind that boundary.

Today `modeld` serves one backend mode at a time (`llama`, `openvino`, or
`none`), but it can open more than one resident session/model. The gRPC transport
keeps a map of handles to sessions, and the runtime-side warm cache limits
resident sessions only per runtime process. A second frontend or a direct client
can still cause extra local models to be resident. That weakens the whole reason
for the daemon: predictable local memory ownership.

There is also a UX gap:

```text
model list
  Correctly reports runtime/kernel availability: models available for use ASAP.

local model library / installed models
  Missing surface: models present on disk or known to runtime, even when they
  are not currently active in modeld.
```

Cloud providers and Ollama hid this distinction because "selectable" and
"runnable" were effectively the same thing. Local `modeld` makes the distinction
real: an installed artifact may be selectable, but not loaded. One loaded model
may be runnable, but switching to another model requires evicting the active
slot first.

## Decision

One `modeld` owner means one active local model slot.

The invariant is:

```text
At most one local chat/generation model is resident in one modeld owner.
At most one turn uses that resident slot at a time.
Runtime may load, unload, and switch the slot at runtime.
Runtime may not keep multiple local generation models resident through modeld.
```

This makes `modeld` a local accelerator owner, not a small multi-model serving
cluster. It also keeps the UX honest: a user can have many installed models, but
only one active local model.

## Terms

```text
library model
  A model known to runtime and/or present on disk. It may be selectable in the UI
  even when modeld is stopped or serving another model.

active model
  The one model identity currently loaded in modeld's slot.

ready model
  The active model is loaded, healthy, not currently busy, and compatible with
  the requested model/config identity.

busy slot
  The active model is in a protected operation: load, unload, switch, prefix
  prefill, suffix prefill, decode, snapshot, or restore.

slot generation
  A monotonically increasing epoch for the active slot. It changes on every
  load, unload, switch, owner takeover, or fatal backend reset.

resident session
  The backend session/context/KV state inside the active slot.
```

The active identity includes more than the model path:

```text
backend type
model name
model digest
model path
runtime digest
device identity
NumCtx
NumBatch
NumGpuLayers
TensorSplit
FlashAttn
KVCacheType
prompt format/template digest
BOS policy
reasoning format
```

If any compatibility field changes, treat it as a different active identity and
require a reload/switch.

## Non-Goals

- Not turning `modeld` into a multi-model inference server.
- Not moving cloud providers behind `modeld`.
- Not redefining `model list` to mean "installed local model library".
- Not keeping two local generation models resident for faster switching.
- Not silently switching away from an active model in the middle of a decode.
- Not promising durable active-slot recovery across daemon crashes.

## UX Surfaces

Keep the runtime state surface and add the missing inventory surface.

```text
model list
  Live runtime/kernel availability. This continues to mean "available ASAP from
  the current runtime state." For local modeld, that usually means the active
  loaded model plus any cloud/Ollama providers that are reachable.

model library / model installed / model local
  Offline inventory. Lists local artifacts and metadata even when modeld is not
  running, the daemon is serving a different backend, or the model is not loaded.

model status / modeld status
  Daemon state. Shows owner instance, backend, active model, slot generation,
  busy/ready state, memory estimate, effective context, last load error, and
  whether the active slot is stale or unreachable.
```

Selection should be explicit:

```text
installed but not active  -> Activate
active and ready          -> Use
active and busy           -> Wait / Cancel if supported
different active model    -> Switch
active model              -> Unload
daemon missing/dead       -> Start modeld / install modeld
```

This preserves the technical meaning of `model list` while giving users the
thing they expected: "show me the models I can choose from."

## Transport Contract Changes

The current `runtime/transport.Service` exposes:

```go
OpenSession(ctx, OpenSessionRequest) (Session, error)
Describe(ctx, OpenSessionRequest) (ModelInfo, error)
Embed(ctx, EmbedRequest) (EmbedResult, error)
```

Add daemon-slot operations without moving high-level runtime planning into
`modeld`:

```go
Status(ctx context.Context) (DaemonStatus, error)
LoadModel(ctx context.Context, req LoadModelRequest) (ActiveModel, error)
UnloadModel(ctx context.Context, req UnloadModelRequest) error
```

`OpenSession` remains the entry point for the session contract, but it must stop
meaning "create another resident model." New behavior:

```text
OpenSession(active identity)
  If the slot is empty:
    Either fail with ErrModelNotActive or load only when the request explicitly
    allows compatibility-mode auto-load.

  If the slot has the same identity:
    Return a handle bound to the current slot generation.

  If the slot has a different identity and is idle:
    Fail with ErrModelSwitchRequired unless the request explicitly allows switch.
    If switching is allowed, close the old slot before loading the new model.

  If the slot has a different identity and is busy:
    Fail with ErrModelBusy.
```

Prefer explicit `LoadModel`/`UnloadModel` from runtime. Compatibility auto-load
can exist for old call paths, but it should be visible in logs and tests so it
does not become hidden policy.

Add typed errors:

```text
ErrModelBusy
ErrModelNotActive
ErrModelSwitchRequired
ErrModelLoadFailed
ErrInsufficientMemory
ErrSlotGenerationStale
ErrBackendMismatch
ErrSessionClosed
ErrStaleFence
```

Every session handle must include both owner instance and slot generation. A
handle from an old generation fails after unload/switch/crash/takeover.

## Slot State Machine

```text
Empty
  -> Loading

Loading
  -> Ready
  -> Failed
  -> Empty

Ready
  -> Busy
  -> Unloading
  -> Switching

Busy
  -> Ready
  -> Failed

Switching
  -> Unloading old slot
  -> Loading new slot
  -> Ready
  -> Failed
  -> Empty

Unloading
  -> Empty

Any state
  -> ShuttingDown
  -> LostOwner
  -> Failed
```

Rules:

- Switch is never in-place. Close the current backend session, clear handles,
  bump the slot generation, then load the new model.
- A turn is serialized as one protected operation:
  `EnsurePrefix -> PrefillSuffix -> Decode`.
- Snapshot/restore, if supported, is also protected by the same slot lock.
- `Describe` can remain non-resident and advisory, but memory must be checked
  again during actual load.

## Safety Nets and Gotchas

### Loaded Model Then Switching Model

Switching drops warm state by design.

Safety requirements:

- Require explicit switch intent from runtime/UI unless this is a documented
  compatibility path.
- Reject switch while the slot is busy, unless the caller explicitly asks to
  cancel and the backend can prove cancellation leaves the slot consistent.
- Close the old backend session before loading the new one so VRAM/RAM is
  actually released before the new allocation.
- Bump slot generation before exposing the new slot.
- Invalidate all old session handles.
- If new load fails after old unload, leave the daemon in `Empty` or `Failed`,
  not half-switched.
- Do not try to silently restore the old model after a failed switch unless the
  implementation can prove the old model was never closed. Prefer memory safety
  over surprise reactivation.

User-visible consequence:

```text
"Switching from qwen2.5 to codellama will unload qwen2.5 and drop its warm
context."
```

### Sudden modeld Shutdown or Crash

The active slot is volatile.

Safety requirements:

- Runtime treats unreachable transport, expired lease, failed health probe, or
  owner mismatch as loss of active slot.
- Runtime drops all warm handles when the owner fence or slot generation is
  stale.
- On modeld restart, `Status` reports `Empty` unless a future explicit autoload
  feature is added.
- Do not persist and trust "active model" as proof that the model is resident.
  Persisted state can be a hint for UI, never a compute guarantee.
- In-flight streams fail fast with a typed transport/owner error.

### Lease Loss and Takeover

`modeld` already self-fences on owner loss. The single-slot rule makes that more
important:

- On `owner.Lost()`, stop accepting new RPCs.
- Cancel or let active RPCs drain only if they can finish before shutdown policy.
- Close the active slot.
- Release the lease when possible so a successor does not wait for TTL.
- New owner starts with a fresh owner instance and slot generation.
- Runtime must never reuse a session handle across owner instance changes.

### Other VRAM/RAM Consumers

Memory availability is not stable. A browser, game, editor, display server,
training job, or another inference stack can consume memory after `Describe`.

Safety requirements:

- Treat `Describe` as advisory capacity planning only.
- Recheck free memory immediately before every `LoadModel` or switch.
- Keep a configurable reserve (`--mem-reserve` / policy `MinFreeBytes`) for the
  desktop and unrelated workloads.
- Return `ErrInsufficientMemory` with required/free/reserve fields when load
  cannot fit.
- If memory pressure appears while a model is already active, do not kill the
  slot mid-turn unless the backend has a safe cancellation/reset path.
- At turn boundaries, `Status` may report pressure and runtime can warn or offer
  unload.
- Effective context is part of active identity. If memory pressure changes the
  resolved context, a reload/switch is required.

### Multiple Clients and Races

The slot manager must be the authority, not each runtime process.

Safety requirements:

- One mutex/state machine serializes load, unload, switch, snapshot, restore,
  and the full turn.
- `LoadModel` and `UnloadModel` should support compare-and-swap fields:
  expected active identity and expected slot generation.
- A forced switch should be explicit and auditable.
- Status should expose enough detail for clients to explain why a request is
  blocked: busy operation, active model, owner instance, and generation.
- Runtime-side warm cache for local modeld must be slot-aware and capped at one
  resident local slot. The current per-process warm cache must not imply daemon
  residency beyond the single-slot invariant.

### Same Model, Different Config

"Same file" is not necessarily "same active slot."

Safety requirements:

- Treat config as part of identity.
- If `NumCtx`, GPU layer count, KV cache type, prompt template digest, BOS
  policy, runtime digest, or device selection differs, require reload.
- If a client asks for a compatible subset of the active config, define that
  explicitly. Do not rely on accidental reuse.

### Embeddings

Embedding requests do not participate in chat KV reuse, but they still consume
memory.

Decision for the first implementation:

```text
Embedding may not cause a second resident generation model.
```

Allowed options:

- Run embedding as one-shot only when the backend can prove it does not create a
  second resident slot.
- Use the single active slot and evict/switch like any other local model.
- Return `ErrModelBusy` or `ErrModelSwitchRequired` if embedding would violate
  the invariant.

Do not special-case embeddings into an unbounded second cache.

### Backend Differences

Both llama.cpp and OpenVINO must implement the same slot semantics.

- llama.cpp slot contains the loaded model/context and KV state.
- OpenVINO slot contains the GenAI pipeline/session and its prefix reuse state.
- Backend-specific caches are allowed only inside the active slot.
- Backend-specific pool implementations must not keep another model alive after
  the slot switches.

### Cancellation

Cancellation has to leave the daemon in a known state.

Safety requirements:

- If canceling decode leaves the backend session valid, return the slot to
  `Ready`.
- If validity cannot be proven, close the slot, bump generation, and report
  `Empty` or `Failed`.
- Cancellation must unblock switch/unload after the backend reaches a safe
  boundary.
- Runtime should assume a canceled turn may have invalidated its session handle
  unless `Status` confirms the same generation is still ready.

### External Process Kill

If the OS kills modeld or the user sends `kill -9`, cleanup code does not run.

Safety requirements:

- Lease TTL and health probe are the recovery path.
- Runtime must surface stale owner/unreachable daemon as "modeld stopped" rather
  than "model unavailable forever."
- Successor modeld starts empty and publishes a new owner instance.
- Partially written status files, if any are added later, must be ignored unless
  they are atomically written and match the live owner.

## Implementation Plan

### P0: Blueprint and Tests First

- Land this blueprint.
- Add tests around the intended slot semantics before changing backend behavior.
- Name the invariant directly in tests: "one active model slot."

### P1: Slot Manager

Add a daemon-side slot manager that wraps the backend `transport.Service`.

Responsibilities:

- Hold active identity, active session, slot generation, status, and last error.
- Serialize load/unload/switch/turn operations.
- Close old sessions on switch/unload.
- Invalidate stale handles.
- Expose active status without requiring runtime to inspect backend internals.

Likely placement:

```text
modeld/internal/slot
  or
runtime/transport/grpc with a modeld-owned service wrapper
```

Prefer a modeld-owned wrapper so the generic gRPC transport remains mostly a
wire adapter, not policy.

### P2: Transport API

Extend `runtime/transport` and the gRPC adapter:

- `Status`
- `LoadModel`
- `UnloadModel`
- active model/status structs
- typed slot errors
- handle format including owner instance and slot generation

Keep runtime as the owner of the transport interface. `modeld` implements it.

### P3: Runtime Local Provider Wiring

Update local runtime providers to treat `modeld` as one active slot:

- Before using a local model, compare requested identity with `Status`.
- Call `LoadModel` or `OpenSession` with explicit switch intent.
- Drop runtime warm cache entries when owner or slot generation changes.
- Ensure local `WarmCacheMaxResident` cannot keep multiple daemon models alive.
- Preserve cloud/Ollama behavior outside this path.

### P4: Local Library Surface

Add the missing offline inventory surface separate from `model list`.

Rules:

- Scan installed model artifacts without requiring `SessionAvailable()`.
- Show backend type, digest/path, size, compatibility, and active status when
  modeld is reachable.
- Do not imply "loaded" when only "installed" is true.
- From the library view, offer activate/switch/unload actions.

### P5: Status Surface

Extend `modeld status` and/or runtime status output:

```text
owner instance
backend
endpoint
active model identity
slot generation
state: Empty / Loading / Ready / Busy / Switching / Unloading / Failed
busy operation
effective context
memory required/free/reserve
last load/switch error
```

This is needed for debugging "why can I not select this model?" without
requiring logs.

### P6: Backend Enforcement

Apply the slot manager to both local backends:

- llama.cpp service
- OpenVINO service
- no-backend service

The no-backend service should report running daemon status but fail load with a
typed backend-unavailable error.

## Test Plan

Unit tests:

- Loading first model transitions `Empty -> Loading -> Ready`.
- Loading the same identity reuses the active slot and generation.
- Loading a different identity while ready requires explicit switch intent.
- Switching closes the old session before opening the new one.
- Switching bumps generation and invalidates old handles.
- Switching while busy returns `ErrModelBusy`.
- Unload is idempotent.
- Load failure leaves the daemon in `Empty` or `Failed`, never with two sessions.
- Cancellation either returns the same generation to `Ready` or closes the slot
  and bumps generation.
- Owner mismatch returns `ErrStaleFence`.
- Slot generation mismatch returns `ErrSlotGenerationStale`.
- Describe remains non-resident.
- Embed cannot create a second resident generation model.

Integration tests:

- Two runtime clients racing to load different models result in one winner and
  one typed busy/switch/stale error.
- Runtime drops warm handles after modeld restart.
- Runtime can list installed local models while modeld is stopped.
- Runtime can activate an installed model, use it, unload it, and activate a
  different installed model without process restart.
- Fake memory pressure between `Describe` and `LoadModel` fails with
  `ErrInsufficientMemory`.
- llama.cpp and OpenVINO both obey the same slot state machine.

Manual/e2e checks:

- Start modeld, load model A, run a turn, switch to model B, verify model A is
  no longer resident.
- Kill modeld during decode, restart, verify runtime reports empty/stale active
  slot and old handles fail.
- Start another VRAM-heavy process before load, verify the load failure is
  actionable.
- Open two editor/CLI clients and verify the status surface explains active and
  busy state consistently.

## Acceptance Criteria

- `modeld` never has more than one resident local generation model.
- Runtime can load, unload, and switch local models without restart.
- Switching never leaves stale session handles usable.
- A crashed or replaced modeld owner never appears as a still-loaded model.
- `model list` keeps its live runtime-state meaning.
- A separate local library/inventory view lists installed models even when they
  are not loaded.
- Memory errors identify required/free/reserve values instead of failing deep in
  backend initialization.
- llama.cpp and OpenVINO share the same user-visible slot semantics.

## Open Questions

- Should `OpenSession` auto-load an empty slot for compatibility, or should all
  new runtime code call `LoadModel` first?
- When the slot is busy, should a switch request fail immediately, queue, or
  offer cancellation?
- Should embeddings evict the active chat model, run only when idle, or require
  a separate explicit embedding mode?
- Should modeld support optional autoload of the last active model on startup,
  or always restart empty?
- What should the final CLI name be for the offline inventory surface:
  `model library`, `model installed`, or `model local`?
- Should active slot state be persisted as UI hint metadata, and if so where?
- Should memory pressure warnings be polled or only computed on load/switch and
  turn boundaries?

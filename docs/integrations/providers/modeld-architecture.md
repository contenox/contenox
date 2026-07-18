---
title: Modeld Architecture
description: How modeld owns hardware, plans capacity, and manages sessions.
---

# Modeld Architecture

This page gives a technical overview of the major systems inside `modeld`. For day-to-day usage see the [main modeld page](/docs/integrations/providers/modeld/).

See also the broader [Local Models (GGUF)](/docs/integrations/providers/local-models/) documentation.

## Lease & ownership

modeld uses a simple file-based cooperative lease (`modeld.lease` in the data root).

- Only the current lease holder may load models or open sessions for that root.
- The lease record advertises the gRPC endpoint and the active backend ("llama" / "openvino").
- Followers (other `contenox` processes) read the lease to discover who to talk to.
- The owner renews on a timer and self-fences on renewal failure.

This design gives safe multi-client sharing (CLI + VS Code + Beam + ACP) without a central server process owning everything.

A second, fixed-location device lease (per accelerator) prevents two daemons from both trying to make models resident on the same GPU.

## Slot model (single active model)

The `slot` layer enforces one resident model at a time:

- `OpenSession` / `LoadModel` will evict the previous resident if necessary.
- Generation numbers + fencing prevent stale clients from using an old model after a switch.
- Explicit `LoadModel` vs implicit sessions (from chat) have slightly different residency rules (idle reaper behavior).

Idle reaping (`--idle-ttl`, default 5m) unloads the resident model after inactivity. This is deliberately cheap (no GPU work) so a 24/7 modeld on a laptop doesn't keep the GPU powered.

## Capacity planner

This is modeld's most distinctive piece.

When asked to open or describe a model, modeld:

1. Parses basic model metadata (layer count, KV heads, head dim, sliding window pattern, file size).
2. Takes a live free-memory snapshot of the target device (system RAM or accelerator VRAM).
3. Applies policy (max resident, min free/reserve, headroom, min-hot-context floor, cold budget).
4. Computes:
   - `EffectiveContext` — what dense context the model can actually serve right now.
   - `HotContextTokens` — physical hot KV budget.
   - `PlannerEffectiveContext` — logical window including host-RAM cold store.
   - GPU layer offload count (llama path sheds layers to guarantee usable hot context).

KV cost calculation understands windowed attention (global vs windowed layers) so it doesn't over-estimate for models like Mistral / Qwen with sliding windows.

The result is reported back to the runtime in `ModelInfo` and used for:
- Prompt shifting / compaction decisions
- User-facing context size indicators
- Clamping requested `num_ctx`

## Transport boundary

The runtime never links llama.cpp or OpenVINO. It talks to modeld over gRPC using the `runtime/transport` contract:

- `OpenSession` / `Describe` / `Embed`
- `EnsurePrefix` → `PrefillSuffix` → streaming `Decode`
- `NodeAdmin` (ListModels, ReceiveModel/Push, DiskStats, RemoveModel)

All calls are fenced with the owner instance ID.

## Session & residency

Persistent sessions on the wire keep KV hot. The runtime can:
- Snapshot / restore session state (for branching, durability)
- Evict ranges under a residency policy (for very long contexts)
- Use host-RAM cold KV blocks when configured

modeld's slot + reaper decide when the underlying native session can be closed.

## Remote Modeld

Exactly the same binary and protocol. The runtime just dials a different address (registered as a `modeld` backend type) and uses the same `Endpoint` / `ModeldTarget` machinery.

Push of models happens over the same `PushModel` / `ReceiveModel` path.

## Related deep material

- [modeld Local Inference Landscape](/docs/development/modeld-local-inference-landscape/)
- Blueprints under `development/blueprints/modeld/` (capacity, single slot, owner coordination, cold store, etc.)
- Source build and release runbooks

This architecture is deliberately opinionated: one user, one active model, long-lived stateful sessions, hardware-aware budgets. It is not trying to be a general multi-tenant inference server.
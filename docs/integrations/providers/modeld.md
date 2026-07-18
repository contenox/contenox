---
title: Modeld
description: The local inference daemon that powers Contenox's on-device models.
---

# Modeld

`modeld` is Contenox's dedicated local inference daemon. It runs on your machine (or a remote GPU box), owns hardware resources, and serves GGUF (via llama.cpp) and OpenVINO models to the runtime over gRPC.

It is the foundation of the "local by default" experience.

## Why a separate daemon?

The runtime (`contenox`) is pure Go and talks to many providers. For local hardware, a specialized daemon makes sense because:

- **Ownership & coordination**: Multiple front-ends (CLI, VS Code, Beam, Zed via ACP) need to share the same resident model state without fighting over GPU memory.
- **Capacity that actually fits**: Real context budgets depend on live free memory, model weights, KV cache costs (including GQA, sliding windows), headroom, and desktop reserves. modeld owns this calculation.
- **Efficient residency**: Single active model slot + idle reaper (default 5 minutes) to return GPU memory to the system when nothing is happening.
- **Warm sessions**: Persistent KV state, prefix reuse, and durable snapshots across turns and restarts.
- **Remote nodes**: The same daemon binary can run on a dedicated machine and be registered as a backend.

## Core concepts

### Lease-based ownership

modeld claims a lease file (`~/.contenox/modeld.lease`). Only the lease owner serves inference for that data root. Other processes become followers and discover the owner via the lease record (which also advertises the gRPC endpoint and active backend).

This is cross-process and survives restarts (with self-fencing).

### Single slot + capacity planner

Only one model is resident at a time. When opening a session, modeld:

- Inspects the model (layers, heads, context length, sliding window pattern)
- Takes a live memory snapshot of the device
- Computes what context window actually fits after weights + KV + overhead + reserves
- Can shed GPU layers to guarantee a minimum usable "hot" context (default 4k)

It returns rich info: `effective_context`, `hot_context_tokens`, `planner_effective_context`, `clamped`, `reason`, device stats, etc.

### Idle reaping

After the configured idle TTL with no activity, modeld unloads the resident model. This is CPU-only and safe. The next request transparently reloads.

### Backends

A single `modeld` binary is built for one (or none) of:

- `llama` (llama.cpp + GGUF)
- `openvino` (OpenVINO GenAI)

Selection happens at build time via tags, with runtime preference for accelerated backends when multiple are compiled in.

## Basic operation

See the [Quickstart](/docs/guide/quickstart/) for the common flow.

Typical commands:

```bash
contenox modeld install          # download + verify prebuilt daemon
contenox model pull qwen3-4b     # fetch a curated GGUF

# In another terminal:
modeld serve                     # or the exact command printed by install
```

Use `contenox doctor`, `contenox model local`, and the Beam "modeld console" (when running `contenox serve`) to inspect and control the resident model.

## Remote Modeld nodes

Register a modeld running elsewhere as a regular backend:

```bash
contenox backend add gpu-box --type modeld --url 100.64.0.5:9090
```

You can then:

- List models on that node
- Push artifacts to it (`contenox model push ... --backend gpu-box` or the Beam push panel)
- Use its models in chains exactly like local ones

The runtime talks to it over gRPC using the same transport contract. Capacity numbers, session residency, etc. all come from the remote daemon.

**Security note**: modeld has no built-in authentication. Only bind it on trusted networks (Tailscale, WireGuard, VPC, etc.) or localhost.

## Related reading

- [Local Models (GGUF)](/docs/integrations/providers/local-models/) — pulling and using GGUF models
- [modeld Architecture](/docs/integrations/providers/modeld-architecture/) — leases, capacity planner, slot model, residency
- [CLI reference — modeld](/docs/reference/contenox-cli/#contenox-modeld)
- Development:
  - [modeld Local Inference Landscape](/docs/development/modeld-local-inference-landscape/)
  - [Source build guide](/docs/development/modeld-source-build/)
  - [Release runbook](/docs/development/modeld-release-runbook/)
  - Deep blueprints under `development/blueprints/modeld/`

## Status

modeld is the production path for local (and remote-local) inference in Contenox. It is intentionally narrow in scope compared to general-purpose servers: it optimizes for long-lived, stateful coding/agent sessions on a single workstation accelerator.
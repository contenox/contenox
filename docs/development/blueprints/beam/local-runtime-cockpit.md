# Blueprint: Beam local runtime cockpit for modeld

## Scope

Beam should become the local runtime cockpit for Contenox, but it should not
talk to `modeld` directly.

The intended shape is:

```text
Beam
  -> contenox serve /api/modeld/*
  -> runtime/modelrepo/modeldconn + model registry + setup/config state
  -> modeld
```

This keeps browser/UI concerns, auth, filesystem safety, config, model registry
resolution, and product policy inside `contenox serve`. `modeld` remains focused
on resident model execution, hardware probing, capacity planning, slot state,
and KV/session behavior.

That boundary fits the current runtime shape: `contenox serve` already exposes
the product API under `/api` and serves Beam at `/`.

## Product questions

Beam should answer the questions users actually have:

- Is local inference available?
- Why did this model fit or fail?
- Why is my context limited to this number?
- What is loaded right now?
- What is hot in VRAM versus cold in host RAM?
- Can I unload or switch safely?
- Which local model variant am I really using?

The goal is not a generic admin UI or a browser wrapper around CLI commands.
Beam is the human-facing control surface for the local AI node. `modeld` is the
machine-facing execution engine.

## Surface 1: modeld status and control

Beam should show daemon state, lease/owner state, backend mode, active slot
state, active model, generation, busy operation, and last error.

The runtime already has real concepts for this:

- `runtime/internal/modeldprobe`: installed/running/stale/unreachable state,
  owner lease, endpoint, instance, backend.
- `runtime/modelrepo/modeldconn`: pure-Go seam to `modeld`.
- `runtime/transport.DaemonStatus`: owner, backend, slot state, active model,
  busy operation, last error.
- `runtime/transport.ActiveModel`: logical model identity, runtime config, slot
  generation.

The first conservative implementation should be read-only:

```text
GET /api/modeld/status
```

The response should expose logical active model identity and slot state, but not
arbitrary daemon-local filesystem paths. Browser-supplied paths should not enter
this surface.

Safe follow-up control endpoints:

```text
POST /api/modeld/unload
POST /api/modeld/load
```

`unload` should use generation fencing. `load` should only accept registered
model identities resolved server-side by Contenox, not arbitrary browser paths.

## Surface 2: hardware and capacity explanation

This is the most important UX win. `modeld` knows details hidden by the normal
provider abstraction:

- device memory total/free
- selected runtime/backend identity
- runtime digest/system info
- offload support
- model maximum context
- effective context
- memory-fit context
- hot context
- planner effective context
- KV bytes per token
- resolved GPU layers
- capacity clamp reasons
- sparse attention / SWA support
- host-cold budget

Beam should turn this into fit diagnostics:

- why a model got 8k context instead of 32k
- why GPU offload did or did not happen
- which memory pool was used
- which budget clamped the model
- what the user can change safely

The API should be registry-backed:

```text
GET /api/modeld/models
GET /api/modeld/capacity?model=<registered-model>
```

The server should resolve `<registered-model>` to a typed `modeldconn.ModelRef`
and call `modeldconn.Describe`. Capacity diagnostics should not require Beam to
know local paths.

## Surface 3: residency and usage observability

The long-context story is not only bigger context. It is hot/cold KV planning,
resident tokens, prefix/suffix reuse, cold-store blocks, pinned task context,
volatile context, and eviction/restore behavior.

Beam should eventually render a token residency timeline:

- hot VRAM segments
- cold host-RAM segments
- task-pinned blocks
- repo-map blocks
- volatile blocks
- reused prefix
- evicted blocks
- restored blocks
- cold hit/miss counts once modeld exposes them

Relevant existing concepts:

- `transport.ContextReport`
- `transport.ResidencyReport`
- resident tokens
- prefix/suffix tokens
- hot context
- planner effective context
- backend residency capability reporting

Open design issue: `ExplainContext` is currently session-scoped. Beam needs
either the runtime to surface the latest session report for the active local
provider path, or `modeld` to expose active session telemetry.

Future endpoint:

```text
GET /api/modeld/residency
```

This should be read-only and should describe current residency. Decode/session
control should stay on the normal chat/task execution path.

## Surface 4: model variants, not raw LoRA controls

The LoRA/adapters UI should not expose "pick random adapter file and scale" as
the primary product concept.

The product abstraction should be:

```text
variant = base model + adapter(s) + runtime profile
```

Beam can manage:

- base models
- adapters
- variants
- adapter provenance
- digest validation
- backend compatibility
- memory/context impact
- active loaded variant

Primary UI language should be "Local custom model" or "Model variant".
"LoRA" and "adapter" can appear as advanced detail.

Cache identity matters. Adapter digest, order, and scale must be part of
session and residency identity because warm KV reuse across different adapters
is invalid.

Future endpoints:

```text
GET  /api/modeld/adapters
POST /api/modeld/adapters/import
GET  /api/modeld/variants
POST /api/modeld/variants
POST /api/modeld/variants/{name}/validate
POST /api/modeld/variants/{name}/load
```

The first variant UI should be conservative: read-only registry and validation
before import/create controls.

## First implementation slice

1. Add read-only `GET /api/modeld/status`.
2. Show it in Beam under Backends -> Local runtime.
3. Omit daemon-local active model paths from the browser response.
4. Keep chat/task execution on the normal model-provider path.

Next slices:

1. Add capacity/fit diagnostics backed by registered model identities.
2. Add safe unload with generation fencing.
3. Add model load through registered model identities.
4. Add session/residency observability.
5. Add variant registry and adapter validation.
6. Add adapter import and variant creation after adapter identity is wired into
   transport/session/cache identity.

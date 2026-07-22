# modeld Blueprints

modeld is the native inference daemon behind the pure-Go `contenox` CLI. It
exists because one user is never one process: VS Code windows, ACP clients,
CLI sessions, and background commands all point at the same machine, the same
data directory, and often the same workspace. Local inference cannot survive
each of them owning the GPU independently.

The runtime's ownership model:

```text
Multiple frontends are allowed.
One per-user local runtime owner controls resident model sessions and live KV state.
Workspace mutations are serialized by lease.
Model artifacts are immutable once published.
```

modeld is that owner: started on demand by the first client, attached to by
every frontend over local IPC, holding the single active model slot, hardware
probing, capacity planning, and KV/session state behind the
`runtime/transport` session contract. It serves the llama.cpp and OpenVINO
backends and versions independently of the runtime.

## Coordination and ownership

| Doc | Status | What it covers |
| --- | --- | --- |
| [multi-client-coordination.md](multi-client-coordination.md) | decision | The founding decision above: option analysis, invariants, decision matrix, acceptance tests |
| [owner-coordination.md](owner-coordination.md) | decision | How the owner is elected, reached, and recovered across Linux/macOS/Windows |

## Daemon architecture and lifecycle

| Doc | Status | What it covers |
| --- | --- | --- |
| [interface-boundary.md](interface-boundary.md) | decision | State vs. compute: why modeld exposes a stateful session boundary, not the stateless provider interface |
| [single-active-model-slot.md](single-active-model-slot.md) | decision | One resident model per daemon; slot lifecycle, generations, eviction |
| [provisioning-detection.md](provisioning-detection.md) | decision | How the runtime discovers/installs modeld and fails honestly when it is absent |
| [version-decoupling.md](version-decoupling.md) | architecture | modeld versions independently of the runtime; selection by protocol compatibility |
| [release-artifacts.md](release-artifacts.md) | packaging | Device-built native dep bundles and final packages in the artifact store; the `contenox setup` download/verify/install flow's version-selection model is defined by [version-decoupling.md](version-decoupling.md) |

## Runtime strategy

| Doc | Status | What it covers |
| --- | --- | --- |
| [effective-context/](effective-context/README.md) | strategy | Long effective context on one consumer accelerator: north star, architecture, parity contract |
| [no-spill-placement.md](no-spill-placement.md) | design | Refuse, don't degrade: no silent spill to hybrid CPU inference when a model does not fit the device budget |
| [lora-adapters.md](lora-adapters.md) | R&D | LoRA adapters as "model variants"; identity, cache keying, native attach |

## Vision (image input)

The llama backend serves vision models (VLMs) natively through llama.cpp's
multimodal stack (libmtmd). The design follows the same capability-truth and
refuse-don't-spill rules as text serving:

- **Two-file layout.** A vision model is `model.gguf` plus its multimodal
  projector `mmproj.gguf` in the same model directory. The projector is
  resolved from the model path — no configuration knob — and a model without
  one is simply a text model. Pushing a VLM to a node uses the tar push format
  so both files install atomically; the model's cache identity stays the
  `model.gguf` content digest.
- **Memory model.** Projector weights load whole (no per-layer offload), so
  capacity planning budgets them plus a vision-encoder compute reserve as
  fixed overhead on the serving device — they shrink the KV window instead of
  spilling. After encoding, each image occupies real sequence positions in the
  KV cache; `Describe` reports a conservative per-image token estimate from
  the projector metadata for planners.
- **Refusal behavior.** Images attach to the volatile suffix; the stable
  prefix stays text-only (prefix reuse is keyed on a token-only tape). An
  image-bearing prefill that does not fit the hot window is refused with the
  typed context-overflow error rather than streamed or degraded, and a prompt
  whose media markers do not match its attached images fails loudly. Models
  without a projector refuse image input as an unsupported feature.
- **Capability truth.** `Describe` sets `SupportsVision` only when a projector
  is resolved *and* its own metadata declares image input — never inferred
  from the model name. The OpenVINO backend does not serve vision; it reports
  that honestly (see
  [effective-context/modeld-capability-truth-blueprint.md](effective-context/modeld-capability-truth-blueprint.md)).

## Backends

| Doc | Status | What it covers |
| --- | --- | --- |
| [llama/binding-ownership-options.md](llama/binding-ownership-options.md) | decision record | Contenox-owned direct llama.cpp shim over third-party bindings |

Contributor build/release docs live in [`docs/development/`](../../modeld-source-build.md) (`modeld-source-build.md`,
`modeld-release-runbook.md`, `modeld-llama-backend.md`).

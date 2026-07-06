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
| [release-artifacts.md](release-artifacts.md) | packaging | Device-built native dep bundles and final packages in the artifact store |
| [setup-artifact-detection.md](setup-artifact-detection.md) | design | `contenox setup` download/verify/install flow; version-selection model updated by [version-decoupling.md](version-decoupling.md) |

## Runtime strategy

| Doc | Status | What it covers |
| --- | --- | --- |
| [effective-context/](effective-context/README.md) | strategy | Long effective context on one consumer accelerator: north star, architecture, parity contract, bench findings |
| [coldstore-sizing-plan.md](coldstore-sizing-plan.md) | plan | Derived hot/cold KV sizing and the host-RAM cold store |
| [lora-adapters.md](lora-adapters.md) | R&D | LoRA adapters as "model variants"; identity, cache keying, native attach |
| [speculative-execution.md](speculative-execution.md) | R&D / vision | Guess-ahead decode strategies for modeld |

## Backends

| Doc | Status | What it covers |
| --- | --- | --- |
| [llama/binding-ownership-options.md](llama/binding-ownership-options.md) | decision record | Contenox-owned direct llama.cpp shim over third-party bindings |
| [llama/coding-node-plan.md](llama/coding-node-plan.md) | graduation plan | The local coding node on llama.cpp |
| [llama/plumbing-log.md](llama/plumbing-log.md) | log | Implementation record of the llama provider plumbing |
| [openvino/coding-node-plan.md](openvino/coding-node-plan.md) | research plan | The local coding node on OpenVINO; proven out by the S-series logs |
| [openvino/plumbing-handover.md](openvino/plumbing-handover.md) | handover | Locked decisions and state of the OpenVINO provider plumbing |
| [openvino/s1-embedded-controls.md](openvino/s1-embedded-controls.md) | log | S1: embedded pipeline controls |
| [openvino/s1-5-genai-provider.md](openvino/s1-5-genai-provider.md) | log | S1.5: GenAI provider |
| [openvino/s2-prefix-reuse.md](openvino/s2-prefix-reuse.md) | log | S2: prefix-cache reuse proof |
| [openvino/s2-7-protocol-registry.md](openvino/s2-7-protocol-registry.md) | log | S2.7: parser protocol registry |
| [tensorrt-llm-backend.md](tensorrt-llm-backend.md) | R&D / vision | TensorRT-LLM against the modeld boundary |
| [ortgenai-windows-ai.md](ortgenai-windows-ai.md) | spike | ORT GenAI / Windows ML as an AI PC backend track; can it express the modeld session shape |
| [alternative-silicon.md](alternative-silicon.md) | strategy | AI PC / alternative-silicon runtime strategy across candidate backends (NPU, QNN, Ryzen AI, RKLLM) |

Contributor build/release docs live in [`docs/development/`](../../development/) (`modeld-source-build.md`,
`modeld-release-runbook.md`, `modeld-llama-backend.md`).

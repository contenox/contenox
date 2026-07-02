# modeld Local Inference Landscape

Snapshot date: 2026-07-02

This note is a high-level landscape overview for `modeld`, not a dependency
review. It compares `modeld` with public local-model runners, desktop AI
products, and production inference stacks that a user or maintainer might
naturally compare it against.

Related technical blueprint:
- [modeld Local Inference Cross-Compare](blueprints/effective-context-kv/modeld-local-inference-cross-compare-blueprint.md)

## Short Position

`modeld` is best described as a local coding-agent memory/runtime daemon:

- one local owner
- one active local model
- many persistent sessions
- resident coding context on a workstation accelerator

That is different from "run any model behind an OpenAI API." `modeld` is shaped
around making a single local model useful for long, stateful coding work, where
stable context should stay resident instead of being resent every turn.

## Current Boundary

Today, `modeld` is the native local inference daemon for Contenox. It serves the
runtime transport contract over gRPC, owns a per-data-root lease, and hosts local
llama.cpp/GGUF and OpenVINO backends while the `contenox` CLI remains pure Go.

The north-star direction goes further: persistent, branchable, warm session
state and effective context beyond the practical native prompt window on limited
local hardware. Treat those as product direction unless the specific release,
backend, model, and device have been certified.

## What modeld Is Not

- It is not a general desktop chat application.
- It is not primarily a model marketplace or model-library UX.
- It is not a broad multimodal gateway.
- It is not a multi-tenant production serving stack.
- It is not trying to beat every backend engine at raw throughput.

Those are valid product shapes; they are just not the core `modeld` bet.

## Comparison Map

| Project family | Examples | Primary shape | modeld difference |
| --- | --- | --- | --- |
| Local model runners | Ollama | Model pull/run UX, local daemon, local HTTP API | `modeld` is less model-marketplace-oriented and more session/residency-oriented for Contenox coding workflows. |
| Desktop local AI products | Jan, LM Studio, Cortex.cpp | App UX, local chat, model selection, local API | `modeld` is the lower-level resident inference layer, not the full desktop product. |
| Local power-user servers | KoboldCpp, textgen-webui | One-machine UI/server with many knobs, modes, APIs, and model formats | `modeld` aims for accelerator-derived policy and fewer user-facing runtime knobs. |
| Self-hosted API gateways | LocalAI | OpenAI-compatible gateway across many models, modalities, users, and backends | `modeld` deliberately narrows scope to local LLM backends needed by Contenox. |
| High-throughput serving engines | vLLM, TGI, SGLang | Throughput, batching, model coverage, distributed or server-side APIs | `modeld` optimizes a single workstation's long-lived coding context, not fleet throughput. |
| Inference orchestration | llm-d, llm-d-router | Kubernetes routing, KV-aware scheduling, autoscaling, multi-tenant traffic | `modeld` shares cache-awareness themes but targets one owner on one machine. |
| Coding assistant products | Tabby | Self-hosted coding assistant, IDE integration, code indexing, team/admin surfaces | `modeld` is only the local inference/memory layer under Contenox, not the whole assistant product. |
| Vendor model servers | OpenVINO Model Server, Triton-class servers | General model serving via REST/gRPC and vendor runtime integration | `modeld` may use vendor runtimes, but wraps them in Contenox ownership, session, and residency semantics. |

## The Real Differentiator

The strongest differentiator is not "local inference." Many projects already do
that well.

The differentiator is durable, reusable coding context:

- stable prefixes should be paid for once, then reused
- sessions should survive beyond a single request
- state should be snapshot-capable and branchable
- accelerator memory should favor one deep local working set over many shallow
  concurrent users
- context policy should be selected from measured device capability rather than
  exposed as a pile of manual knobs

In production serving systems, prefix/KV reuse is usually a throughput feature.
In `modeld`, it is product behavior: the local coding agent should feel like it
remembers the working set because the runtime actually keeps the working set
resident when the backend and device can support it.

## Closest Neighbors

The closest product neighbor is Ollama because it owns a local daemon and a
simple model lifecycle. The difference is that Ollama's center of gravity is
download, run, and expose models broadly, while `modeld`'s center of gravity is
resident context for Contenox sessions.

The closest architecture neighbors are SGLang and llm-d because they put serious
attention on prefix/KV-cache behavior. The difference is deployment target and
product contract: they are serving infrastructure for throughput and scale;
`modeld` is a single-user workstation daemon for coding continuity.

Tabby is the closest user-problem neighbor for self-hosted coding assistance.
The difference is product layer: Tabby is an assistant/server product, while
`modeld` is the local model runtime layer below Contenox.

## Positioning Language

Use:

- "local coding-agent memory/runtime daemon"
- "resident workstation context"
- "one owner, one active model, many persistent sessions"
- "effective context through warm session reuse"
- "backend-neutral session contract over certified local runtime cells"

Avoid:

- "Ollama alternative" without qualification
- "OpenAI-compatible local server" as the headline
- "multi-user serving stack"
- "supports every model"
- "infinite context" or any context claim not backed by a certified model/device
  profile

## Practical Takeaway

If a user wants a simple local model runner, Ollama or LM Studio may be the
cleaner comparison. If an operator wants high-throughput serving, vLLM, SGLang,
TGI, or llm-d are the right comparison set.

`modeld` matters when the goal is different: make local coding-agent work keep
state on a personal machine, reuse expensive context, and let Contenox route
routine tokens to local/private hardware by default.

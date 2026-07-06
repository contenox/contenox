# Blueprint: Session aac21f41 Learning Map

Status: fact map
Owner: runtime / modeld
Source: Claude resume session `aac21f41-5492-42e8-8ef9-a721040dd0a5`

Purpose: map the resumed Claude session into the current effective-context work without
carrying invalid benchmark claims forward.

## Session Source

Local transcript:

```text
/home/naro/.claude/projects/-home-naro-src-github-com-contenox-enterprise-runtime/aac21f41-5492-42e8-8ef9-a721040dd0a5.jsonl
```

Task files:

| Task | Subject | Session status | Current read |
|---|---|---:|---|
| 1 | OpenVINO device autodetect + priority selection | completed | Device ordering and OpenSession fallback are represented in the current OpenVINO code. |
| 2 | NPU viability spike + per-device capability probe | completed | NPU is not a certified ContinuousBatching/PagedAttention target. |
| 3 | Push SWA into residency policy | pending in task file | Current code has SWA metadata and SWA-aware capacity/residency paths; keep tests as the source of truth. |

## Facts To Carry Forward

| Area | Fact | Blueprint consequence |
|---|---|---|
| Capacity terms | `EffectiveContext` is the dense served window and cache identity. `MemoryContextTokens` is raw fit. `HotContextTokens` is physical hot KV. `PlannerEffectiveContext` is the logical planner window when cold/host budget exists. | UI, API, and docs must not collapse these into one "context" value. |
| Fit versus serving | Numeric GGUF or IR metadata can be parsed even when the runtime cannot load or serve the model. | `Describe` and `ModelInfo` must be capability-truthful, not metadata-only. |
| llama.cpp pin | The session found a pinned llama.cpp commit that lacked `gemma4`; the pin was later moved to `86b94708...`, which contains `LLM_ARCH_GEMMA4`. | Dependency pin bumps are runtime integration changes and must be smoke-tested through build, package, and load. |
| Load errors | The native loader reason can be more useful than the wrapped error. | Preserve architecture/load diagnostics at the modeld boundary. |
| OpenVINO text path | The certified text path is OpenVINO GenAI `ContinuousBatchingPipeline`. | VLM repos are not text effective-context targets unless a VLM adapter is explicitly implemented and certified. |
| NPU | Intel NPU enumerates as a device, but cannot run the CB/PagedAttention path used here. | AUTO excludes NPU; explicit NPU pins fail with an actionable unsupported-feature error. |
| Session state | OpenVINO CB is request based. The Go adapter owns the transport token tape and resubmits sequence state; vendor prefix cache is physical reuse, not a direct exposed KV handle. | Snapshot, restore, prefix identity, and cold-KV semantics stay modeld-owned unless a backend exposes strict equivalents. |
| Sparse/XAttention | Arc iGPU driver stacks can reject XAttention. | Automatic sparse can retry dense; explicit sparse failure remains a hard certification failure. |
| Scheduler cache | Too-small CB block pools can poison a pipeline; oversized pools can thrash shared-memory iGPUs. | Cache sizing is part of the certified profile, not a free tuning knob. |
| Windows launcher | Direct `modeld.exe` on Windows can fail with `0xC0000135` when DLL paths are not set. `modeld.cmd` sets `PATH`. | Windows benchmark tooling must launch the packaged modeld launcher, not the bare executable. |
| Windows app control | The session saw CodeIntegrity/WDAC blocking `contenox.exe`; after the user disabled Smart App Control, `contenox version v0.32.8` ran. | Benchmark preflight records app-control state and treats blocks as environment failures, not modeld performance data. |
| Raw OpenVINO probes | Python `openvino_genai` rows bypassed `contenox`, routing, transport, profiles, traces, and session bookkeeping. | Raw rows are substrate controls only. Product claims require the `contenox` path. |
| TinyLlama context | TinyLlama raw probes can accept prompts beyond the runtime-advertised trained ceiling. | Runtime context claims stay at the certified model/profile ceiling. |
| Quality | TinyLlama can produce throughput while echoing or continuing the prompt. | Throughput without answer-quality smoke is not a certified row. |

## Current Work Mapping

| Learning | Current blueprint |
|---|---|
| Certified context is latency-budgeted, not just memory-fit. | [Latency-budgeted effective context](hardware-effective-context-blueprint.md) |
| Runtime cells can be vendor runtimes, modeld-native kernels, or compatibility backends. | [Specialization cells](specialization-cells-blueprint.md) |
| OpenVINO needs hard routing, device, scheduler, and benchmark gates. | [OpenVINO hardening](openvino-hardening-blueprint.md) |
| Raw backend data must not be headline product data. | [Benchmark integrity](benchmark-integrity-blueprint.md) |
| `Describe` must not advertise impossible models/devices. | [modeld capability truth](modeld-capability-truth-blueprint.md) |

## Do Not Carry Forward

- Do not present raw Python OpenVINO rows as `contenox` results.
- Do not certify `gemma4-e4b-ov` as a text CB model; it is a VLM repo unless a VLM cell exists.
- Do not treat NPU enumeration as NPU support for the effective-context text path.
- Do not advertise context above trained/certified model ceilings because a raw backend run accepted input.
- Do not treat VRAM capacity as effective context without TTFT, TPOT, wall time, quality, and stability.

## Remaining Work

- Convert every OpenVINO hardware/model claim into a benchmark row with product-path provenance.
- Add or verify capability-truth checks for architecture support, text/VLM pipeline compatibility, and explicit device support.
- Preserve native loader diagnostics in user-facing modeld errors.
- Make Windows benchmark packaging reproducible and launcher-aware.
- Keep raw substrate scripts as controls, with labels that cannot be confused with product results.

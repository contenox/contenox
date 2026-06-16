# Plan: Alternative Silicon & AI PC Runtime Strategy

> **Status:** expert product-strategy blueprint, corrected against primary docs
> on 2026-06-16.
> **Sibling docs:** `local-coding-node-goals.md`, `plan-openvino.md`,
> `plan-llamacpp.md`, `plan-ortgenai-windows-ai.md`.

## Executive Verdict

Do not position Contenox as the "OS-level runtime" replacing Windows ML,
OpenVINO, QNN, Ryzen AI, RKLLM, or llama.cpp. That claim is too broad and picks
the wrong fight.

The defensible claim is:

```text
Contenox is the workspace-state runtime above fragmented local accelerators.
```

Hardware vendors and OS/runtime teams own kernels, drivers, graph compilation,
execution providers, and model packaging. Contenox owns what they do not know:

```text
deterministic workspace context
repo maps, pinned files, diffs, logs, tests, tools
stable/volatile segment manifests
token hashes and invalidation rules
session lifecycle and warm-reuse policy
coding workflow UX
benchmark-backed device/model profiles
```

The strategy is to make local silicon useful for developer workflows by turning a
workspace into reusable execution state, not by pretending Contenox replaces the
substrates. There is no sidecar product strategy in this plan: a runtime profile
is either a standalone Contenox llama profile that can run the main session
contract, or it is out of scope.

## Verified Corrections

### Windows ML / ORT GenAI Is A Real Track

Windows ML is now positioned by Microsoft as a local Windows inference framework
powered by ONNX Runtime, with Windows-managed execution providers for NPUs, GPUs,
and CPUs. It supports x64 and ARM64 Windows PCs and can dynamically acquire EPs
instead of every app bundling them.

ONNX Runtime GenAI also exposes generator operations that map to Contenox's workspace-context
shape:

```text
OgaGenerator::AppendTokenSequences
OgaGenerator::AppendTokens
OgaGenerator::GenerateNextToken
OgaGenerator::RewindTo
OgaGenerator::GetSequenceCount
OgaGenerator::GetSequenceData
```

This is enough to justify a first-class AI PC compatibility track:

```text
plan-ortgenai-windows-ai.md
```

Important caveat: provider support is version- and EP-dependent. ORT's migration
docs say chat/system-prompt caching support is limited for some providers, while
AMD's Ryzen AI 1.7.1 article says ONNX Runtime GenAI continuous decoding supports
KV reuse on Ryzen AI OGA configurations. Treat this as a required probe, not a
guarantee.

### NPUs Are Not The First 7B/64k Target

Do not make this the first Snapdragon/Ryzen NPU success metric:

```text
7B/8B coding model
64k hot context
<60s warm TTFT
Hexagon/XDNA NPU only
```

Near-term public material points to smaller NPU workloads. ORT's Snapdragon
tutorial currently centers on SLMs such as Phi-3.5 mini instruct and Llama 3.2
3B, with Snapdragon-specific assets. AMD's Ryzen AI 1.7 highlights LLM context
support up to 16K on NPU. That is useful, but it is not the T1/T2 64k-128k main
coder target.

Treat 2026 AI PC NPUs as:

```text
1. small-model engines: 1.5B-4B, 4k-16k context
2. future main-coder candidates only after measured proof
3. unsupported for Contenox if they only work as helper sidecars
```

The 7B/8B 64k-128k coding-node target remains first for Intel Arc/Arc Pro,
mature GPU paths, high-memory APUs, and proven OpenVINO/llama.cpp profiles.

### llama.cpp Is A Broad Bridge, Not Free Hardware Support

llama.cpp remains valuable because it carries GGUF, CPU fallback, Vulkan/SYCL,
and backend experiments such as Snapdragon CPU/OpenCL/Hexagon builds. But "support
for free" is too strong.

Every llama.cpp backend still must prove:

```text
sequence/state correctness
warm suffix equals cold full prompt
KV memory placement
context length
KV quant behavior
cancellation and error recovery
packaging on the target OS/arch
```

### Rockchip Is A Constrained Standalone Lane

RKLLM/RKNN-class hardware is useful for edge demos, small helpers, offline
automation, summaries, and local indexing in other products. For Contenox, it is
only interesting if it can run a standalone `LocalSession` profile. It is not a
sidecar lane and not the flagship 7B/64k local coding node.

### Jetson Is A CUDA Edge Lane, Not The Main Budget Node

Jetson Orin NX 16GB is worth tracking even though it is "NVIDIA-shaped." NVIDIA
positions Jetson Orin NX as a compact edge module with up to 157 TOPS, 10W-40W
power modes, an Ampere GPU, and 16GB/8GB LPDDR5 options. That makes it a serious
edge inference node, but Contenox should only care about it as a standalone
llama profile.

Do not confuse that with the main Contenox coding-node target:

```text
Jetson Orin NX 16GB:
  good standalone edge-node / small-coder candidate
  likely useful for 7B-class Q4 daily assistants at modest context
  constrained by shared 16GB system memory and model residency

Contenox T1/T2:
  7B/8B coder
  64k-128k hot context
  warm suffix equivalence
  no swap path during decode
```

The lesson from the Pi 5 + Jetson Orin NX NOUS field report is operational:
unified memory gets painful when multiple large models stay resident. A large
infrequent model should be on-demand, not permanently warmed, if it pushes the
daily model into swap. The Pi+Jetson split is a useful field report, but
Contenox should not build a required multi-node sidecar architecture from it.

Second lesson: local "OpenAI-compatible" tool-call endpoints are not enough by
themselves. Tool-call reliability must be certified per backend/model/template
protocol. If a local serving layer cannot reliably emit declared tool calls,
prompt-injected RAG is a valid product fallback, but the profile gets no
tool-call certification.

## Runtime Lanes

### 1. Intel / OpenVINO Certified Node

Flagship path for the main coding-node target:

```text
7B/8B coder
64k-128k hot context
200k+ effective context through retrieval/pins/summaries
Arc/Arc Pro GPU first, NPU/AMX as measured
```

OpenVINO remains the strongest narrow stack for the budget Intel node.

### 2. llama.cpp Llama Runtime

Fastest cross-backend proof path:

```text
GGUF model ecosystem
GPU fallback and broad local hardware support
sequence/KV primitives
tiny correctness fixtures
cross-substrate validation of the Contenox manifest/session layer
```

Treat each llama.cpp backend as a profile to certify, not an automatic product
claim.

### 3. ONNX Runtime GenAI / Windows ML

AI PC compatibility lane:

```text
Windows x64
Windows ARM64
Qualcomm QNN EP
AMD Ryzen AI / Vitis AI EP
DirectML / GPU
CPU fallback
Foundry Local / Windows ML distribution alignment
```

This track is not a replacement for OpenVINO or llama.cpp. It exists because AI
PCs are Windows-heavy, and Windows ML/ORT is becoming the system-supported
distribution layer.

### 4. Direct Vendor SDKs

Escalate only after a bridge runtime fails a measured Contenox requirement:

```text
runtime/modelrepo/qualcomm/qnnsession
runtime/modelrepo/amd/ryzenaisession
runtime/modelrepo/rockchip/rkllmsession
```

Reasons to escalate:

```text
missing append/rewind/remove-tail
bad memory placement
no metrics
bad cancellation semantics
too much bridge overhead
unacceptable model conversion friction
```

### 5. NVIDIA Jetson / CUDA Edge Nodes

Jetson belongs in this strategy as an edge-node lane:

```text
runtime routes:
  llama.cpp CUDA / GGUF
  Ollama as compatibility baseline only
  TensorRT-Edge-LLM or TensorRT paths when they expose enough session control
  JetPack/CUDA ecosystem only if it runs inside the standalone profile

recommended role:
  standalone edge assistant at measured context
  small-coder or 7B-class daily assistant if the profile passes

not assumed:
  64k hot coding context
  multiple resident large models
  reliable OpenAI-compatible tool calls through every serving layer
```

The runtime policy requirement is stronger than on a desktop dGPU: Contenox must
know which models are resident, which can be evicted, which are on-demand, and
whether swap/zram pressure has appeared. A Jetson profile that swaps during
normal daily use is not certified for that workload even if it eventually
answers.

## Capability Contract

Product code must ask backends what they can do. It must not infer capabilities
from branding such as "AI PC", "Hexagon", "XDNA", or "NPU".

```go
type RuntimeCapability struct {
    Backend              string // openvino, llamacpp, ortgenai, qnn, ryzenai, rkllm
    OS                   string // linux, windows, darwin, android
    Arch                 string // amd64, arm64
    DeviceClass          string // cpu, gpu, npu, hybrid
    ModelFormat          string // gguf, openvino-ir, onnx-genai, qnn-context, rkllm

    CanGenerate          bool
    CanStream            bool
    CanTokenize          bool
    CanReportMetrics     bool
    CanCancelPrefill     bool
    CanCancelDecode      bool

    HasPersistentSession bool
    CanAppendTokens      bool
    CanRewindToToken     bool
    CanRemoveTail        bool
    CanCopySequence      bool
    CanSnapshotKV        bool
    CanRestoreKV         bool
    CanBranchSession     bool

    ReportsKVBytes       bool
    ReportsDeviceMemory  bool
    ReportsHostMemory    bool
    ReportsSwapPressure  bool
    SupportsKVQuant      bool
    SupportsPrefixCache  bool
    SupportsSparseAttn   bool
    SupportsModelUnload  bool
    SupportsIdleEvict    bool

    MaxTestedContext     int
    MaxTestedHotPrefix   int
    MaxTestedSuffix      int
    BatchSize            int

    KnownLimitations     []string
}
```

Planner rules:

```text
Exact suffix reuse:
  append + rewind/remove-tail, or equivalent sequence ops.

Coarse warm-prefix reuse:
  runtime prefix cache hit, suffix re-prefill.

Snapshot branch:
  only when save/restore is exact and measured.

Chat-only fallback:
  no workspace-context-reuse claim, no long-context tier claim.
```

## Certification Tiers

Do not reuse T1/T2/T3 for alternative silicon. Those are main coding-node goals.
Use A-tier certification for the hardware/runtime profile:

| Tier | Name | Requirement |
|---|---|---|
| A0 | Local chat baseline | model loads, generates, streams, reports basic metrics |
| A1 | Persistent generator | append tokens across turns without full prompt rebuild |
| A2 | Rewind/suffix equivalent | warm suffix output matches cold full prompt under greedy decode |
| A3 | Constrained standalone node | one model/profile runs locally with measured context, no required sidecar |
| A4 | Small main coder | 1.5B-4B coder/helper, 8k-16k hot context, useful coding workflows |
| A5 | Large main coder | 7B/8B, 64k hot, warm useful response target |
| A6 | Snapshot/branch | raw or logical session snapshot/restore, branch, resume |

Example public claims:

```text
Contenox Certified: Snapdragon X A4 Small-Coder
Contenox Certified: Ryzen AI 300 A4 Small-Coder
Contenox Certified: Intel Arc Pro A5 Main-Coder
Contenox Certified: Jetson Orin NX 16GB A3 Edge Assistant
Contenox Certified: RK3588 A3 Constrained Standalone Node
```

## Backend-Neutral Session Shape

```go
type TokenPosition int

type LocalSession interface {
    Profile() RuntimeProfile
    Capabilities() RuntimeCapability

    Tokenize(ctx context.Context, text string) ([]int32, error)
    AppendTokens(ctx context.Context, tokens []int32) (TokenPosition, error)
    Rewind(ctx context.Context, pos TokenPosition) error

    DecodeNext(ctx context.Context, opts DecodeOptions) (Token, error)
    Stream(ctx context.Context, opts DecodeOptions) (<-chan Token, <-chan error)

    Metrics() SessionMetrics
    DeviceMemory() (DeviceMemory, error)

    SnapshotSave(ctx context.Context, path string) error    // optional
    SnapshotRestore(ctx context.Context, path string) error // optional
    Branch(ctx context.Context) (LocalSession, error)       // optional

    Close() error
}
```

The existing `llama.Session` can evolve toward this shape without exposing
substrate details to product code.

## Model/Profile Strategy

Do not force one model format. Certify profiles.

| Format | Runtime | Use |
|---|---|---|
| GGUF | llama.cpp | broad local support and GPU fallback |
| OpenVINO IR | OpenVINO | Intel certified node and NPU/GPU/CPU profiles |
| ONNX GenAI folder | ORT GenAI / Windows ML | Windows AI PC track |
| QNN context binaries | ORT QNN / QNN direct | Snapdragon optimized deployment |
| RKLLM | RKLLM Runtime | constrained standalone edge node only |
| GGUF / TensorRT artifacts | Jetson CUDA / TensorRT-Edge-LLM | NVIDIA standalone edge node |

Example profile metadata:

```json
{
  "id": "qwen2.5-coder-3b-ortgenai-qnn-snapdragon-x",
  "model_family": "qwen2.5-coder",
  "format": "onnx-genai-qnn",
  "runtime": "ortgenai",
  "execution_provider": "QNNExecutionProvider",
  "os": "windows",
  "arch": "arm64",
  "device": "snapdragon-x-hexagon",
  "weights": "int4",
  "max_tested_context": 8192,
  "max_tested_hot_prefix": 6144,
  "supports_append": true,
  "supports_rewind": true,
  "supports_snapshot": false,
  "certification": "A4",
  "warnings": [
    "not a 7B/64k main-coder profile",
    "model assets are EP-specific",
    "rerun certification after QNN EP update"
  ]
}
```

## Benchmarks

Every adapter/profile should emit one JSON report:

```json
{
  "profile_id": "...",
  "hardware": "...",
  "runtime": "...",
  "runtime_version": "...",
  "driver_version": "...",
  "os": "...",
  "arch": "...",
  "cold_full_prefill": {
    "tokens": 8192,
    "ms": 0,
    "prompt_tps": 0,
    "ttft_ms": 0,
    "device_memory_peak_mb": 0
  },
  "warm_append": {
    "cached_tokens": 6144,
    "new_tokens": 512,
    "ttft_ms": 0,
    "equivalent_to_cold_greedy": true
  },
  "rewind_suffix": {
    "rewind_to": 6144,
    "new_suffix_tokens": 1024,
    "ttft_ms": 0,
    "equivalent_to_cold_greedy": true
  },
  "decode": {
    "tokens": 256,
    "tokens_per_second": 0
  },
  "snapshot": {
    "supported": false,
    "save_ms": null,
    "restore_ms": null,
    "bytes": null
  },
  "residency": {
    "resident_models": [],
    "on_demand_models": [],
    "model_unload_ms": 0,
    "host_memory_peak_mb": 0,
    "swap_events": 0
  },
  "failure_cases": {
    "over_context": "structured_error",
    "cancel_prefill": "session_valid_or_dead_explicit",
    "cancel_decode": "session_valid_or_dead_explicit",
    "profile_mismatch": "cache_miss"
  }
}
```

The most important correctness test remains:

```text
warm suffix output == cold full prompt output under greedy decoding
```

## Revised Phases

### AS0 — Capability Matrix

Add a probe command:

```sh
contenox silicon probe --json
```

Report:

```text
OS/arch
available CPUs/GPUs/NPUs
unified-memory devices and usable memory
swap/zram status
installed EPs/plugins/drivers
runtime candidates
model formats supported
session capabilities
model unload / idle eviction support
known unsupported paths
```

### AS1 — ORT GenAI Adapter Spike

Create `runtime/modelrepo/ortgenai/`.

Minimum proof:

```text
load ONNX GenAI model
select CPU / DirectML / QNN / AMD Vitis AI or Ryzen AI EP where available
append stable prompt
append user suffix
generate greedily
rewind to stable boundary
append alternate suffix
verify warm output equals cold full prompt
```

If a provider does not support append/rewind for chat mode, record that in
capabilities and downgrade the profile.

### AS2 — Windows ARM64 Native Build

Target Snapdragon X devices:

```text
GOOS=windows GOARCH=arm64
CGo/native DLL loading proven
SQLite/storage path proven
ORT GenAI/QNN DLL loading proven
streaming/cancel works
zip/installer packaging works
```

### AS3 — Edge Model Residency Gate

Required for Jetson-class unified-memory nodes and useful everywhere:

```text
declare always-warm daily model
declare secondary on-demand models
measure load/unload latency
evict idle large models before daily model swap
report resident model memory and swap/zram pressure
fail certification if decode depends on swapping
```

### AS4 — Small-Coder Certification

Certify 1.5B-4B models:

```text
4k, 8k, 16k contexts where supported
append/rewind correctness
warm TTFT
energy/thermal notes
useful coding tasks, not only chat
```

### AS5 — Main-Coder Attempt

Only after AS4 passes:

```text
attempt 7B/8B
attempt 32k, then 64k
measure memory and warm TTFT
promote to A5 only if it passes
```

### AS6 — Direct SDK Escalation

Use QNN/RyzenAI/RKLLM direct adapters only when bridge runtimes fail a measured
requirement.

## Risks

| Risk | Mitigation |
|---|---|
| TOPS marketing trap | publish profile benchmarks only |
| Hidden KV/state APIs | accept append/rewind as a logical primitive when exact |
| Provider drift | pin runtime + driver + EP version in profiles |
| Static-shape/context limits | certify max tested context only |
| Windows ARM64 CGo friction | AS2 packaging spike before product promise |
| Model conversion friction | reproducible model-profile recipes |
| No raw snapshot | keep snapshots optional; prefer live sessions first |
| Unified-memory contention | model residency policy, idle eviction, no-swap certification |
| Local tool-call protocol drift | certify parser/tool protocol per model/runtime; prompt-injected RAG fallback gets no tool-call claim |
| Vendor lock-in | backend-neutral session interface and capability matrix |

## Vendor Ask

Ask vendors for:

```text
sample devices
redistributable runtime terms
stable C/C++ APIs
append/rewind or raw KV primitives
device memory reporting
prefill/decode metrics
cancellation semantics
model conversion recipes for Qwen/Phi/coder models
permission to publish benchmark-backed Contenox profiles
```

Demo target:

```sh
contenox silicon certify --profile snapdragon-x-qnn-qwen3-4b
contenox init
contenox node explain-context
contenox "find why this test fails"
```

## Final Strategy

```text
Intel/OpenVINO:
  flagship certified main coding node.

llama.cpp:
  fast GGUF bridge and cross-backend validation.

ORT GenAI / Windows ML:
  AI PC compatibility lane, especially Windows ARM64, Qualcomm, AMD, DirectML.

Direct SDKs:
  escalation path when bridge layers fail.

Jetson / CUDA edge:
  standalone edge assistant only;
  model residency and no-swap behavior are certification gates.

NPUs:
  small-coder engines first;
  main 7B/64k only after benchmark proof.
```

## References

- Microsoft Windows ML overview: https://learn.microsoft.com/en-us/windows/ai/new-windows-ml/overview
- ONNX Runtime GenAI C++ API: https://onnxruntime.ai/docs/genai/api/cpp.html
- ONNX Runtime GenAI migration / continuous decoding: https://onnxruntime.ai/docs/genai/howto/migrate.html
- ONNX Runtime GenAI Snapdragon tutorial: https://onnxruntime.ai/docs/genai/tutorials/snapdragon.html
- ONNX Runtime QNN Execution Provider: https://onnxruntime.ai/docs/execution-providers/QNN-ExecutionProvider.html
- ONNX Runtime Vitis AI Execution Provider: https://onnxruntime.ai/docs/execution-providers/Vitis-AI-ExecutionProvider.html
- AMD Ryzen AI Software: https://www.amd.com/en/developer/resources/ryzen-ai-software.html
- AMD KV cache reuse on Ryzen AI / ORT GenAI: https://www.amd.com/en/developer/resources/technical-articles/2026/accelerating-local-llm-conversations-with-kv-cache-reuse-on-amd-.html
- llama.cpp Snapdragon backend docs: https://github.com/ggml-org/llama.cpp/blob/master/docs/backend/snapdragon/README.md
- llama.cpp SYCL backend docs: https://github.com/ggml-org/llama.cpp/blob/master/docs/backend/SYCL.md
- Rockchip RKLLM repository: https://github.com/airockchip/rknn-llm
- NVIDIA Jetson Orin specifications: https://www.nvidia.com/en-us/autonomous-machines/embedded-systems/jetson-orin/
- NVIDIA Jetson memory optimization: https://developer.nvidia.com/blog/maximizing-memory-efficiency-to-run-bigger-models-on-nvidia-jetson/
- NOUS Pi 5 + Jetson Orin NX field repo: https://github.com/Discod73/nous-core

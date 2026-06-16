# Plan: ORT GenAI / Windows ML AI PC Runtime Track

> **Status:** spike blueprint, researched against primary docs on 2026-06-16.
> **Sibling docs:** `local-coding-node-goals.md`, `plan-openvino.md`,
> `plan-llamacpp.md`, `plan-alternative-silicon.md`.

## Purpose

This track exists because AI PCs are becoming a real deployment surface, and the
Windows path is not just generic ONNX anymore. Windows ML is now Microsoft's
local inference layer over ONNX Runtime, and ONNX Runtime GenAI exposes logical
generator operations that may express the same Contenox session shape:

```text
append stable prefix
append changed suffix
generate
rewind to a stable token boundary
append alternate suffix
prove warm output equals cold full prompt
```

This is not the flagship main-node path. The flagship 7B/8B 64k-128k coding node
still starts with Intel/OpenVINO and llama.cpp profiles. ORT GenAI / Windows ML
is the AI PC compatibility lane for Windows x64, Windows ARM64, Qualcomm QNN,
AMD Ryzen AI / Vitis AI EP, DirectML, and CPU fallback.

## Corrected Thesis

Do not claim:

```text
ORT GenAI gives Contenox raw KV snapshots on every AI PC.
Windows ML makes all NPUs good 7B/64k coding engines.
DirectML/QNN/Vitis automatically support the same append/rewind behavior.
```

Claim only this:

```text
ORT GenAI exposes enough generator-level primitives to justify a measured
Contenox adapter spike. Each execution provider must advertise capabilities and
pass warm/cold equivalence before product code trusts it.
```

## Evidence And Caveats

The relevant ORT GenAI C++ surface includes:

```text
OgaModel
OgaConfig::AppendProvider / SetProviderOption
OgaTokenizer / ApplyChatTemplate
OgaGenerator
OgaGenerator::AppendTokenSequences
OgaGenerator::AppendTokens
OgaGenerator::GenerateNextToken
OgaGenerator::RewindTo
OgaGenerator::GetSequenceCount
OgaGenerator::GetSequenceData
```

That API shape is promising, but the provider matrix is the gate. ORT migration
docs say chat/system-prompt caching support is limited by provider. AMD's Ryzen
AI 1.7.1 article separately describes ONNX Runtime GenAI continuous decoding,
KV reuse, token appending, rewind, and branching on Ryzen AI. Treat that as an
AMD-specific claim to verify, not as proof every ORT EP behaves the same.

Windows ML is still valuable even if a provider fails append/rewind because it
can become the Windows distribution and device-discovery path. A provider that
only reaches chat baseline is still useful for fallback, but it gets no Contenox
workspace-context-reuse claim.

## Non-Goals

- Not replacing OpenVINO or llama.cpp.
- Not promising raw KV snapshot/restore unless an API exposes it safely.
- Not using NPU TOPS as a proxy for LLM performance.
- Not certifying 7B/8B 64k on Snapdragon/Ryzen NPU without benchmark proof.
- Not building an NPU sidecar path for embeddings, reranking, summaries, file
  triage, STT, vision, or repo refresh. The profile must run the main local
  session contract or stay out of scope.
- Not adding direct QNN/RyzenAI/RKLLM adapters before a bridge-runtime blocker is
  measured.

## Package Candidate

```text
runtime/modelrepo/ortgenai/
```

Expected shape:

```text
provider.go        catalog/profile integration
client.go          modelrepo client implementation
session.go         backend-neutral LocalSession adapter
capabilities.go    provider/device capability reporting
tokenizer.go       OgaTokenizer wrapper and template hashing
probe.go           installed runtime / provider / DLL probing
errors.go          structured error mapping
bench.go           warm/cold correctness and metrics harness
```

Build strategy:

```text
default build:
  no native dependency, probe stubs and profile parsing only

ortgenai build tag:
  CGo/native wrapper around ONNX Runtime GenAI C API or a small C++ shim

windows/arm64:
  separate packaging gate for DLL loading, search paths, and QNN assets
```

## Capability Contract

The adapter must report capabilities instead of letting product code infer them
from provider names:

```go
type OrtGenAICapability struct {
    RuntimeVersion       string
    Provider             string // cpu, DirectML, QNN, VitisAI, WindowsML, ...
    OS                   string
    Arch                 string
    DeviceClass          string // cpu, gpu, npu, hybrid
    ModelFormat          string // onnx-genai, qnn-context, provider-specific

    CanGenerate          bool
    CanStream            bool
    CanTokenize          bool
    CanApplyChatTemplate bool
    CanReportMetrics     bool
    CanCancel            bool

    HasPersistentSession bool
    CanAppendTokens      bool
    CanRewindToToken     bool
    CanReadSequence      bool
    CanSnapshotKV        bool
    CanRestoreKV         bool

    SupportsSystemPromptCache bool
    ReportsKVBytes           bool
    ReportsDeviceMemory      bool

    MaxTestedContext     int
    MaxTestedHotPrefix   int
    MaxTestedSuffix      int
    KnownLimitations     []string
}
```

Mapping to the shared local session interface:

```text
Tokenize        -> OgaTokenizer::Encode / ApplyChatTemplate
AppendTokens    -> OgaGenerator::AppendTokens or AppendTokenSequences
Rewind          -> OgaGenerator::RewindTo
DecodeNext      -> OgaGenerator::GenerateNextToken + GetSequenceData
Stream          -> repeated DecodeNext with tokenizer stream
SnapshotSave    -> unsupported until a real API exists
Branch          -> logical only if rewind + copied generator state is proven
```

## Correctness Gate

The first spike is not "it chats." The first spike is equivalence:

```text
stable = system/tools/repo prefix
suffix_a = user/task A
suffix_b = user/task B

cold_a = new generator + stable + suffix_a + greedy decode
warm_a = generator + stable, append suffix_a + greedy decode

rewind to len(stable)
warm_b = append suffix_b + greedy decode
cold_b = new generator + stable + suffix_b + greedy decode

required:
  warm_a == cold_a
  warm_b == cold_b
  mismatched profile/template/tokenizer => cache miss
```

Use deterministic greedy settings only. Do not compare sampled output.

Tiny/small models are preferred for this gate. The point is session semantics,
not a headline benchmark.

## Provider Lanes

| Lane | Role | Gate |
|---|---|---|
| CPU | baseline correctness and CI/dev fallback | A1/A2 append/rewind if supported |
| DirectML/GPU | broad Windows GPU fallback | must prove chat-mode append/rewind, not assumed |
| Qualcomm QNN EP | Snapdragon X / Windows ARM64 probe | QNN model assets, HTP backend, packaging, append/rewind equivalence |
| AMD Ryzen AI / Vitis AI EP | Ryzen AI NPU probe | 8k-16k small-model path first; verify AMD continuous-decoding claims locally |
| Windows ML | distribution/device management layer | discover EPs, package cleanly, preserve capability details |

Provider downgrade rule:

```text
If a provider loads and generates but cannot append/rewind exactly, it is A0
local chat only. It is not a Contenox workspace-state runtime.
```

## Windows ARM64 Gate

Snapdragon support is a product packaging milestone, not just a runtime flag:

```text
GOOS=windows GOARCH=arm64 build succeeds
CGo/native DLL search path is deterministic
ORT GenAI DLLs load
QNN/Windows ML provider discovery works
model asset layout is profile-declared
SQLite/storage paths work
streaming and cancellation behave predictably
zip/installer bundle can be tested on a clean machine
```

Do not hide this behind a Linux-only spike. If Windows ARM64 packaging fails,
the AI PC lane is not product-ready even if the API looks good.

## Small Standalone First

Near-term NPUs should be certified only as standalone small local sessions:

```text
1.5B-4B coder/helper at 4k-16k context
append/rewind equivalence
profile/template/tokenizer compatibility
structured cancellation
no cloud dependency
```

An NPU that cannot run the main local session contract does not get a separate
sidecar role in this plan.

## Bench Report

Each ORT GenAI profile should emit the same high-level report shape as other
llama backends, plus provider fields:

```json
{
  "profile_id": "qwen3-4b-ortgenai-qnn-snapdragon-x",
  "runtime": "ortgenai",
  "runtime_version": "...",
  "provider": "QNNExecutionProvider",
  "provider_version": "...",
  "os": "windows",
  "arch": "arm64",
  "hardware": "...",
  "model_format": "onnx-genai-qnn",

  "capabilities": {
    "append_tokens": true,
    "rewind_to_token": true,
    "system_prompt_cache": true,
    "snapshot_kv": false
  },

  "cold_full_prefill": {
    "tokens": 8192,
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

  "failure_cases": {
    "over_context": "structured_error",
    "cancel_decode": "session_valid_or_dead_explicit",
    "profile_mismatch": "cache_miss"
  }
}
```

## Phases

### OGA0 - Source And API Probe

```text
verify installed ORT GenAI version
list available providers
load a tiny ONNX GenAI model on CPU
tokenize and apply chat template
generate one greedy token
report capabilities as JSON
```

### OGA1 - Append/Rewind Equivalence

```text
append stable prefix
append suffix A
generate greedily
rewind to stable boundary
append suffix B
generate greedily
compare warm runs to cold full-prompt runs
```

Kill gate:

```text
If warm output cannot equal cold output under greedy decode, stop using that
provider/profile for workspace-state claims.
```

### OGA2 - Model Profile And Manifest Integration

```text
profile_id includes runtime/provider/model/tokenizer/template identity
token hashes and template digests participate in cache compatibility
provider limitations are persisted in the profile
```

### OGA3 - Windows ARM64 Packaging

```text
build Contenox for windows/arm64
load ORT GenAI and provider DLLs from the declared bundle
run OGA1 on Snapdragon hardware or an equivalent test device
emit bench JSON
```

### OGA4 - Small-Coder Certification

```text
1.5B-4B model
8k hot context minimum, 16k preferred where provider supports it
append/rewind equivalence
useful coding tasks, not only chat prompts
```

### OGA5 - Main-Coder Attempt

Only after OGA4 passes:

```text
try 7B/8B
try 32k, then 64k
publish memory, TTFT, decode, and failure behavior
promote to A5 only after measured proof
```

## Open Questions

- Does each provider preserve exact generator state after `RewindTo`, or is the
  API present but behavior provider-limited?
- Can device memory and KV size be reported accurately enough for cache policy?
- Can cancellation interrupt prefill/decode without corrupting the generator?
- How stable are model folders and provider-specific assets across ORT GenAI
  releases?
- Is Windows ML usable as the distribution layer while still exposing enough
  provider detail for Contenox certification?

## References

- Microsoft Windows ML overview: https://learn.microsoft.com/en-us/windows/ai/new-windows-ml/overview
- ONNX Runtime GenAI C++ API: https://onnxruntime.ai/docs/genai/api/cpp.html
- ONNX Runtime GenAI migration / chat mode: https://onnxruntime.ai/docs/genai/howto/migrate.html
- ONNX Runtime GenAI Snapdragon tutorial: https://onnxruntime.ai/docs/genai/tutorials/snapdragon.html
- ONNX Runtime QNN Execution Provider: https://onnxruntime.ai/docs/execution-providers/QNN-ExecutionProvider.html
- ONNX Runtime Vitis AI Execution Provider: https://onnxruntime.ai/docs/execution-providers/Vitis-AI-ExecutionProvider.html
- AMD Ryzen AI Software: https://www.amd.com/en/developer/resources/ryzen-ai-software.html
- AMD KV cache reuse on Ryzen AI / ORT GenAI: https://www.amd.com/en/developer/resources/technical-articles/2026/accelerating-local-llm-conversations-with-kv-cache-reuse-on-amd-.html

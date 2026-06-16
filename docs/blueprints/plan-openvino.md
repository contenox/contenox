# Plan: Contenox Local Coding Node on OpenVINO

> **Status:** research blueprint. OpenVINO is now the strongest narrow-stack
> candidate for a certified Contenox local coding node, but it is not selected
> until the substrate spike below proves the latency and embedding assumptions.
> The previous "thin LLMPipeline wrapper" plan remains useful as an on-ramp. The
> destination is a Contenox-owned workspace/session layer over OpenVINO's local
> inference primitives.

---

## Goal

Build a single-user, workspace-aware local coding node on a deliberately narrow
stack. Not a generic inference server, not a multi-model demo, not a flag-tuning
wrapper.

```text
Target:
  one developer, one machine, one active repo/workspace
  one hot 7B/8B-class coding model (INT4/INT8 weights)
  64k-128k hot context, 200k+ effective context
  first useful response < ~1-2 min, warm
  fully local / offline, no cloud, no API keys
  target hardware budget under ~1.5k EUR
  likely node: Intel Arc / Arc Pro B-Series dGPU, 32GB RAM minimum,
               64GB RAM preferred when the bill of materials allows it
```

The product advantage is **not** raw model serving. It is turning a coding
workspace into reusable model execution state: stable repo/tool/system prefixes
stay hot, only changed suffixes are re-prefilled, cold context is retrieved, and
state checkpoints are used only where they prove useful for durability, branch,
crash recovery, or measured warm start. "200k effective" comes from cache +
retrieval + pins + summaries, not from shoving 200k fresh tokens into dense
attention every turn.

Target UX (unchanged, still the front door):
```
contenox init
contenox model pull qwen2.5-coder-7b
contenox "trace this bug"
```

---

## Strategic framing

Two decisions from the design discussion drive this plan:

1. **Single vendor is deliberate, not accidental.** We are not chasing portable
   support for the whole hardware world in this product track. We pick one
   stack, prove it on one budget hardware class, and optimize deeply.

2. **The owned workspace/session layer is the differentiator.** OpenVINO may own
   the KV blocks, cache compression, sparse attention, and kernels. Contenox owns
   the coding semantics above them: deterministic repo segments, invalidation,
   warm workspace sessions, context budgeting, and when to reuse or rebuild a
   prefix.

---

## The three layers (what we reuse vs. what we own)

A model runtime is three stacked layers. Being honest about which we own is the
whole plan:

```text
Model/IO layer       : IR loading, arch support, tokenizer, chat templates,
                       sampling
                       -> REUSE OpenVINO + optimum-intel. Do not reimplement.
Inference primitives : paged KV, KV compression, prefix caching, sparse
                       attention, prefill/decode execution, device scheduling
                       -> REUSE OpenVINO where it exposes the right controls.
Contenox session     : workspace identity, context segments, invalidation,
                       cache policy, warm/cold session lifecycle, benchmarks
                       -> OWN. This is Contenox's runtime differentiation.
Kernel layer         : attention / matmul / dequant on CPU(AMX) / iGPU / Arc /
                       NPU
                       -> REUSE OpenVINO kernels. Never hand-write these first.
```

We do not write kernels. We do not reimplement tokenizers or the architecture
zoo. We **own the Contenox session layer** and only extend lower levels when a
measured product requirement cannot be met through the exposed OpenVINO surface.
The special knowledge OpenVINO cannot have is what a repo map is, when a diff
invalidates a segment, which prefix is stable, and what latency contract a
coding agent needs.

---

## Why OpenVINO (single-vendor substrate)

| | OpenVINO | Notes |
|---|---|---|
| Intel CPU/iGPU/Arc/NPU | Native (AVX2/AVX-512/AMX, GPU plugin, NPU plugin) | the budget hardware story |
| Tokenization / templates | Built-in (compiled as IR), Jinja applied automatically | no separate tokenizer lib |
| Model format | OpenVINO IR primary; GGUF preview for limited topologies | IR is the production path, GGUF is not yet a blanket replacement |
| Long-context primitives | prefix caching, KV compression, paged attention, sparse attention, cache eviction | several controls are documented through GenAI/OVMS scheduler configuration |
| Low-level state | OpenVINO Runtime state API exists for stateful models | must validate if it gives useful LLM KV snapshot granularity |
| Go ecosystem | No native Go bindings exist — we'd be first | first-mover OSS contribution |

The key reason OpenVINO works for an *owned* runtime (not just a wrapper) is that
it exposes the lower-level primitives we need below the high-level pipeline:

- **`ContinuousBatchingPipeline` + `SchedulerConfig`** — paged-attention KV
  blocks, `enable_prefix_caching`, `dynamic_split_fuse` (chunked prefill), KV
  block budget, and cache-eviction config. This is the prefix-reuse + chunked-
  prefill engine we would otherwise have to build from scratch.
- **Stateful model APIs** — OpenVINO Runtime exposes `query_state()` plus
  `VariableState::get_state()` / `set_state()` / `reset_state()` for stateful
  graphs. Spike S2 validated that a stateful OpenVINO LLM exposes useful KV
  tensors and that a CGo/C++ shim can snapshot and restore them across a fresh
  session on CPU. This is not yet a full prefix-segment product API; it is the
  low-level state primitive the product can build on.

> **Key risk to validate first (spike S0):** the GenAI **C API**
> (`src/c/include/openvino/genai/c/`) today centers on `llm_pipeline`. The
> `ContinuousBatchingPipeline` and the Runtime state APIs may be **C++-only**.
> If so, we write our own C++ shim over them (and, ideally, contribute the C
> bindings upstream — a genuine OSS contribution either way). This shim is the
> foundation everything else stands on; prove it exists and is bindable before
> committing to the rest.

---

## What the 2026 OpenVINO docs add

Current OpenVINO documentation is much closer to the Contenox Tier 3 thesis than
the older "run a small model locally" framing:

- **Long-context serving is explicit.** OpenVINO Model Server documents prefix
  caching and KV cache compression for long-context LLMs. Its Qwen2.5-7B-1M demo
  shows large TTFT reductions when repeated long prefixes hit the cache. The
  exact numbers are hardware- and setup-specific, and the 200k row in the docs
  is formatted ambiguously, so treat the table as directionally important rather
  than a Contenox performance promise.
- **Scheduler controls are real concepts, not marketing terms.** The GenAI
  `SchedulerConfig` exposes `enable_prefix_caching`, `cache_size`,
  `dynamic_split_fuse`, `max_num_batched_tokens`, `cache_eviction_config`,
  `use_sparse_attention`, and related fields. That is the control surface
  Contenox needs to test.
- **Sparse attention is now part of the documented surface.**
  `SparseAttentionConfig` documents `TRISHAPE` and `XATTENTION`, including
  retained-start tokens, retained-recent tokens, block size, stride, and
  threshold controls. OpenVINO 2026 release notes also describe XAttention as a
  preview feature for long-context TTFT.
- **KV compression is a first-class optimization.** Release notes document INT8
  KV compression defaults and INT4 KV compression options in parts of the stack,
  with warnings that some models can be sensitive to INT4. Contenox profiles
  must therefore treat KV precision as a tested model/hardware setting.
- **Modern model families are being targeted.** OpenVINO 2026 release notes call
  out improvements for models such as Qwen3-30B-A3B and GPT-OSS-20B, plus
  hybrid/sparse attention work. This matters because local long-context coding
  should track low-KV and hybrid-attention model shapes, not only dense Llama
  style models.
- **GGUF support exists, but it is preview and limited.** OpenVINO GenAI can
  instantiate some GGUF models directly. The docs currently list limited
  topology support such as SmolLM and Qwen2.5 and still recommend IR conversion
  for other models.
- **Speculative decoding is useful but secondary.** OVMS documents EAGLE3 with
  Qwen3-8B, which is relevant to decode latency. Its current documented
  limitations include no prefix caching in that mode, so it cannot replace the
  core warm-prefix strategy.

Implication: OpenVINO may let Contenox reuse more of the hard KV machinery than
we expected. The product work remains ours: make the coding workspace stable,
hashable, explainable, and cache-friendly.

---

## Two-tier runtime: simple pipeline + owned session engine

We keep both paths. They serve different needs and the simple one de-risks the
hard one.

```text
Tier 0  LLMPipeline path (the original plan)
        high-level, internal KV, start_chat/finish_chat
        -> fallback + simple one-shot chat + bring-up / parity baseline

Tier 1+ Contenox session engine (the new core)
        SchedulerConfig / prefix cache / sparse attention controls
        -> long-lived per-workspace sessions
        -> deterministic prefix segments + prefix-cache reuse
        -> suffix-only prefill
        -> optional state snapshots if the Runtime API proves useful
        -> coding-context planner above it (Go)
```

### Memory policy (non-negotiable for the latency target)

```text
GPU/Arc memory = hot live KV + active weights + compute buffers
system RAM     = cold session metadata, repo index, semantic cache,
                 optional state snapshots if supported
NVMe           = persistent per-repo session/cache metadata
```

Live attention KV must stay on the accelerator. Cold context influences answers
through retrieval, re-prefill, and optional state restore when supported. It
should not rely on streaming huge cold KV across the bus every generated token.
The normal hot loop is live prefix-cache reuse; state snapshots are a durability
and branching primitive until S3 measurements prove they belong in the latency
path.

---

## The owned engine: API we expose

A Contenox-owned C/C++ ABI over OpenVINO's lower-level primitives (built behind a
thin Go-side provider seam so product code never rots into C):

```c
// libcontenox_ov — Contenox session/KV/prefix layer over OpenVINO
cx_model_load(model_dir, device)          // CPU / GPU / NPU
cx_session_new(model)                     // long-lived, per workspace
cx_session_free(session)

cx_tokenize(session, text)
cx_prefill_chunked(session, tokens)       // dynamic_split_fuse under the hood
cx_decode_next(session)

cx_prefix_lookup(session, segment_hash)   // prefix-cache hit?
cx_prefix_commit(session, segment_hash)
cx_prefix_evict(session, policy)

cx_snapshot_save(session, path)           // optional: if state API is useful
cx_snapshot_restore(session, path)        // optional: if state API is useful
cx_session_branch(session)                // optional: if snapshots are useful

cx_bench_prefill(session) / cx_bench_decode(session)
```

Go owns workspace/session identity, segment hashing, the context planner, the
repo index, policy, and telemetry. C/C++ owns the model handle, the embedded
OpenVINO calls, the prefill/decode loop, and any state bytes that OpenVINO makes
safe to persist. **This is a Contenox local-node ABI over OpenVINO, not a new
transformer runtime.**

---

## The context planner (Go, substrate-independent)

The runtime assembles every turn as deterministic, hashable segments so prefix
cache hits are reliable:

```text
A system/developer prompt      stable, cached
B tool schemas                 stable, cached
C chain policy                 stable, cached
D repo instructions / AGENTS.md stable, cached
E repo map / symbol graph      semi-stable
F pinned files                 semi-stable
G current diff                 new
H terminal/test output         new
I current user turn            new
```

Rule: stable segments appear first and **byte-identically** every turn. Each
segment carries a manifest entry, not just a content hash:

```json
{
  "profile_id": "qwen2.5-coder-7b-int4-ov",
  "backend": "openvino",
  "backend_version": "...",
  "model_digest": "...",
  "tokenizer_digest": "...",
  "chat_template_digest": "...",
  "context_size": 65536,
  "kv_precision": "f16",
  "sparse_attention": "profile-tested",
  "cache_block_size": 32,
  "segments": [
    {
      "kind": "repo_map",
      "byte_hash": "...",
      "token_hash": "...",
      "token_start": 5100,
      "token_end": 17100,
      "cache_class": "task_pinned",
      "invalidation": "repo_index_change"
    }
  ]
}
```

A cache hit requires compatible model digest, tokenizer digest, chat template
digest, backend/runtime version, context/RoPE settings, KV precision, segment
token hash, token position, and cache block/page alignment. Byte-identical text
is not enough if the tokenizer, special tokens, BOS/EOS policy, chat template,
or model profile changed. Stable segments should prefer cache-block-aligned
boundaries so OpenVINO's block cache does not lose reuse on large partial tails.

A turn becomes: reuse A–D (+maybe E,F), append changed G–I, prefill only the
suffix, decode, and record the manifest + cache outcome.

Explainability is a first-class command:

```bash
contenox node explain-context
# system:      900 tokens   cached
# tools:      4200 tokens   cached
# repo map:  12000 tokens   cached
# pinned:    18000 tokens   cached
# diff:       2200 tokens   new
# user:        180 tokens   new
```

### Cache admission and eviction

OpenVINO can evict KV blocks, but Contenox should decide what deserves to stay
hot:

```text
highest: system/developer prompt, tool schemas, repo instructions
high:    repo map, pinned files, active task summary
medium:  current diff, recent failing test output
low:     stale terminal logs, old user turns, exploratory snippets
```

The policy is part of the product layer. Plain LRU can evict the most expensive
and most reusable coding prefix after a few large logs; that is a correctness
and latency bug for a single-user coding node.

---

## Success criteria (tiers)

Benchmarks are the product spec. Anchor models should include at least one
low-KV-head 7B/8B coding model and one current Qwen3-class model converted to
INT4 IR. The model is a **test vector, not the runtime identity.**

| Tier | Hardware profile | Target |
|---|---|---|
| T0 | any supported x86 CPU / iGPU | LLMPipeline chat works, parity baseline |
| T1 | Arc / Arc Pro 16GB-class, 32GB RAM minimum | 7B/8B INT4, 64k hot, first useful response < 60s |
| T2 | Arc / Arc Pro 16GB-class, 32-64GB RAM | 7B/8B INT4, 128k warm-prefix response < 120s |
| T3 | Arc Pro 24GB-class or better if the BOM fits | 200k **effective** via prefix cache + pins + retrieval + summaries + optional snapshots |

### The hard truth (why caching, not bigger windows)

Raw fresh prefill is the wall, not memory:

```text
 64k / 60s  = ~1067 prompt tok/s
128k / 120s = ~1067 prompt tok/s
```

So T2/T3 should not be marketed as "fresh dense N-k every turn." They are only
reachable as a local coding product by keeping stable prefixes hot and prefilling
deltas. The whole bet is that a coding workspace repeats the same segments all
day, which is exactly what a single-user node can exploit and a generic server
does not know how to plan around.

### Required report and go/no-go

Each OpenVINO model/hardware profile must emit the common local-node benchmark
report defined in `local-coding-node-goals.md`: cold full prefill, warm same
prefix, warm changed suffix, edited-stable-segment miss, snapshot save/restore,
decode, and failure cases. S3 is not complete until it records the suffix-growth
curve at 0, 256, 1k, 4k, 8k, and 16k changed tokens and verifies warm output
equivalence against a cold full prompt under deterministic decoding.

The go/no-go gate is blunt: 7B/8B coder, 64k hot prefix, 1k-8k edited suffix,
warm first useful response under 60s on target budget hardware, no memory spill
during decode, and structured recovery after cancellation.

---

## Foundation (kept from the original plan — still correct)

These sections are unchanged in intent; they are the on-ramp the new core stands
on. Full detail retained below.

- **C API surface & CGo bindings** — `llm_pipeline`, `generation_config`,
  `chat_history`, `json_container`, streaming callback bridge, `ov_status_e`
  error mapping. (See "Target C API Surface" / "CGo Considerations" below.)
- **Standalone Go package** `github.com/contenox/openvino-go` — reusable,
  upstream-visible. Extended with the session/state surface above.
- **Provider integration** — `runtime/modelrepo/openvino/` implementing
  `modelrepo.Provider` / `LLMChatClient` / `LLMStreamClient`, wired into
  `runtimestate`.
- **Model management** — `contenox model pull/list/remove`, IR layout under
  `~/.contenox/models/<name>/`, HF download with no Python dependency.
- **Distribution** — vendored `.so` (RPATH) for the one-binary story; static
  linking investigation; `.deb`/`.rpm`/Homebrew.

---

## Target C API Surface

The openvino-genai project (`openvinotoolkit/openvino.genai`) ships a C API at
`src/c/include/openvino/genai/c/`. The high-level surface is small and
purpose-built; the lower-level engine surface (CB pipeline + state) may need our
own C++ shim (spike S0).

### Headers to wrap

| Header | Purpose | Priority |
|---|---|---|
| `llm_pipeline.h` | Pipeline create/generate/stream/chat (Tier 0) | P0 |
| `generation_config.h` | Temperature, top_k, max_tokens, etc. | P0 |
| `chat_history.h` | Multi-turn conversation management | P0 |
| `json_container.h` | JSON data for messages/tools | P0 |
| `perf_metrics.h` | TTFT, throughput, token counts | P0 (now P0: bench is the spec) |
| ContinuousBatchingPipeline (C++ / our shim) | paged KV, prefix cache, split-fuse | **P0 for the core** |
| Runtime state API (C++ / our shim) | `query_state` / get/set/reset state | P0 spike; core only if useful for LLM KV |
| `vlm_pipeline.h` | Vision-language models | P3 — future |
| `whisper_pipeline.h` | Speech-to-text | P3 — future |

### High-level C function count (Tier 0 scope)

```
llm_pipeline.h:      ~12 functions
generation_config.h:  ~30 functions (mostly setters)
chat_history.h:       ~14 functions
json_container.h:      ~7 functions
```

**Tier 0 surface: ~63 functions** — tractable. The Tier 1+ engine surface is
smaller in function count but is where the real work and risk live.

---

## Key C API Functions (Tier 0)

### LLM Pipeline (llm_pipeline.h)

```c
// Opaque handles
typedef struct ov_genai_llm_pipeline_opaque ov_genai_llm_pipeline;
typedef struct ov_genai_decoded_results_opaque ov_genai_decoded_results;

// Lifecycle
ov_status_e ov_genai_llm_pipeline_create(
    const char* models_path,    // directory with .xml/.bin + tokenizer
    const char* device,         // "CPU", "GPU", "NPU"
    const size_t property_args_size,
    ov_genai_llm_pipeline** pipe,
    ...);                       // variadic properties (needs C shim)
void ov_genai_llm_pipeline_free(ov_genai_llm_pipeline* pipe);

// Generate (single prompt)
ov_status_e ov_genai_llm_pipeline_generate(
    ov_genai_llm_pipeline* pipe,
    const char* inputs,
    const ov_genai_generation_config* config,
    const streamer_callback* streamer,     // NULL for non-streaming
    ov_genai_decoded_results** results);

// Generate (multi-turn chat)
ov_status_e ov_genai_llm_pipeline_generate_with_history(
    ov_genai_llm_pipeline* pipe,
    const ov_genai_chat_history* history,
    const ov_genai_generation_config* config,
    const streamer_callback* streamer,
    ov_genai_decoded_results** results);

// Chat session (Tier 0 — internal KV, black box)
ov_status_e ov_genai_llm_pipeline_start_chat(ov_genai_llm_pipeline* pipe);
ov_status_e ov_genai_llm_pipeline_finish_chat(ov_genai_llm_pipeline* pipe);

// Results
ov_status_e ov_genai_decoded_results_get_string(
    const ov_genai_decoded_results* results,
    char* output,          // NULL on first call to get size
    size_t* output_size);
ov_status_e ov_genai_decoded_results_get_perf_metrics(
    const ov_genai_decoded_results* results,
    ov_genai_perf_metrics** metrics);
```

### Streaming Callback

```c
typedef enum {
    OV_GENAI_STREAMING_STATUS_RUNNING = 0,
    OV_GENAI_STREAMING_STATUS_STOP    = 1,
    OV_GENAI_STREAMING_STATUS_CANCEL  = 2,
} ov_genai_streaming_status_e;

typedef struct {
    ov_genai_streaming_status_e (*callback_func)(const char* str, void* args);
    void* args;
} streamer_callback;
```

### Generation Config (generation_config.h)

```c
ov_genai_generation_config_create(ov_genai_generation_config** config);
ov_genai_generation_config_free(ov_genai_generation_config* handle);

ov_genai_generation_config_set_max_new_tokens(config, size_t)
ov_genai_generation_config_set_temperature(config, float)
ov_genai_generation_config_set_top_p(config, float)
ov_genai_generation_config_set_top_k(config, size_t)
ov_genai_generation_config_set_do_sample(config, bool)
ov_genai_generation_config_set_repetition_penalty(config, float)
ov_genai_generation_config_set_stop_strings(config, const char** strings, size_t count)
ov_genai_generation_config_set_stop_token_ids(config, const int64_t* ids, size_t count)
// ... ~20 more setters for beam search, penalties, etc.
```

### Chat History (chat_history.h)

```c
ov_genai_chat_history_create(ov_genai_chat_history** history);
ov_genai_chat_history_free(ov_genai_chat_history* history);
ov_genai_chat_history_push_back(history, const ov_genai_json_container* message);
ov_genai_chat_history_set_tools(history, const ov_genai_json_container* tools);
ov_genai_chat_history_size(history, size_t* size);
ov_genai_chat_history_clear(history);
// Messages are JSON: {"role":"user","content":"..."}
```

### JSON Container (json_container.h)

```c
ov_genai_json_container_create_from_json_string(
    ov_genai_json_container** container,
    const char* json_str);
ov_genai_json_container_to_json_string(
    container, char* output, size_t* output_size);
ov_genai_json_container_free(ov_genai_json_container* container);
```

---

## CGo Considerations

### Variadic function problem

`ov_genai_llm_pipeline_create` is variadic (`...`). CGo cannot call C variadic
functions. Solution: a small C shim that wraps the variadic call:

```c
// shim.c — compiled alongside the Go package
#include <openvino/genai/c/llm_pipeline.h>

ov_status_e ov_genai_llm_pipeline_create_simple(
    const char* models_path,
    const char* device,
    ov_genai_llm_pipeline** pipe) {
    return ov_genai_llm_pipeline_create(models_path, device, 0, pipe);
}

ov_status_e ov_genai_llm_pipeline_create_with_cache(
    const char* models_path,
    const char* device,
    const char* cache_dir,
    ov_genai_llm_pipeline** pipe) {
    return ov_genai_llm_pipeline_create(
        models_path, device, 2, pipe,
        "cache_dir", cache_dir);
}
```

The same shim file is where the **CB pipeline + state-API C++ wrappers** live if
the C API does not expose them (spike S0).

### String output pattern

OpenVINO C API uses a two-call pattern for string results:
1. Call with `output=NULL` → get `output_size`
2. Allocate buffer, call again → get string

```go
func (r *DecodedResults) String() (string, error) {
    var size C.size_t
    if status := C.ov_genai_decoded_results_get_string(r.ptr, nil, &size); status != 0 {
        return "", statusError(status)
    }
    buf := make([]byte, size)
    if status := C.ov_genai_decoded_results_get_string(r.ptr, (*C.char)(unsafe.Pointer(&buf[0])), &size); status != 0 {
        return "", statusError(status)
    }
    return string(buf[:size-1]), nil // trim null terminator
}
```

### Streaming callback

The streamer callback crosses the CGo boundary. Use `cgo.NewHandle` to pass Go
state through `void* args`:

```go
//export goStreamerCallback
func goStreamerCallback(str *C.char, args unsafe.Pointer) C.ov_genai_streaming_status_e {
    h := cgo.Handle(args)
    ch := h.Value().(chan string)
    ch <- C.GoString(str)
    return C.OV_GENAI_STREAMING_STATUS_RUNNING
}
```

---

## Go Package Design

### Package: `openvino` (standalone, reusable, upstream-visible)

```
github.com/contenox/openvino-go/
├── openvino.go           // Pipeline, Config, ChatHistory types (Tier 0)
├── session.go            // Session, prefix cache, snapshot/restore (Tier 1+)
├── shim.c / shim.h       // C wrappers for variadic + CB pipeline + state APIs
├── status.go             // ov_status_e → Go error mapping
├── config.go             // GenerationConfig builder
├── chat.go               // ChatHistory + JSON container wrappers
├── stream.go             // Streaming callback bridge
├── metrics.go            // PerfMetrics
├── bench.go              // prefill/decode/TTFT/cache-hit harness
├── openvino_test.go      // Integration tests (need a model dir)
└── README.md
```

### Go API sketch — Tier 0

```go
type Pipeline struct { ptr *C.ov_genai_llm_pipeline }

func NewPipeline(modelDir, device string) (*Pipeline, error)
func (p *Pipeline) Close()
func (p *Pipeline) Generate(prompt string, opts ...ConfigOption) (string, error)
func (p *Pipeline) GenerateWithHistory(h *ChatHistory, opts ...ConfigOption) (string, error)
func (p *Pipeline) Stream(prompt string, opts ...ConfigOption) (<-chan string, error)

type ConfigOption func(*GenerationConfig)
func WithMaxNewTokens(n int) ConfigOption
func WithTemperature(t float32) ConfigOption
func WithStopStrings(ss ...string) ConfigOption
```

### Go API sketch — Tier 1+ (the new core)

```go
// Session is a long-lived, per-workspace inference context with explicit
// prefix-cache control and optional state snapshot support.
type Session struct { /* ... */ }

func (p *Pipeline) NewSession(opts ...SessionOption) (*Session, error)
func (s *Session) PrefillChunked(tokens []int32) error      // dynamic_split_fuse
func (s *Session) DecodeNext() (int32, error)

func (s *Session) PrefixLookup(segmentHash string) (hit bool)
func (s *Session) PrefixCommit(segmentHash string) error
func (s *Session) PrefixEvict(policy EvictPolicy) error

func (s *Session) SnapshotSave(path string) error           // optional state API path
func (s *Session) SnapshotRestore(path string) error        // optional state API path
func (s *Session) Branch() (*Session, error)                // optional state API path

func (s *Session) Bench() BenchResult                       // prefill/decode/TTFT/hit-rate
```

### Metrics

```go
type Metrics struct {
    TTFTCold     float32 // ms
    TTFTWarm     float32 // ms (prefix cache hit)
    PrefillTPS   float32 // prompt tokens/sec
    DecodeTPS    float32 // output tokens/sec
    CacheHitRate float32
    InputTokens  int
    OutputTokens int
}
```

---

## Integration into Contenox

### Model provider: `runtime/modelrepo/openvino/`

Implements the existing `modelrepo.Provider` interface. Tier 0 maps directly to
`LLMChatClient.Chat()` via `GenerateWithHistory`; Tier 1+ drives a `Session` and
the context planner.

```go
type openvinoProvider struct {
    pipeline *openvino.Pipeline
    sessions *SessionPool        // per workspace/repo
    planner  *ContextPlanner     // segment assembly + invalidation
    model    string
    caps     modelrepo.CapabilityConfig
}
```

`LLMChatClient.Chat()` (Tier 1+ path):
1. Plan context segments, hash each, order stable-first.
2. Reuse warm prefixes through OpenVINO prefix caching; restore state only if the
   spike proves it is safe and useful.
3. `PrefillChunked` only the changed suffix (diff + tools delta + user turn).
4. Decode; map tool calls from the response.
5. Record cache metrics and, if supported, persist useful state checkpoints per
   repo/session.

### Backend registration

```
contenox backend add local --type openvino --model-dir ~/.contenox/models/qwen2.5-coder-7b-int4
```

Auto-detect: if `~/.contenox/models/` contains OpenVINO IR, register a local
backend automatically.

### Model profiles (not "read n_ctx and believe it")

Each model ships a tested profile — context the runtime is allowed to use is a
benchmarked value, not the architecture's theoretical max:

```json
{
  "id": "qwen2.5-coder-7b-int4-ov",
  "family": "qwen2.5-coder",
  "format": "openvino-ir",
  "weights": "int4",
  "default_context": 65536,
  "max_tested_context": 131072,
  "kv_precision": "u8",
  "sparse_attention": "profile-tested",
  "prefix_caching": "required",
  "device_preference": ["GPU", "CPU", "NPU"],
  "warning": "do not exceed max_tested_context without a fresh benchmark"
}
```

---

## Model Format & Distribution

### OpenVINO IR model directory layout

```
~/.contenox/models/qwen2.5-coder-7b-int4/
├── openvino_model.xml / .bin           # graph + weights
├── openvino_tokenizer.xml / .bin       # tokenizer (compiled as OV model)
├── openvino_detokenizer.xml / .bin
├── config.json / generation_config.json
├── tokenizer.json / tokenizer_config.json / special_tokens_map.json
└── chat_template.jinja                 # applied automatically
```

Tokenization and chat templates are automatic — loaded from the model directory.
No external tokenizer library.

### Converting models

```bash
pip install optimum-intel[openvino] nncf
optimum-cli export openvino \
    --model Qwen/Qwen2.5-Coder-7B-Instruct \
    --weight-format int4 \
    ./qwen2.5-coder-7b-int4
```

Pre-converted models exist under the HuggingFace `OpenVINO/` org. `contenox model
pull` downloads IR snapshots over the HF Hub HTTP API (no Python at runtime),
verifies checksums, shows progress.

---

## Runtime Dependencies & Distribution

Shared libraries at runtime:

```
libopenvino_genai_c.so / libopenvino_genai.so   # GenAI + C API
libopenvino.so / libopenvino_c.so               # Runtime core + C API
libopenvino_intel_cpu_plugin.so                 # CPU plugin (AMX)
libopenvino_intel_gpu_plugin.so                 # Arc / iGPU plugin
libopenvino_intel_npu_plugin.so                 # NPU plugin
libopenvino_tokenizers.so                       # tokenizer plugin
```

Distribution options (best → simplest): **static link** (true single binary,
build OpenVINO with `-DBUILD_SHARED_LIBS=OFF`) → **vendored `.so` + RPATH**
(`contenox` + `lib/`) → **system install** (`apt install openvino
openvino-genai-dev`). Ship `.deb`/`.rpm`/Homebrew with libs included.

System requirements: Linux, x86_64 with AVX2, Intel Arc / Arc Pro dGPU for the
certified node path, and fast NVMe for persistent cache state. 32GB RAM is the
minimum budget target; 64GB is preferred if the bill of materials still fits.
Live KV should be sized for accelerator memory, not system RAM spill.

---

## Performance reality

The old plan's "4B INT4 CPU, ~20-40 tok/s decode" numbers describe **Tier 0
interactive chat** and remain a fair baseline for that path. They are **not** the
spec for the coding node. The coding-node spec is:

```text
prefill tok/s @ 64k and @ 128k   -> decides whether cold start fits 1-2 min
TTFT warm (prefix cache hit)     -> the number users actually feel all day
decode tok/s                     -> whether responses feel alive
state checkpoint time            -> whether optional snapshotting is useful
cache hit rate                   -> whether the planner is doing its job
```

Whether Arc/NPU/AMX hit these for a 7B INT4 at 64-128k is the single most
expensive unknown in the plan, and it is measurable in days on one machine.

---

## Implementation Phases

### Phase 0 — Substrate spike (1 week) — **gate**

- [ ] **S0:** map which OpenVINO long-context features are available in the
      embeddable GenAI C++ API, which are only exposed through OVMS today, and
      which require a C++ shim or upstream work.
- [x] **S1:** confirm `ContinuousBatchingPipeline` / `SchedulerConfig` style
      controls are reachable from the Contenox process. Minimum proof:
      `enable_prefix_caching`, KV precision, cache size, split-fuse/chunked
      prefill, sparse attention controls, and perf metrics. Result: a
      build-tagged Go/CGo probe constructs OpenVINO GenAI
      `ContinuousBatchingPipeline` from Contenox, applies
      `SchedulerConfig` with prefix caching, cache size, dynamic split-fuse, and
      XAttention sparse-attention controls, compiles with
      `KV_CACHE_PRECISION=f16`, generates one token, and reads
      `PipelineMetrics`. The probe currently keeps the GenAI pipeline alive for
      process lifetime because `ContinuousBatchingPipeline` destruction inside
      the CGo call corrupts the Go test process; production lifecycle handling
      remains a follow-up.
- [x] **S1.5:** promote the GenAI proof into a minimal provider path without
      waiting for benchmarks. Result: `ovsession` now has a worker-thread-backed
      GenAI session ABI with create/generate/stream/metrics/cancel/close; the
      OpenVINO provider advertises prompt/chat/stream only in
      `openvino openvino_genai` builds and wires them through
      `ContinuousBatchingPipeline`. Model directories can provide a strict
      `contenox-openvino.json` profile for scheduler/session settings. Prompt,
      chat, and stream clients share pooled GenAI sessions. Embeddings and
      coding-context integration remain follow-up work.
- [x] **S2:** test whether Runtime state APIs can snapshot useful LLM state.
      Result: `prefill -> snapshot_save -> fresh session -> snapshot_restore ->
      decode` reproduces greedy continuation exactly on
      `OpenVINO/Qwen2.5-Coder-0.5B-Instruct-int4-ov` CPU when compiled with
      `KV_CACHE_PRECISION=f16`. OpenVINO exposes the state tensors as `float32`
      through `get_state()` even with f16 KV storage configured.
- [x] **S2.5:** prove the deterministic assembler drives cache behavior. Result:
      stable-first `AssembleContext` segments predict OpenVINO prefix-cache hits;
      same stable prefix warmed from 7.61s to 73ms on CPU, while an edited stable
      segment forced a cold re-prefill.
- [x] **S2.7:** add strict parser protocol registry for tool-call and reasoning
      parsing. Result: no raw model-output regex fallback, no Contenox-invented
      tool-call schema, and parser selection comes from model/profile-declared
      OpenVINO protocols. Python-only adapters such as `VLLMParserWrapper` remain
      explicit non-support in the native bridge.
- [ ] **S3:** benchmark 7B/8B INT4 on the target Intel device: prefill tok/s @
      64k, warm TTFT after prefix cache hit, decode tok/s, model load time, KV
      memory growth, cache eviction behavior, snapshot save/restore bytes/ms,
      failure cases, and warm-suffix output equivalence against cold full prompt.

### Phase 1 — Tier 0 bindings (2-3 weeks)

`github.com/contenox/openvino-go`: variadic shim, `Pipeline`
create/close/generate/stream, `GenerationConfig`, `ChatHistory`/`JSONContainer`,
streaming bridge, `PerfMetrics`, error mapping, integration tests, CI with
OpenVINO installed, README. Ships the "no Ollama, local chat" front door.

### Phase 2 — Provider + long-lived sessions (2 weeks)

`runtime/modelrepo/openvino/` implementing `modelrepo.Provider` /
`LLMChatClient` / `LLMStreamClient`; session pool keyed by workspace; **stop
recreating the inference context per turn.**

### Phase 3 — Deterministic context segments (2 weeks)

Context planner, token cache, stable byte-identical ordering, segment
invalidation from repo/git state, manifest generation, token hashes, cache-block
alignment hints, profile compatibility checks, and `contenox node
explain-context`.

### Phase 4 — Prefix cache + optional state checkpoints (3-4 weeks) — **the core**

`PrefixLookup/Commit/Evict`, semantic cache admission/eviction policy,
suffix-only prefill, and optional `SnapshotSave/Restore` if the state API proves
useful. Start with deterministic coarse prefixes (`system+tools`,
`+repo-rules`, `+repo-map`, `+pinned`), reuse the nearest cached prefix, prefill
the rest, and prove warm output equals cold full prompt output under
deterministic decoding.

### Phase 5 — Repo-scale coding context (ongoing)

Symbol/import/test/git graphs, file summaries, lexical + semantic search, manual
and automatic pins, context budgeter, and the coding-context eval gate from
`local-coding-node-goals.md`. At this point 200k **effective** context is real
only if the benchmark gates and repo-task evals pass.

### Phase 6 — Distribution (ongoing)

Vendored `.so` → static link; `.deb`/`.rpm`/Homebrew; multi-OS as a second pass
(Linux/Intel is the first hard-optimized path).

---

## Resolved questions (were "Open Questions")

1. **KV/cache management** — no longer deferred. It is the core deliverable
   (Phase 4), built first on OpenVINO prefix caching, KV compression, cache
   eviction, and sparse attention. Raw state save/restore is a spike item, not a
   public promise.
2. **Long conversations / memory growth** — handled by prefix segments + cache
   eviction + optional state checkpoint offload to RAM/NVMe, not by hoping the
   pipeline copes.
3. **Concurrency** — explicitly a non-goal. One user, one active decode stream,
   interruptible prefill, exact session ownership. We do not inherit the
   multi-tenant server problem.

## Still open (validate, don't assume)

1. **Embedded surface vs. OVMS surface.** Some long-context controls are clearly
   documented in OVMS. The product needs the same controls embedded or shimmed,
   not a user-managed server process.
2. **State granularity.** Whole-session KV snapshot/restore works. Still
   validate whether snapshots are fast and small enough at 7B/8B and whether
   prefix-segment restore should use raw state snapshots, OpenVINO prefix-cache
   behavior, or both.
3. **Prefix-cache determinism.** Does prefix caching reuse blocks reliably for
   byte-identical stable segments, or only opportunistically? This determines
   warm TTFT predictability.
4. **Sparse attention accuracy.** XAttention/TRISHAPE settings trade compute for
   possible accuracy loss. Each model profile needs a correctness gate, not only
   a speed benchmark.
5. **Speculative decoding interaction.** EAGLE3 can improve decode latency, but
   the current OVMS docs list prefix caching as unsupported in that mode. Treat
   it as secondary until proven compatible with the warm-prefix path.
6. **Arc Pro availability and pricing.** B50/B60-class hardware is interesting
   only if retail availability keeps the certified node below the budget target.
7. **Tool-call parser coverage.** Native parser protocols are now selected
   through strict model profile declarations, and raw regex fallback is forbidden.
   Remaining risk is coverage: some OpenVINO parser adapters, notably the Python
   `VLLMParserWrapper`, are not native C++ bridge primitives and need an explicit
   bridge or a different profile-declared parser.

---

## References

- OpenVINO 2026 release notes: https://docs.openvino.ai/2026/about-openvino/release-notes-openvino.html
- OpenVINO long-context optimizations: https://docs.openvino.ai/2026/model-server/ovms_demo_long_context.html
- OpenVINO Model Server LLM reference: https://docs.openvino.ai/2026/model-server/ovms_docs_llm_reference.html
- OpenVINO GenAI inference and GGUF preview: https://docs.openvino.ai/2026/openvino-workflow-generative/inference-with-genai.html
- OpenVINO GenAI `SchedulerConfig`: https://docs.openvino.ai/2026/api/genai_api/_autosummary/openvino_genai.SchedulerConfig.html
- OpenVINO GenAI `SparseAttentionConfig`: https://docs.openvino.ai/2026/api/genai_api/_autosummary/openvino_genai.SparseAttentionConfig.html
- OpenVINO speculative decoding demo: https://docs.openvino.ai/2026/model-server/ovms_demos_continuous_batching_speculative_decoding.html
- OpenVINO 2025.3 release notes with Arc Pro B-Series support: https://www.intel.com/content/www/us/en/developer/articles/release-notes/openvino/2025-3.html
- Intel Arc Pro B60 datasheet: https://www.intel.com/content/dam/www/central-libraries/us/en/documents/2026-03/datasheet-b60-gpu.pdf
- OpenVINO GenAI source and C API headers: https://github.com/openvinotoolkit/openvino.genai
- Pre-converted OpenVINO models: https://huggingface.co/OpenVINO
- Model conversion with Optimum Intel: https://huggingface.co/docs/optimum-intel/en/openvino/export

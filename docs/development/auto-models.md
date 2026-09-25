# Automatic local model selection

REVISED 2026-09-22, replacing context-first selection: automatic selection now gates on
curated agentic capability and treats context as a floor rather than a prize. `SelectAuto` and
the curated registry are the source of truth; `contenox model registry-list` lists downloadable
entries.

`AutoEligible` records a permissive license, an accessible artifact, and compatible architecture and tool parser in the pinned backend. It does **not** certify a successful generation on every device. GGUF downloads are pinned to verified Hugging Face revisions and exact file sizes. Existing OpenVINO repository downloads follow their current repository snapshots; their exported configurations and sizes were checked during this revision.

## The agentic baseline

`AgenticTier` is the curation-side capability signal: `agentic` when the model is evidenced to
run a multi-step tool loop, `tool-chat` when it can call a tool but is not evidenced to sustain
one, and `none` when the runtime cannot read calls from its template at all. `AgenticEvidence`
records why that tier was assigned — a measured multi-turn result where the generation has one,
training provenance where it does not.

Automatic selection picks nothing below `agentic`. A large window on a model that cannot use it
is not a capability: the ordering this replaces preferred whichever model reached 131072 hot
tokens, which is how a 0.8B model came to be installed on machines that could run a 9B one. A
model curated as `tool-chat` remains downloadable and selectable by name; it is only excluded
from the automatic pick.

## The KV cache type is an input

A window costs device memory, and on a small card that cost is what decides
whether a model fits at all. `capacity.LayerKVProfile` is an f16 estimate, and
automatic selection scales it by the cache type the worker will run — read from
`CONTENOX_LLAMA_KV_CACHE_TYPE`, the same variable modeld reads. Unset means f16;
`q8_0` halves the cache and therefore buys roughly twice the window on the same
device, which is what lets a 6-8 GB machine hold a 4B-class model at a usable
context instead of nothing. Selection and the worker must agree: an estimate
made for one type while the session allocates another would promise a window
that cannot be opened. `auto` prints the type it planned with, and the opened
session's real hot allocation is re-checked after download.

## The small-device class

The agentic tier used to start at roughly 5 GB of weights, which left a 6 GB
device with no automatic pick at all and told its operator to buy a bigger
machine. It now carries 2-3B entries whose KV geometry a small card can afford
(`minicpm5-2b`, `nanbeige4-3b-thinking`): on a 6 GiB card the first fits with ~75K
tokens of f16 context, and the second with ~35K — ~65K at q8_0, which is that
model's own ceiling.

The budget the planner may spend is nearly the free card. The reserve floor kept
for every other client on the device is the only deliberate subtraction beyond
the model's own weights, runtime overhead and a small fragmentation headroom. A
deeper cap protects nobody and hides VRAM the operator paid for: it is what kept
a 4 GB model off a card that could hold it, and what made a bigger tier
unreachable on the hardware it was curated for.

Most community GGUF quants of those models carry no `tokenizer.chat_template`,
because the quantiser strips it. llama.cpp then falls back to its built-in ChatML
template, which declares no tool-call markers: the tool definitions are never
rendered in the dialect the model was trained on, and its calls come back as
prose. Such an entry therefore names a `ChatTemplateURL`, `model pull` fetches it
as `chat_template.jinja` beside the weights, and modeld hands it to llama.cpp in
place of the GGUF's own template. The template is not optional the way a vision
projector is: a missing projector costs images, a missing template costs the
model's ability to call anything.

## Selection and hot context

The input budget is the hardware probe's usable memory after device reservations. The selector deducts all weights, including a projector, and a conservative runtime allowance of the larger of one quarter of weight bytes or 256 MiB. It budgets the remaining memory with modeld's `capacity.LayerKVProfile.ContextForBudget`, using f16 KV storage and the publisher configuration's context ceiling. Advisory VRAM tiers do not constrain this calculation.

A model must first hold `MinAgenticHotContext` (32768 tokens) of usable context: an agent spends
its window on the tool catalogue before it spends it on the task, and a model that cannot hold
that much calls a tool once and then runs out. Among the models that clear the gate the larger
one wins — parameter count is a weak signal across families and a fair one inside a tier — then
the larger hot context, then the name. Release date is not a tiebreak, so a new release does not
displace a model that already does the job; and the caller keeps the previous model while it
still qualifies, so a working pick is never swapped for a marginal gain. Context is capped at the
lesser of the verified model limit and 286720 tokens. Explicit origins never fall back to another
origin; empty or `auto` allows all origins. When nothing qualifies, selection fails rather than
degrading, and the error names the smallest curated agentic model of that origin, the memory it
would need, and the alternatives: raise the budget, choose another origin, or use a hosted
backend.

The estimate excludes cold storage, eviction-backed history and planner-effective context. After download and a tool-call check, setup reads the opened session's hot allocation through `ExplainContext`; an earlier `Describe` estimate is not treated as a verified allocation. That check is a single probe tool with one required parameter: it certifies that a call parses, not that the model selects and fills calls across turns. Grading it is the open work that would let a below-baseline model earn its place at runtime instead of by curation alone. A successful estimate is not a throughput guarantee: prefill and long-context inference still depend on memory bandwidth and hardware execution support.

The source configurations distinguish full and sliding attention. Gemma profiles use the larger per-layer KV footprint across the two attention types, without assuming savings from shared KV or equal K/V. Qwen3.5/3.8 are conservatively treated as dense because the current modeld GGUF inspection does not expose their recurrent/full-attention split. GigaChat MLA likewise receives a conservative dense estimate rather than unverified compressed-cache savings. These choices can select a smaller model than a more complete backend estimator would permit. Unknown context or KV metadata excludes a model from automatic selection.

For recurrent models, a changed prefix currently requires recomputation when the
backend cannot rewind its state. That work reports zero reused tokens. Resident
context capacity is therefore separate from prefix-cache reuse and turn latency.

## Publisher and artifact evidence

Candidate discovery included the broad local-model landscape; publisher cards and exported configurations determine eligibility, not comparison-site rankings.

- **China:** [Qwen's release record](https://github.com/QwenLM/Qwen3.8/blob/main/README.md) identifies Qwen3.8-27B and the Qwen3.5 small releases. [Qwen3.8's publisher card and configuration](https://huggingface.co/Qwen/Qwen3.8-27B) declare Apache-2.0 and `qwen3_5`; [Unsloth's quantized artifacts](https://huggingface.co/unsloth/Qwen3.8-27B-GGUF) and the corresponding Qwen3.5 repositories supply pinned GGUFs. Architecture support exists at the llama.cpp pin, but the new exact model has not been runtime-certified here.
- **US:** [Gemma4](https://blog.google/innovation-and-ai/technology/developers-tools/gemma-4/) and [Gemma4 12B](https://blog.google/innovation-and-ai/technology/developers-tools/introducing-gemma-4-12b/) are Apache-2.0 releases. [ggml-org artifacts](https://huggingface.co/ggml-org/gemma-4-12B-it-GGUF) provide accessible model/projector pairs. [IBM's Granite4.1 announcement](https://research.ibm.com/blog/granite-4-1-ai-foundation-models) and [publisher GGUFs](https://huggingface.co/ibm-granite/granite-4.1-3b-GGUF) provide a current dense alternative with Apache-2.0 licensing and an exact upstream chat-template test.
- **EU:** [Mistral3](https://mistral.ai/news/mistral-3/) supplies Apache-2.0 Ministral3 small models; [publisher GGUFs](https://huggingface.co/mistralai/Ministral-3-3B-Instruct-2512-GGUF) have native Ministral tool formatting supported by the pinned common-chat parser. Their published context ceiling is 262144, not an invented extension. More recent Mistral Small4 and Medium3.5 releases are substantially larger; no manageable, checked artifact was admitted merely to claim the latest release.
- **GUS:** [GigaChat3.1's publisher card](https://huggingface.co/ai-sage/GigaChat3.1-10B-A1.8B) and [publisher GGUF](https://huggingface.co/ai-sage/GigaChat3.1-10B-A1.8B-GGUF) declare MIT. The pinned backend includes its tool template. [The publisher's later releases](https://huggingface.co/ai-sage/models) were also considered; GFusion lacks a checked quantized combination here, and larger GigaChat releases are not interchangeable desktop candidates.

The region is publisher origin, not a network routing or data-residency assertion. The source model's `config.json` supplies layers, attention heads, KV heads, head dimensions, window sizes and context ceilings. Exact artifact revisions and bytes live in `curated.go`.

## Exclusions and backend differences

[Meta Muse Glimmer30B](https://huggingface.co/meta-models/Muse-Glimmer-30B) is an accessible Apache-2.0 model, but its `muse_glimmer` architecture is absent from the pinned llama.cpp architecture/converter support. It remains excluded rather than being silently treated as another transformer.

The checked OpenVINO release exports current Qwen3.5/3.8 and Gemma4 through its vision-language path. Modeld's current vision session rejects model-native tool definitions, so those exports cannot automatically serve the harness tool loop. Compatible text-only Qwen3, Qwen3-Coder, Phi4-mini and GPT-OSS exports remain candidates. Their actual exported context ceilings apply; Qwen3's ordinary export is 40960 tokens, and the checked Phi4-mini export declares 4096. No automatic YaRN or rope extension is assumed. An explicit region with no compatible OpenVINO candidate returns an error.

The checked llama.cpp pin is `86b94708f22478f900b76ca02e316f4f3418faff`. Architecture and template evidence comes from its `src/llama-arch.cpp`, converter definitions, `common/chat.cpp` and `tests/test-chat.cpp`. Tests cover origin isolation, memory accounting, native context ceilings, the agentic gate
(including that a `tool-chat` model with the largest window is still never picked) and
capability-first ordering across hardware budgets. They do not substitute for loading and exercising each artifact on the selected device.

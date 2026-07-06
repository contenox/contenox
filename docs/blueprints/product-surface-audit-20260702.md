# Working Doc: Product Surface Audit — "does everything we surface actually work?"

Status: working document; drives the full-E2E test program and the niche-down decision.
Owner: runtime
Companion: capacity-bug-inventory-20260702.md (capacity bugs, mostly fixed/committed).

## Why this doc

Goal shift: from fixing individual bugs to auditing everything the product
starts, surfaces, or promises — verify it works end-to-end, fix or remove what
doesn't, and decide where to niche down for maintenance leverage. "The model
decided" is not an acceptable control boundary — and neither is "the README
promised".

## The "weird shit", concretely (evidence found so far)

### Class 1 — promises that break in the user's hands

| Item | Evidence | Severity |
|---|---|---|
| Curated gemma4 GGUFs are unservable | `runtime/modelregistry/curated.go` ships 4 gemma4 entries (5-17 GB downloads); the pinned llama.cpp (`tmp/ref/llama.cpp/src/llama-arch.cpp`) has GEMMA/2/3 but **no LLM_ARCH_GEMMA4**. A user can `model pull` 17 GB and get "unsupported architecture". Known incident per modeld-capability-truth-blueprint — still shipping. | high |
| openrouter advertised but not in the service validation list | CLI front-page help + `backend_cmd.go:51,72,142` promote `--type openrouter`; `backendservice.go:80` validation list omits it (runtimestate handles it at :344). Needs a live add+chat test — either validation rejects an advertised type, or dead validation. | high (test first) |
| TinyLlama-ov curated despite 2048-token ceiling + poor output | openvino-bench-findings: rejects long prompts by design, "output quality is poor: echoes or continues the prompt". Curated for what user? | med |
| `model list` CTX advertises planner ceiling, not certified/reachable context | P2.7 from the capacity inventory, still open. | med |
| Alias table over-matches | `curated.go:255-273`: substring "gemma" → gemma4-e4b (unservable!), "deepseek" → r1-0528, "qwen3" → qwen3-8b. Fuzzy aliases route users to wrong/broken models. | med |
| darwin/windows modeld story | source-build doc: "Linux path verified end-to-end; darwin/windows native build chain still needs porting" while install.sh/setup present a turnkey story. | med |

### Class 2 — surfaced-but-confusing (each personally hit this week)

- Onboarding health step shows the dormant openvino backend as ✗-error on a
  perfectly healthy single-engine setup.
- Chat narrates tool calls it never made when no toolchain is attached (BUG-6,
  open; chain declares `tools:['*']` + tool-assuming instructions).
- Setup-blocked banner persists on stale cache after CLI-side fixes (fixed:
  refresh button added).
- Capacity panel showed three inconsistent context numbers for one resident
  model (fixed in capacity inventory work).
- `--input @file` documented, was broken (fixed).

### Class 3 — maintenance surface disproportionate to evidence of use

- **10+ backend types** (llama, openvino, ollama, vllm, openai, openrouter,
  anthropic, mistral, bedrock, gemini, vertex-google) — each carries provider
  code, catalog, prompt shaping, tool-protocol handling, upstream-API drift
  risk. bedrock/vertex can't even infer URLs and need cloud-specific auth.
- **5 frontends**: CLI, Beam web UI, VS Code extension, ACP (`acp` + headless
  `acpx`), Ollama/OpenAI compat HTTP APIs. Features (sessions, chains, HITL,
  model management) must be mirrored across all of them.
- **26 curated models × 2 local backends** with fuzzy alias tables.
- **OpenVINO backend**: large share of modeld's native complexity (allocator
  poisoning marked fatal, NPU rejection paths, XAttention dense-retry,
  "Windows packaging needs a checked-in reproducible path" per bench doc) for
  an Intel-AI-PC niche.
- Long tail: MCP servers, remote tools/hooks, LoRA adapters, model groups,
  embeddings, tensor-split multi-GPU, HITL policies, session fork/compaction,
  benchreport, cache subcommand.

## E2E test program (fix-or-file each; update this doc per row)

Walk every advertised surface exactly as a user would. Order by blast radius:

1. **CLI golden paths** (`setup` → `model pull` → chat/run/session/doctor) on
   this machine — llama backend. Includes every example command printed by
   `contenox --help` and each subcommand's help (69 flag/command surfaces).
2. **Curated registry sweep**: for each curated model that fits this host
   (≤8 GB class): pull → describe → 1 chat turn. Catch gemma4-class
   unservables mechanically (describe/load must fail-fast with a clear error,
   or the entry gets dropped).
3. **Backend add matrix**: every advertised `--type` with a fake/real key —
   at minimum validation coherence (openrouter!), URL inference, doctor
   diagnostics.
4. **Beam UI walk**: onboarding wizard fresh-state, chat with/without
   toolchain, backends page, model registry page, capacity panel, HITL.
5. **ACP + VS Code bridge**: handshake + one session each; autocomplete FIM
   path with the separate autocomplete model config.
6. **Compat APIs**: /api/chat (Ollama shape) + /v1/chat/completions (OpenAI
   shape) against contenox serve, including one nested contenox-as-backend.
7. **OpenVINO cell** (decision-gated): only if the keep decision is made —
   CPU device on this host, curated -ov model, same golden path.

## Niche-down analysis (decision needed)

The product's stated differentiator (README): governed, reviewable Chains
around an agent loop + local-first inference with honest capacity. Everything
else is reach. Candidates, with the maintenance win:

| Drop/demote candidate | Win | Risk |
|---|---|---|
| bedrock + vertex-google first-class support → "openai-compatible URL" escape hatch only | two cloud-auth stacks, URL ceremony, API drift, test matrix rows | enterprise checkbox loss |
| mistral + vllm as named types → alias to openai-compatible | provider-code dedupe; vllm IS openai-compatible | version-specific quirks |
| Curated registry → only benchmark-certified models (capability-truth rule: capability output must be servability output) | no unservable downloads; alias table shrinks; registry becomes a quality signal instead of a listing | fewer names in the picker |
| gemma4 entries out until llama.cpp pin supports the arch | removes the worst live footgun | none |
| Beam UI → admin/setup console only (chat demo clearly labeled), or freeze | the single largest mirrored-feature surface | demo appeal |
| OpenVINO → certified-cell-only (2-3 models, CPU+iGPU, Linux) or experimental flag | biggest native-code maintenance cut after llama.cpp itself | Intel AI-PC strategy |
| LoRA adapters, model groups, remote hooks → "experimental" label + no default surfacing | sets user expectations honestly; frees them from the E2E gate | power users |

Non-candidates (core, invest): CLI, chains/taskengine, modeld llama path,
capacity truth, sessions/state, ACP (it's the agentic integration story),
Ollama-daemon backend (the pragmatic on-ramp).

## E2E round 1 results (2026-07-02, this host, live)

**WORKS** (verified end-to-end):
- doctor; model list / local / registry-list; session list; state; cache;
  config list; mcp list; tools list; version.
- `model pull granite-3.2-2b` → auto-resolved open (arch=granite supported) →
  chat completion. Second model/architecture E2E confirmed.
- `contenox serve` boots, Beam HTML serves on /.
- llama chat path incl. multi-turn context growth (see capacity inventory).

**BROKEN** (reproduced, exact commands in transcript):
- `backend add openrouter --type openrouter` — the exact command on the CLI
  front-page help — rejected by `backendservice.go:80` validation ("Type must
  be ollama, vllm, …"). CLI infers the URL for a type the service refuses.
- **The entire Ollama/OpenAI compat API is dead code.** `AddOllamaRoutes`,
  `AddOpenAIRoutes`, `AddRootRoutes` (runtime/internal/compatapi) have ZERO
  callers. Live server: `POST /v1/chat/completions` → returns Beam HTML (200!),
  `POST /api/chat` → 405, `GET /api/tags` → "serverops: not found" (a
  different /api handler), `GET /v1/models` → Beam HTML. Any OpenAI/Ollama
  client pointed at contenox serve gets a webpage or an error.
- BUG-6 reproduced in plain CLI: `contenox chat` with NO tools attached,
  model (granite-2b) claims "I've confirmed the echo pong command executed
  successfully". Fabricated execution; small models worst.

**WEIRD**:
- `qwen3-4b` — the model doctor recommends and the only one proven working —
  is the ONLY registry entry marked NOT curated (`-`); the unservable gemma4
  entries all show curated ✓.
- Session workspace scoping is invisible: sessions created from another cwd
  vanish from `session list` with no hint that a workspace filter applies.
- `--model granite-3.2-2b` flag on chat: worked, but the turn continued the
  active session's history from a different model — model switching
  mid-session has no guardrail/notice.

**NOT YET TESTED**: acp/acpx handshake, vscode-agent bridge, autocomplete/FIM
path, gemma4 pull fail-fast behavior, backend add for remaining cloud types,
Beam interactive walk, HITL, embeddings, session fork/compact, update.

## modeld model-schema audit (round 2, 2026-07-02 — the "worry list")

Focus: per-model tool schemas, thinking schemas, hardware special-casing,
hardcoded/unimplemented-with-a-note gaps inside modeld and its protocol edges.

**VERIFIED WORKING (live on this host):**
- Tool-call schema E2E on llama/qwen3-4b: model emitted a structured call,
  `llama:common_chat_tool_parser` parsed it, HITL approval gate fired and a
  denial was honestly reported by the model; with `--auto` the command executed
  and the answer was grounded in real output. The CLI also prints an honest
  WARN when local_shell runs with no HITL and no allowed-dir.
- `--think off/high` boolean genuinely toggles template thinking on qwen3-4b
  (off → clean answer, high → reasoning block).
- granite arch loads/serves (second architecture verified).

**HOLLOW SURFACE (advertised precision that does not exist):**
- The 7-level think knob (`auto/off/minimal/low/medium/high/xhigh`, CLI +
  vscode + config, default "high") collapses to a BOOLEAN for every local
  model (`runtime/modelrepo/llama/client.go:397-405`: anything ≠ off/auto →
  EnableThinking=true). Levels are only real for cloud providers. No warning
  is given. Worst case: gpt-oss-20b — a curated local model whose signature
  feature IS effort levels — gets the boolean.

**UNIMPLEMENTED BUT CURATED ANYWAY:**
- gpt-oss-20b (12 GB curated download): no reasoning protocol declared, no
  harmony-channel parsing anywhere in modeld, no reasoning_effort plumbing
  (`grep reasoning_effort|harmony` = zero hits). Its two signature features
  are unreachable; raw harmony channel markers likely leak into chat output
  (untested — needs a pull+chat run on a bigger host).
- gemma4 ×4: unservable by the pinned llama.cpp (see round 1).

**BLANKET ASSIGNMENTS (plausible but unverified per model):**
- Every llama curated model gets `llama:common_chat_tool_parser`; every -ov
  model gets `openvino:json_schema_tool_calls` (except tinyllama: none).
  Delegating to llama.cpp's template-aware common_chat is reasonable, but only
  qwen3 is verified; granite/phi/deepseek templates untested for tool calls.
- deepseek reasoning format declared for qwen3+deepseek families — matches the
  observed qwen3 reasoning block; deepseek-r1 itself untested.

**HARDWARE SPECIAL-CASING FOUND:**
- OpenVINO XAttention detection is error-STRING sniffing:
  `strings.Contains(err.Error(), "XAttention is not supported")`
  (`modeld/openvino/service.go:262`) — brittle against upstream wording.
- "disabling CUDA graphs due to GPU architecture" spams per-token on Turing
  (upstream llama.cpp; cosmetic but floods logs).
- Overflow errors suggest "enable q8_0 KV cache", but KVCacheType is only
  settable via a hand-written per-model `contenox-llama.json` — no CLI flag,
  no config key. Advice the user cannot follow with shipped knobs.
- Skip-note grep over modeld came back nearly clean (3 hits, benign) — the
  gaps above live in protocol tables and missing plumbing, not TODO comments.

## Progress log

- 2026-07-02: doc created. Evidence gathered for Class 1-3. E2E program
  defined. Niche-down framing revised after review: tier promises, don't
  delete working code; only remove what lies (user pushed back correctly —
  and round 1 proved the point: the compat API I'd assumed working from code
  reading was never even mounted).
- 2026-07-02 (round 1): CLI surface swept live. Two major broken promises
  found (openrouter type, whole compat API), one dead-code package, several
  UX weirdnesses. granite E2E works. modeld + serve left running for round 2.

# Plan: Fix the three Vertex partner providers (anthropic, meta, mistralai)

Status: IMPLEMENTED — build/vet/tests green. Pending manual real-account test.

## Direct providers (standalone Anthropic + Mistral) — DELIVERED

New packages reusing the shared codecs, only transport differs:
- `runtime/modelrepo/anthropic/` — direct `api.anthropic.com`: `x-api-key` +
  `anthropic-version: 2023-06-01` header, model in body, `/v1/messages`. Wraps
  the `messages` codec. httptest round-trip test included.
- `runtime/modelrepo/mistral/` — direct `api.mistral.ai/v1`: Bearer auth, model
  in body, `/chat/completions`. Wraps the `chatcompletions` codec. httptest
  round-trip test included.

Full wiring for the two NEW backend types (`anthropic`, `mistral`):
- `runtimestate/catalogimports.go` — blank imports (registers catalogs)
- `runtimestate/state.go` — dispatch (`processOpenAIBackend`, generic over type)
- `runtimestate/provider.go` — `AnthropicKey` / `MistralKey` config keys
- `runtimestate/catalogstate.go` — `providerConfigKey` cases
- `backendservice/backendservice.go` — type validation
- `contenoxcli/backend_cmd.go` — default URL inference + `--type` help
- `contenoxcli/init.go` — `providerConfigs` (display name, default model, env key)
- `internal/setupcheck/setupcheck.go` — display name, fix-path, add/repair/diagnostic
  commands, no-models guidance, API-key hints

Verified: full `go build ./...`, `go vet`, and all affected package tests pass
(modelrepo incl. new providers, backendservice, runtimestate-adjacent, setupcheck,
contenoxcli). Live API calls remain your manual test. Test matrix is now **8**:
the 6 above plus `anthropic` and `mistral` direct.

## Implementation summary (what landed — Vertex partner fix)

Shared, transport-agnostic codec packages (decision §8.1):
- `runtime/modelrepo/codec/chatcompletions/` — OpenAI Chat Completions codec
  (request build w/ tool-name sanitization, response decode, SSE stream decoder
  w/ per-index tool-arg assembly). Tests included. Powers vertex-meta + vertex-mistralai.
- `runtime/modelrepo/codec/messages/` — Anthropic Messages codec (system lift,
  content blocks, tool_use/tool_result, named-SSE-event stream decoder w/
  input_json_delta assembly). Tests included. Powers vertex-anthropic.

Vertex package (publisher-aware dispatch; vertex-google path unchanged):
- `families.go` — publisher → family mapping + endpoint/body-model policy
  (anthropic/mistral → :rawPredict/:streamRawPredict; meta → openapi
  chat/completions; meta body model publisher-prefixed).
- `client.go` — added `postJSON` (raw bytes) + `openStream` + `bearer`/`authHeaders`;
  `sendRequest` now thin wrapper (Gemini behavior unchanged).
- `chat.go` / `stream.go` — dispatch by family; existing Gemini logic moved into
  `chatGemini`/`streamGemini` verbatim.
- `codec_anthropic.go` / `codec_openai.go` — the new chat+stream paths + a shared
  SSE loop (`streamSSE`).
- `catalog.go` — partner models now default CanChat/CanStream/CanPrompt=true (§6).

Tool calling: full on the non-streaming `Chat` path (both codecs decode tool
calls). Streaming surfaces text/thinking only — parity with the existing
Gemini/OpenAI stream clients, whose `StreamParcel` has no tool-call field; the
engine sources tool calls from non-streaming `Chat`. Not a regression, not a cut
relative to how tools already work.

NOTE: the "stream token refetch" item from the original screening was a
misread — `c.tokenFn` is already the provider's cached (`tokenOnce`) source;
no change made.

---

## Original plan (for reference)

Status: draft for review. Implementation not started.

## 1. Problem

`vertex-anthropic`, `vertex-meta`, and `vertex-mistralai` are accepted backend
types, list models correctly, but **cannot chat/stream/prompt**. The Vertex
chat/stream/prompt clients hardcode Gemini's wire format for every publisher:

- `vertex/client.go:28` (`endpoint`) builds `.../publishers/{publisher}/models/{model}:{method}`.
- `vertex/chat.go:28` always calls `endpoint("generateContent")` with a Gemini-shaped body (`vertexRequest`: `contents`/`parts`/`systemInstruction`/`functionDeclarations`).
- `vertex/stream.go:38` always calls `endpoint("streamGenerateContent")`.
- The response is parsed as Gemini `candidates[].content.parts` (`vertex/types.go:62`).

`generateContent` is a Gemini-only verb/schema. The three partners require
different endpoints **and** different request/response/stream schemas (verified
against Google + vendor docs — see §3). Net: inference for the three partners
fails at the transport+codec layer.

## 2. What is NOT broken (do not touch)

The wiring screen confirmed every provider-type site already handles all four
`vertex-*` types. The fix needs **no** changes to:

- `backendservice.go:80` (validation) — all four already valid.
- `runtimestate/state.go:294` — all four already route to `processVertexBackend`.
- `runtimestate/catalogstate.go` + `provider.go` — config keys exist for all four.
- `vertex/catalog.go:16-21` — all four `RegisterCatalogProvider` calls exist; listing works.
- `internal/setupcheck/setupcheck.go` — display names, hints, repair/diagnostic commands already cover all four.
- `contenoxcli/backend_cmd.go`, `init.go` — help text + provider config already list all four.
- `taskengine/llmretry/retry.go` — provider-agnostic (string match on 429/529/etc.); already covers Anthropic overload.

`GetType()` returning `"vertex-"+publisher` is consumed by `llmrepo` as
`ProviderType` (telemetry/routing) and asserted in tests — **leave as-is**.

**Conclusion:** the fix is contained inside `runtime/modelrepo/vertex/`.

## 3. Verified specs (Vertex)

Common: regional host `https://LOCATION-aiplatform.googleapis.com`, base URL
already carries `/v1/projects/{P}/locations/{L}`. Auth = `Authorization: Bearer
<OAuth2>` (already implemented in `vertex/auth.go`), optional
`x-goog-user-project` (already set from URL). `Content-Type: application/json`.

### (A) Anthropic — Messages schema, `rawPredict`
- Endpoint: `{base}/publishers/anthropic/models/{model}:rawPredict` (stream: `:streamRawPredict`).
- Body: Anthropic Messages — `messages`, `max_tokens` (**required**), optional `system` (top-level), `temperature`, `top_p`, `tools`, `tool_choice`, `stream`. **`anthropic_version: "vertex-2023-10-16"` in body.** Model NOT in body (it's in URL).
- Response: `content` = block array — `{type:"text",text}`, `{type:"tool_use",id,name,input}`. `stop_reason`. `usage`.
- Stream (named SSE events): `message_start` → per block `content_block_start` + N×`content_block_delta` + `content_block_stop` → `message_delta` → `message_stop`. Text: `delta.type=="text_delta"` → `.text`. Tool calls: `content_block_start` carries `tool_use{id,name}`, then `delta.type=="input_json_delta"` → concat `partial_json` per `index`, parse at `content_block_stop`.
- Tools: `tools:[{name,description,input_schema:{type:object,properties,required}}]`; `tool_choice:{type:auto|any|none|tool}`. tool_result returned as a `user` message with `content:[{type:"tool_result",tool_use_id,content}]`.

### (B) Mistral — OpenAI-ish schema, `rawPredict`
- Endpoint: `{base}/publishers/mistralai/models/{model}:rawPredict` (stream: `:streamRawPredict`). Model often carries `@version`.
- Body: `{model, messages, max_tokens, stream, temperature?, top_p?, tools?, tool_choice?}`. **Model duplicated in body.**
- Response: OpenAI shape — `choices[].message{content,tool_calls[]}`, `finish_reason`, `usage`.
- Stream: SSE `data: {chunk}` … `data: [DONE]`; OpenAI delta assembly.

### (C) Meta/Llama (and other open MaaS) — OpenAI-compatible, openapi path
- Endpoint: `{base}/endpoints/openapi/chat/completions` — **NOT** under `/publishers/.../models/`, **NOT** a `:method` verb. Streaming via `"stream":true` in body (same path).
- Body: OpenAI chat — `model` in body as `{publisher}/{model}` (e.g. `meta/llama-3.3-70b-instruct-maas`), `messages`, `max_tokens`, `stream`, `tools`, `tool_choice:"auto"` (no `"required"`/named on Llama). GCP extras under `extra_body`.
- Response/stream: OpenAI shape, same as Mistral.

Two codecs cover all three: **Anthropic Messages** (A) and **OpenAI Chat
Completions** (B + C). Transport differs along two axes: endpoint construction
(verb/path) and whether streaming is a separate path (`:streamRawPredict`) or a
body flag (`stream:true`, openapi path).

## 4. Design — codec/transport seam inside the vertex package

Introduce a per-publisher **profile** = `{codec, endpoint policy}`. Keep
`vertex-google` on the existing Gemini path unchanged (zero regression risk).

```
type vertexCodec interface {
    // build the request body from neutral messages + config.
    BuildRequest(model string, msgs []modelrepo.Message, cfg *modelrepo.ChatConfig, streaming bool) (any, error)
    // parse a non-streaming response body into a ChatResult.
    ParseResponse(raw []byte) (modelrepo.ChatResult, error)
    // parse one SSE `data:` line into incremental output (text/thinking/tool-call deltas).
    ParseStreamLine(line []byte, st *streamState) (*modelrepo.StreamParcel, error)
}

type vertexProfile struct {
    codec        vertexCodec
    endpoint     func(base, model string, streaming bool) string // verb/path policy
    modelInBody  bool   // mistral/meta: yes; anthropic: no; gemini: no
}
```

- `geminiCodec` — wrap the existing `buildVertexRequest`/`vertexResponse` logic (already present; just move behind the interface). Endpoint: `:generateContent` / `:streamGenerateContent?alt=sse`.
- `anthropicCodec` — new. Messages body (`anthropic_version`, `system`, `messages`, `max_tokens`, `tools`, `tool_choice`); content-block response; named-SSE-event stream parser with per-`index` `input_json_delta` accumulation. Endpoint: `:rawPredict` / `:streamRawPredict`. `modelInBody=false`.
- `openaiCompatCodec` — new. Chat-completions body/response/SSE (`choices`/`tool_calls`, `data:`/`[DONE]`, per-`index` argument-fragment accumulation). Used by both mistral and meta. `modelInBody=true`.
  - mistral profile endpoint: `:rawPredict` / `:streamRawPredict` under `/publishers/mistralai/models/{model}`.
  - meta profile endpoint: `{base}/endpoints/openapi/chat/completions` (ignore model-in-URL); model in body as `meta/{model}`; streaming = body flag (no path change).

Profile is selected by `c.publisher` (already on `vertexClient`/`vertexProvider`).
`chat.go`/`stream.go`/`prompt.go` become publisher-agnostic: pick profile →
build via codec → POST to `profile.endpoint(...)` → parse via codec.

### Tool-call name handling
Reuse the existing schema sanitization for Gemini; the OpenAI-compat and
Anthropic codecs need their own (OpenAI tolerates dots? — confirm; Anthropic
`input_schema` is plain JSON Schema). Keep neutral `modelrepo.ToolCall`
(OpenAI-shaped) as the canonical form; each codec maps to/from it. Anthropic
`tool_use`→`ToolCall` and `ToolCall`/tool-result→Anthropic blocks is the most
intricate mapping; isolate and unit-test it.

## 5. File-by-file changes (all under `runtime/modelrepo/vertex/`)

Modify:
- `client.go` — replace single `buildVertexRequest`+`endpoint("generateContent")` assumption with profile selection; move existing Gemini logic into `geminiCodec`.
- `chat.go` — call `profile.codec.ParseResponse`; POST to `profile.endpoint(base, model, false)`.
- `stream.go` — POST to `profile.endpoint(base, model, true)`; drive `profile.codec.ParseStreamLine`; **also fix** the incidental token bug (uses inline `BearerTokenWithCreds` instead of cached `c.tokenFn`, `stream.go:62-77`).
- `prompt.go` — **confirmed** it wraps `vertexChatClient.Chat` (`vertex/prompt.go`), so it's fixed automatically once Chat is publisher-aware. No direct changes.
- `catalog.go` — §6 capability enrichment for `publisherCatalogProvider.ListModels` (default chat caps); update stale comment. For `meta`, note the bare model name from `listVertexPublisherModelNames` must be sent in the body as `meta/{model}` (often `...-maas`) — the openapi path needs the publisher-prefixed form; make this transform explicit in the meta profile.
- `provider.go` — wire the profile/publisher into the clients it builds.

Add:
- `codec_gemini.go` — `geminiCodec` (extracted existing logic).
- `codec_anthropic.go` + `types_anthropic.go` — Messages request/response/stream + mapping.
- `codec_openai.go` + `types_openai.go` — chat/completions request/response/stream + mapping.
- `profiles.go` — `publisherProfile(publisher)` selector + endpoint builders.
- Tests: `codec_anthropic_test.go`, `codec_openai_test.go` (golden request/response + stream assembly), `endpoint_test.go` (per-publisher URL/verb), mirroring existing `chat_test.go`/`stream_test.go` patterns.

No changes outside the package are required for the fix.

## 6. Capability handling — MUST fix, else the fix is unusable

Partner catalogs return models with **all capability flags false,
ContextLength=0** (`vertex/catalog.go:233-235`), with a comment saying users
must "declare models via `contenox model register`". **That command does not
exist.** `model_cmd.go` exposes only `list` and `set-context`;
`model_registry_cmd.go` is GGUF-oriented (`--url`/`--size`). There is no CLI
path to set `CanChat`. So a listed partner model can never become
chat-eligible — the codec fix alone would still be unusable.

Fix: enrich partner publisher models with sensible default capabilities in the
catalog, exactly as the other providers already do:
- `vertex-google` → `enrichGooglePublisherModel` defaults `CanChat/CanPrompt/CanStream=true` (`vertex/catalog.go:72-87`).
- `vllm` → all chat caps true (`vllm/catalog.go:70-79`).
- `local` → all caps true (`local/catalog.go:40-45`).

Claude / Mistral / Llama are all chat models, so `publisherCatalogProvider.ListModels`
(`vertex/catalog.go:220-238`) should set `CanChat=true, CanStream=true,
CanPrompt=true` (CanEmbed=false; CanThink only where supported — leave false for
now). ContextLength can default to 0 and be set per-model via
`contenox model set-context`. Update the stale "model register" comment.

This removes the only remaining blocker to the manual test and matches existing
provider behavior — it is part of "fix properly," not scope creep.

## 7. Manual test matrix (the 6 the user will run with real accounts)

For each: `backend add` → `model list` (capabilities now default-on per §6) →
run a chat prompt + a streaming prompt + a tool-calling prompt.

**Prerequisite that will otherwise cause false 404s:** the partner models are
served only in *specific regions* and frequently must be enabled in Vertex
Model Garden first. **Do not reuse the Gemini `us-central1` URL for the
partners.** Each `vertex-*` backend's `--url` must point at a region where that
publisher's chosen model is actually offered, or you'll get `NOT_FOUND` that
looks exactly like a codec bug. Confirm per-publisher region + model
enablement before concluding anything about the codec.

| Type | URL (region matters) | Auth | Model placement | Streaming | Expect |
|---|---|---|---|---|---|
| `vertex-google` | `.../v1/projects/P/locations/L` (Gemini region) | ADC/SA | URL | `:streamGenerateContent` | unchanged (regression check) |
| `vertex-anthropic` | Claude-enabled region (e.g. `us-east5`) | ADC/SA | URL | `:streamRawPredict` | Messages codec |
| `vertex-mistralai` | Mistral-enabled region (e.g. `europe-west4`) | ADC/SA | URL + body | `:streamRawPredict` | openai-compat codec |
| `vertex-meta` | Llama-MaaS region | ADC/SA | body (`meta/...-maas`) | `stream:true`, openapi path | openai-compat codec |
| `openai` | default | API key | body | flag | unchanged |
| `gemini` | default | API key | URL | `:streamGenerateContent` | unchanged |

(`ollama`/`vllm`/`local` are untouched; include if "6" maps differently.)

## 8. Scope decisions — DECIDED

1. **Codec placement → EXTRACT SHARED PACKAGES NOW.** Create transport-agnostic
   codec packages (`runtime/modelrepo/codec/chatcompletions`,
   `runtime/modelrepo/codec/messages`) as the canonical implementations; the new
   Vertex partner paths consume them, and a future direct `anthropic`/`mistral`
   provider reuses them with only a different transport.
   **Guardrail:** do NOT migrate the existing `openai`/`gemini` packages onto the
   shared codecs in this pass — they are working providers under manual test and
   migrating them is separate risk. The shared codecs are designed so they *can*
   adopt them later. `vertex-meta` (pure chat-completions) proves the
   `chatcompletions` codec is genuinely reusable.
2. **Tool calling depth.** Include full tool-calling (Anthropic `tool_use` +
   OpenAI `tool_calls`) in this pass? It's the riskiest/most intricate part.
   **Recommendation: yes** ("fix properly"), but build it as a *separable second
   step within the same implementation*: land text + streaming for all three
   first (so you can confirm basic chat on real accounts), then add tool-calling
   additively. Anthropic's `input_json_delta` assembly and
   tool_result-as-user-message mapping is the riskiest surface; keeping it
   additive means a tool-calling bug can't mask working basic chat during the
   manual pass.
3. **Embeddings.** Out of scope (Anthropic has none; Mistral embed via
   `rawPredict` could be a follow-up). Leave `GetEmbedConnection` unsupported.
4. **`tool_choice` enums / Llama limitations** (no `required`/named tool calls)
   — pass through what the model supports; don't emulate.

## 9. Risks

- **Regression on `vertex-google`:** mitigated by moving (not rewriting) the
  Gemini logic behind a codec and keeping its endpoint/verb identical; covered
  by existing `vertex` tests.
- **Anthropic streaming assembly** (named events, `input_json_delta`) is the
  trickiest; unit-test against recorded event sequences.
- **Meta openapi URL** derivation differs from the `/publishers/...` pattern;
  endpoint builder must special-case it and the `meta/{model}-maas` body name.
- **Auth/quota** (`x-goog-user-project`, regional vs global host) — already
  handled in transport; verify for `global` location if used.

## 10. Future unlock (not in this task)

Because partner endpoints are the vendors' native schemas, the
`anthropicCodec` and `openaiCompatCodec` are the same codecs a **direct**
`anthropic` / `mistral` provider would need — differing only in transport
(api-key header + vendor base URL instead of OAuth + Vertex URL). After this
fix, adding direct providers is a thin transport wrapper around the extracted
codecs. `vertex-meta`'s codec is already what the `openai` package implements,
so a future consolidation is possible but optional.

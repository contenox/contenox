# Model Provider Thinking Detection Research

Date: 2026-06-05

This note records which provider catalog protocols expose thinking/reasoning capability metadata that Contenox can detect at runtime. "Detect" means the provider/backend model-list or model-detail protocol returns an explicit field for thinking/reasoning support. Public model names, static docs, and local family lists are not detection.

Manual provider/model overrides are separate from detection. Operators can set them with `contenox model capability set <provider> <model> --think true|false` or ACP `/capability set ...`; runtime state applies those overrides after catalog/cache data and before executable providers are constructed.

## Detection matrix

| Provider | Catalog protocol in this repo | Thinking detection status | Implementation decision |
| --- | --- | --- | --- |
| OpenAI | `GET /v1/models` | Not detected. The model object/list API exposes IDs and generic model metadata, while reasoning support is controlled at request time by parameters such as `reasoning_effort`. No model-list capability field was found. | Do not set `CanThink` from model names. OpenAI request builders send reasoning fields only when the effective runtime capability has `CanThink=true`, usually through a manual override. |
| Anthropic | `GET /v1/models` | Detected when model metadata includes explicit `capabilities.thinking` or `capabilities.effort` support. Missing capability metadata is treated as unknown, not inferred from the Claude model name. | Call the stable model list; set `CanThink` only from capability fields; preserve explicit `CapabilityConfig.CanThink` through `ProviderFor`. |
| Gemini API | `GET /v1beta/models`, then `GET /v1beta/{model}` | Detected. The Model resource includes a `thinking` boolean alongside token limits and supported generation methods. | Decode `thinking` and copy it to `CanThink`. |
| Vertex Google | `GET /v1beta1/publishers/google/models` | Not detected from the publisher list used here. The list response is useful for publisher model IDs but not Gemini thinking support. | Do not set `CanThink` from Gemini-family model names. User/admin capability config can still set it explicitly. |
| Ollama | `GET /api/tags`, then `POST /api/show` | Not detected. `/api/show` exposes standard capabilities such as completion, embedding, and tools plus model info/template metadata; the public thinking docs identify supported model families but that is static documentation, not backend metadata. | Do not set `CanThink` from family names. Preserve explicit `CapabilityConfig.CanThink`. |
| vLLM | `GET /v1/models` | Not detected. The OpenAI-compatible model list exposes model IDs and server metadata such as `max_model_len`; reasoning parser/chat-template settings are server configuration, not advertised as a model capability in the model list. | Do not set `CanThink` from family names. Preserve explicit `CapabilityConfig.CanThink`. |
| Mistral | `GET /models` | Not detected in the reviewed catalog surface. The current repo catalog observes model IDs and coarse chat/embed capability only; no verified model-list thinking-capability field is consumed. | Do not set `CanThink` from model names such as `magistral`; preserve explicit `CapabilityConfig.CanThink`. |
| Bedrock | curated list today; AWS API alternative is `ListFoundationModels` | Not detected. Bedrock foundation-model summaries expose modalities, streaming, customizations, inference types, and lifecycle, not provider-specific thinking controls. The current repo catalog is a static Converse-capable list. | Do not set `CanThink` from Bedrock model IDs; preserve explicit `CapabilityConfig.CanThink`. |
| Local llama.cpp | filesystem directory scan for subdirectories containing `model.gguf`; execution uses in-process `github.com/ollama/ollama/llama` clients | Not detected. The local catalog currently observes model directories and file presence only; it does not parse GGUF metadata, chat templates, or runtime traces into thinking capability metadata. | Do not set `CanThink` from local model names such as `FastThink`; preserve explicit `CapabilityConfig.CanThink`. |

## Primary sources reviewed

- OpenAI Models API and Chat Completions request parameters: https://platform.openai.com/docs/api-reference/models/list and https://platform.openai.com/docs/api-reference/chat/create
- Anthropic Models API and generated SDK model-capability schema: https://docs.anthropic.com/en/api/models-list and https://github.com/anthropics/anthropic-sdk-typescript/blob/main/src/resources/models.ts
- Gemini API Model resource and thinking configuration docs: https://ai.google.dev/api/models and https://ai.google.dev/gemini-api/docs/thinking
- Vertex AI publisher models REST API: https://cloud.google.com/vertex-ai/docs/reference/rest/v1beta1/publishers.models
- Ollama API and thinking capability docs: https://docs.ollama.com/api and https://docs.ollama.com/capabilities/thinking
- vLLM OpenAI-compatible server docs: https://docs.vllm.ai/en/latest/serving/openai_compatible_server.html
- Mistral API spec: https://docs.mistral.ai/api/
- Amazon Bedrock ListFoundationModels API: https://docs.aws.amazon.com/bedrock/latest/APIReference/API_ListFoundationModels.html
- Local llama.cpp integration in this repo: `runtime/modelrepo/local/catalog.go`, `runtime/modelrepo/local/provider.go`, and `runtime/modelrepo/local/client.go`

## Follow-up criteria

A provider should set `CanThink=true` only when one of these is true:

1. A live catalog/model-detail protocol returns an explicit thinking/reasoning capability field.
2. A local/admin override explicitly sets the effective provider/model `CanThink` value, for example with `contenox model capability set <provider> <model> --think true|false`.
3. A future provider implementation performs an active capability probe and records the result as observed metadata.

Static public model lists and name-pattern families should not update `CanThink`.

Provider-created clients gate request-time thinking controls on the effective `CanThink` value. Low-level encoding helpers still map normalized `WithThink` levels for tests and explicit internal use, but runtime chat/stream clients omit thinking request fields when the provider capability is false.

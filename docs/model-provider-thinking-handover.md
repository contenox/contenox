# Handover: Model Provider Thinking Capability Detection

Use this prompt if the session rolls over before the provider-capability work is complete:

```
Continue in /home/naro/src/github.com/contenox/runtime. The user wants thinking/reasoning controls to be driven by effective model capability, not model-name guesses.

Current contract:
- Public model names, static supported-model docs, and hard-coded family lists are not runtime detection.
- Provider catalog/model-detail metadata can set `CanThink` only when it exposes an explicit thinking/reasoning capability field.
- Manual provider/model overrides are persisted in KV with keys shaped as `model-capability:<provider>:<model>`.
- CLI: `contenox model capability set|show|unset <provider> <model> --think true|false`.
- ACP: `/capability set|show|unset <provider> <model> --think true|false`.
- Runtime state applies manual overrides after catalog/cache data and before `PulledModels` are stored. Executable providers inherit the effective capability through the existing runtime adapter.
- Request-time `WithThink` controls are gated on effective `CanThink`; when false, provider clients omit thinking/reasoning request fields.

Provider state:
- OpenAI `/v1/models` does not advertise reasoning capability. OpenAI sends `reasoning_effort` only when effective `CanThink=true`, typically from a manual override.
- Anthropic calls stable `/v1/models` and parses `capabilities.thinking` / `capabilities.effort`; missing capability metadata does not infer from Claude names.
- Gemini API detection is tied to the explicit Model resource `thinking` boolean; Gemini model names alone do not set `CanThink`.
- Vertex publisher list, Ollama `/api/tags` + `/api/show`, and vLLM `/v1/models` do not advertise thinking support in the currently implemented catalog paths, so they do not infer from model families.
- Mistral and Bedrock do not detect thinking from their current catalog paths and preserve explicit/effective `CanThink`.
- Local llama.cpp is integrated through filesystem discovery of subdirectories containing `model.gguf` and in-process `github.com/ollama/ollama/llama` clients; local catalog names such as `FastThink` do not set `CanThink`.

Useful verification:
- Focused tests should cover `runtime/modelcapability`, `runtime/runtimestate`, `runtime/contenoxcli`, `runtime/acpsvc`, and affected provider packages.
- Before declaring done, run focused `go test` for the affected packages and inspect `git diff`.
```

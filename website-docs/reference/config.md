# Configuration

Contenox stores all configuration in SQLite (`.contenox/local.db`, or `~/.contenox/local.db` globally).
There is no YAML file — register backends and set defaults using CLI commands.

## Register a backend

```bash
# Local Ollama (base URL inferred automatically)
contenox backend add local --type ollama

# OpenAI (base URL inferred)
contenox backend add openai --type openai --api-key-env OPENAI_API_KEY

# Google Gemini
contenox backend add gemini --type gemini --api-key-env GEMINI_API_KEY

# Self-hosted vLLM or compatible endpoint
contenox backend add myvllm --type vllm --url http://gpu-host:8000
```

## Set persistent defaults

```bash
contenox config set default-model    qwen2.5:7b
contenox config set default-provider ollama
contenox config set default-chain    .contenox/default-chain.json

contenox config list   # review current settings
```

## Manage backends

```bash
contenox backend list
contenox backend show openai
contenox backend remove myvllm
```

## Supported providers

| `--type` | Notes |
|---|---|
| `ollama` | Local. Run `ollama serve` first. |
| `openai` | Use `--api-key-env OPENAI_API_KEY`. Base URL inferred. |
| `gemini` | Use `--api-key-env GEMINI_API_KEY`. Base URL inferred. |
| `vllm`   | Self-hosted OpenAI-compatible endpoint. Requires `--url`. |

## Database location

Contenox resolves the database path in this order:
1. `--db <path>` flag
2. `.contenox/local.db` in the current working directory
3. `~/.contenox/local.db` (global fallback)


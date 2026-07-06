---
title: OpenRouter
description: Connect Contenox to OpenRouter's multi-provider model gateway.
---

# OpenRouter

OpenRouter gives Contenox one API key for many hosted models. Use it when you want to switch between model families without registering each upstream provider separately.

## Setup

```bash
export OPENROUTER_API_KEY=...

contenox backend add openrouter --type openrouter --api-key-env OPENROUTER_API_KEY
contenox config set default-model deepseek/deepseek-chat-v3-5
contenox config set default-provider openrouter

contenox doctor
```

The base URL (`https://openrouter.ai/api/v1`) is inferred; pass `--url` only for a custom compatible gateway.

## Notes

- Browse model ids and pricing at [openrouter.ai/models](https://openrouter.ai/models).
- Model names should be used exactly as OpenRouter reports them, for example `deepseek/deepseek-chat-v3-5`.
- If a model's thinking/tool capability is not advertised correctly, use `contenox model capability set <provider> <model> ...` to add a local override.

## Related

- [OpenAI](/docs/integrations/providers/openai/) — direct OpenAI API
- [Configuration reference](/docs/reference/config/)

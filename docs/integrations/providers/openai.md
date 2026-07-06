---
title: OpenAI
description: Connect Contenox to OpenAI or any OpenAI-compatible endpoint.
---

# OpenAI

Any OpenAI-compatible endpoint works — OpenAI, vLLM, LM Studio, or your own proxy.

```bash
export OPENAI_API_KEY=your-key

contenox backend add openai --type openai --api-key-env OPENAI_API_KEY
contenox config set default-model gpt-4.1-mini
contenox config set default-provider openai
```

For a custom endpoint (vLLM, LM Studio, etc.) add `--url`:

```bash
contenox backend add local-vllm --type openai --url http://localhost:8000/v1 --api-key-env OPENAI_API_KEY
```

## See also

- [Configuration reference](/docs/reference/config/)

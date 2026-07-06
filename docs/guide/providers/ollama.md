---
title: Ollama
description: Connect Contenox to a local Ollama instance or Ollama Cloud.
---

# Ollama

Ollama runs models locally on your machine — no API key, no data leaving your network.

## Local Ollama

Install Ollama from [ollama.com](https://ollama.com), pull a model, then register it:

```bash
ollama pull qwen2.5:7b

contenox backend add local --type ollama
contenox config set default-model qwen2.5:7b
```

## Ollama Cloud

Get an API key at [ollama.com/settings/keys](https://ollama.com/settings/keys), then:

```bash
export OLLAMA_API_KEY=your-key

contenox backend add ollama-cloud --type ollama --url https://ollama.com/api --api-key-env OLLAMA_API_KEY
contenox model list
contenox config set default-model <name-from-list>
contenox config set default-provider ollama
```

## See also

- [Local Models (GGUF)](/docs/guide/local-models/) — no Ollama required, runs directly in Contenox
- [Configuration reference](/docs/reference/config/)

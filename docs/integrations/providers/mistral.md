---
title: Mistral
description: Connect Contenox to Mistral's models via the Mistral API (La Plateforme).
---

# Mistral

Mistral models direct from La Plateforme. Get a key at [console.mistral.ai](https://console.mistral.ai/api-keys/).

```bash
export MISTRAL_API_KEY=your-key

contenox backend add mistral --type mistral --api-key-env MISTRAL_API_KEY
contenox model list                                       # list the Mistral models your key can reach
contenox config set default-model mistral-large-latest    # example — use an id from `model list`
contenox config set default-provider mistral
```

The base URL (`https://api.mistral.ai/v1`) is inferred; pass `--url` only for a proxy or a self-hosted Mistral endpoint.

## See also

- [Configuration reference](/docs/reference/config/)

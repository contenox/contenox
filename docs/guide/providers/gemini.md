---
title: Google Gemini
description: Connect Contenox to Google Gemini via AI Studio or Vertex AI.
---

# Google Gemini

No GPU required. Get a free API key at [aistudio.google.com/apikey](https://aistudio.google.com/app/apikey).

## AI Studio (simplest)

```bash
export GEMINI_API_KEY=your-key

contenox backend add gemini --type gemini --api-key-env GEMINI_API_KEY
contenox config set default-model gemini-flash-latest
contenox config set default-provider gemini
```

## Vertex AI

Want Gemini billed through your GCP project instead of AI Studio? Use the Vertex AI backend. See [Vertex AI](/docs/guide/providers/vertex/) for setup, auth methods, and credential renewal.

## See also

- [Vertex AI](/docs/guide/providers/vertex/) — Gemini hosted on GCP
- [Configuration reference](/docs/reference/config/)

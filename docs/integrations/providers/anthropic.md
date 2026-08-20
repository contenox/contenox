---
title: Anthropic
description: Connect Contenox to Anthropic's Claude models via the Anthropic API.
---

# Anthropic

Claude models direct from the Anthropic API. Get a key at [console.anthropic.com](https://console.anthropic.com/settings/keys).

```bash
export ANTHROPIC_API_KEY=your-key

contenox backend add anthropic --type anthropic --api-key-env ANTHROPIC_API_KEY
contenox model list                                    # list the Claude models your key can reach
contenox config set default-model claude-sonnet-4-5    # example — use an id from `model list`
contenox config set default-provider anthropic
```

The base URL (`https://api.anthropic.com`) is inferred; pass `--url` only for a proxy.

## See also

- [AWS Bedrock](/docs/integrations/providers/bedrock/) — Claude (and others) billed through your AWS account
- [Configuration reference](/docs/reference/config/)

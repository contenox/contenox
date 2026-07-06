---
title: Quickstart
description: Install Contenox and connect a model.
---

# Quickstart

## 1. Install

**macOS / Linux (one line):**

```bash
curl -fsSL https://contenox.com/install.sh | sh
```

Or download the binary directly from [GitHub Releases](https://github.com/contenox/runtime/releases/latest).

---

## 2. Initialize a workspace

Run this once in the project directory you want Contenox to work in:

```bash
contenox init
```

This creates the workspace marker, writes the default chain and HITL policy presets, and ensures the built-in `local` backend exists.

---

## 3. Pull a local model

For the local-first path, pull a curated GGUF model:

```bash
contenox model pull granite-3.2-2b
contenox doctor
```

On a fresh install, the first pulled model becomes `default-model`, and `contenox init` sets `default-provider` to `local` when no provider was already configured.

Run your first prompt:

```bash
contenox "hello, what can you do?"
```

For a persistent chat session:

```bash
contenox chat -e
```

---

## 4. Optional editor use

Contenox can also run inside editor or desktop clients that speak ACP. The same chains, model config, tools, and HITL policy are used either way:

- [Use from Zed](/docs/guide/zed/)
- [Use from VS Code or VSCodium](/docs/guide/vscode-vscodium/)
- [Use from JetBrains](/docs/guide/jetbrains/)
- [Use from AionUi](/docs/guide/aionui/)

---

## Cloud providers

Contenox needs at least one model to work. Pick the option that fits:

| Option | What you need |
|--------|--------------|
| [Built-in local models](/docs/guide/local-models/) | Nothing - Contenox downloads and runs GGUF models itself |
| [Ollama](/docs/guide/providers/ollama/) | Ollama installed locally, or an Ollama Cloud key |
| [Google Gemini](/docs/guide/providers/gemini/) | A free Gemini API key (no GPU) |
| [OpenRouter](/docs/guide/providers/openrouter/) | One OpenRouter API key for many hosted models |
| [OpenAI](/docs/guide/providers/openai/) | An OpenAI API key |
| [Anthropic](/docs/guide/providers/anthropic/) | An Anthropic API key (Claude) |
| [Mistral](/docs/guide/providers/mistral/) | A Mistral API key |
| [AWS Bedrock](/docs/guide/providers/bedrock/) | An AWS account with Bedrock model access |

If you're not sure, start with [built-in local models](/docs/guide/local-models/) — no account or API key needed.

---

## Next steps

- [**Your first chain**](/docs/guide/first-chain/) — author your own agent in five edits
- [Core concepts](/docs/guide/concepts/) — how chains, tasks, and tools fit together
- [MCP integration](/docs/guide/mcp/) — connect external tools

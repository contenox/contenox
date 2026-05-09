# Contenox
**The AI copilot that's actually yours.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/github/v/release/contenox/contenox?label=version&logo=github)](https://github.com/contenox/contenox/releases)

For anyone whose job touches a terminal. Describe what you want in plain English; Contenox figures out which of your tools to call — your shell, your MCP servers, your APIs — and pauses to ask before each step runs. You approve, skip, or stop. Nothing runs past you.

📖 **[contenox.com](https://contenox.com)**

---

## Install

<!-- Release tooling: keep next line in sync with runtime/version/version.txt (updated by `make -f Makefile.version bump-*`). -->
<!-- TAG=v0.10.3 -->

```bash
curl -fsSL https://contenox.com/install.sh | sh
```

---

## Quick Start

You can use Contenox in one-shot commands, or drop into an interactive chat:

```bash
# One-shot command
contenox run "say hello world in python"

# Interactive session
contenox chat

# Resume a previous session
contenox session list
contenox chat --session <session-id>
```

---

## What it does

The connective tissue between the systems you already use, done in plain English with a human pause at every step.

```bash
# Someone yelled at you on Teams about a bug
cat teams-bug.txt | contenox "check the issue tracker for a duplicate; if none, file it and assign to dev group"

# Friday and you forgot the timesheet
contenox "use my git log to fill the timesheet, round to 9-5"

# New app on localhost:3000, you promised someone documentation
contenox "drive localhost:3000 with playwright, write the doc into Notion"
```

Useful day-to-day for the work above. Also a workbench for testing new chains and MCP servers, and a primitive other agents can shell out to.

State lives locally in SQLite. Sessions persist across invocations. The AI provider is a config line — Ollama, OpenAI, Gemini, vLLM, Vertex, or in-process llama.cpp.

---

## Connect your stack

Anything you can reach over MCP, an OpenAPI spec, or a shell command is a tool Contenox can call:

```bash
# Any MCP-compatible server (Notion, Linear, Playwright, GitHub, Postgres, …)
contenox mcp add notion https://mcp.notion.com/mcp --auth-type oauth

# Any HTTP API with an OpenAPI spec
contenox tools add my-api --url http://localhost:8000

# The shell, with your own command policy declared in the chain
contenox --shell "check Proxmox and flag anything red"
```

---

## Backends

Switch providers in one config line:

```bash
# Local providers
contenox backend add ollama    --type ollama
contenox backend add embedded  --type local  --url <path-to-gguf-or-hf-url> # In-process llama.cpp
contenox backend add myvllm    --type vllm   --url http://gpu-host:8000

# Cloud providers
contenox backend add openai    --type openai --api-key-env OPENAI_API_KEY
contenox backend add gemini    --type gemini --api-key-env GEMINI_API_KEY
contenox backend add vertex    --type vertex-google

# Set your defaults
contenox config set default-model qwen2.5:7b
contenox config set default-provider ollama
```

---

## Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/contenox/contenox
cd contenox
make build-contenox
```

---

> Questions: **hello@contenox.com**

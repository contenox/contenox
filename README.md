# Contenox
**The AI copilot that's actually yours.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/github/v/release/contenox/contenox?label=version&logo=github)](https://github.com/contenox/contenox/releases)

You describe what you want in plain English. *How* the agent behaves — system prompt, model selection, tool policy, retries, when to pause, when to branch — is a chain file you wrote, not a binary the vendor compiled. Edit it, version it in git, port it anywhere the engine runs.

📖 **[contenox.com](https://contenox.com)**

---

## Install

<!-- Release tooling: keep next line in sync with runtime/version/version.txt (updated by `make -f Makefile.version bump-*`). -->
<!-- TAG=v0.16.1 -->

```bash
curl -fsSL https://contenox.com/install.sh | sh
```

---

## Quick Start

```bash
# Scaffold a workspace and register the local backend
contenox init

# Pull a model — first pull becomes the default-model automatically
contenox model pull granite-3.2-2b

# Use it
contenox "say hello world in python"
contenox chat -e                        # open $EDITOR to compose a prompt
```

That's it. No API key, no external server, no `backend add` ceremony — `init` registers the local llama.cpp backend pointed at `~/.contenox/models/`, `model pull` populates it. Resume past sessions with `contenox session list` and `contenox session switch <name>`. To use a cloud provider instead, see [Backends](#backends) below.

---

## What you author

The agent's behavior is a chain file. Every decision is a JSON key:

```json
{
  "id": "review",
  "tasks": [
    {
      "id": "review",
      "handler": "chat_completion",
      "system_instruction": "You are a code reviewer. Analyze the diff, run the tests if tools are available, then give a concise review.",
      "execute_config": {
        "model": "{{var:model}}",
        "provider": "{{var:provider}}",
        "tools": ["local_shell", "local_fs"],
        "tools_policies": {
          "local_shell": { "_allowed_commands": "go,make,npm,cargo,grep,cat" }
        }
      },
      "transition": {
        "branches": [
          { "operator": "equals", "when": "tool-call", "goto": "run_tools" },
          { "operator": "default", "goto": "end" }
        ]
      }
    },
    {
      "id": "run_tools",
      "handler": "execute_tool_calls",
      "input_var": "review",
      "transition": {
        "branches": [
          { "operator": "default", "goto": "review" }
        ]
      }
    }
  ]
}
```

System prompt, model, tool policy, allowed commands — all yours. Save it and pipe in a diff:

```bash
git diff | contenox run --chain ./review.json
```

Walk through your first chain step by step: **[contenox.com/docs/guide/first-chain](https://contenox.com/docs/guide/first-chain/)**.

---

## What it does

The connective tissue between the systems you already use, done in plain English with a human pause at every step.

```bash
# Someone yelled at you on Teams about a bug
cat teams-bug.txt | contenox --shell "check the issue tracker for a duplicate; if none, file it and assign to dev group"

# Friday and you forgot the timesheet
contenox --shell "use my git log to fill the timesheet, round to 9-5"

# New app on localhost:3000, you promised someone documentation
contenox --shell "drive localhost:3000 with playwright, write the doc into Notion"
```

Useful day-to-day for the work above. Also a workbench for testing new chains and MCP servers, and a primitive other agents can shell out to.

State lives locally in SQLite. Sessions persist across invocations. The AI provider is a config line — Ollama, OpenAI, Gemini, vLLM, Vertex, or in-process llama.cpp. Any model, any vendor — or no vendor at all if you serve your own.

---

## Connect your stack

Anything you can reach over MCP, an OpenAPI spec, or a shell command is a tool Contenox can call:

```bash
# Any MCP-compatible server (Notion, Linear, Playwright, GitHub, Postgres, …)
contenox mcp add notion https://mcp.notion.com/mcp --auth-type oauth

# Any HTTP API with an OpenAPI spec (no glue code required)
# Slice a monolithic API into safe subsets by pointing --spec at a curated local file
contenox tools add erp_billing --url https://erp.internal.example.com --spec ./billing-subset.yaml

# The shell, with your own command policy declared in the chain
contenox --shell "check Proxmox and flag anything red"
```

---

## Backends

The `local` backend (in-process llama.cpp) is registered automatically by `contenox init` and lives at `~/.contenox/models/`. Populate it with `contenox model pull <name>` — never type `backend add local` yourself. To add anything else:

```bash
# Other local servers
contenox backend add ollama    --type ollama
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

# Contenox
**A local runtime for packaged, auditable AI workflows.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/github/v/release/contenox/runtime?label=version&logo=github)](https://github.com/contenox/runtime/releases)

Contenox is an Apache 2 runtime for turning repeatable knowledge/tool workflows into versioned chains. A chain makes the important parts explicit: system prompts, model routing, tool allowlists, retries, pauses, branch conditions, and human approval gates. Edit it, review it, commit it, and run it on your machine with the models and tools you choose.

The useful unit is a known workflow with examples, tools, acceptance rules, and review points, not a vague promise of fully delegated work.

📖 **[contenox.com](https://contenox.com)**

---

## Install

<!-- Release tooling: keep next line in sync with runtime/version/version.txt (updated by `make -f Makefile.version bump-*`). -->
<!-- TAG=v0.28.1 -->

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

## Local UI

Start the local HTTP server and Beam UI:

```bash
contenox serve
```

By default it listens on `127.0.0.1:32123` and serves the UI at the printed URL.
Use `PORT=32125 contenox serve` or `ADDR=127.0.0.1 PORT=32125 contenox serve`
to override the bind address.

---

## What you author

The workflow behavior is a chain file. Every decision is a JSON key:

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
          { "operator": "equals", "when": "tool_call", "goto": "run_tools" },
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

System prompt, model, tool policy, allowed commands, retry budget, and transitions are all visible. Save the chain and pipe in a diff:

```bash
git diff | contenox run --chain ./review.json
```

Walk through your first chain step by step: **[contenox.com/docs/guide/first-chain](https://contenox.com/docs/guide/first-chain/)**.

---

## What it is good for

Contenox is strongest when the workflow is specific and repeatable: known inputs, known tools, known output shape, and explicit review gates.

Examples of workflows you can package as chains:

```text
Release evidence pack
Input: git log, PRs, tickets, CI output
Output: changelog, risk notes, deployment checklist, reviewer packet
Gate: human approval before publishing
```

```text
API-to-workflow wrapper
Input: internal OpenAPI spec
Output: curated tool subset, hidden tenant/env args, auth handling, HITL policy
Gate: approval for mutating calls
```

```text
Repo maintenance chain
Input: issue or migration request
Output: patch, test run, PR description
Gate: shell/filesystem approval and human merge
```

State lives locally in SQLite. Sessions persist across invocations. The AI provider is a config line — Ollama, OpenAI, Anthropic, Mistral, Gemini, AWS Bedrock, vLLM, Vertex (Gemini), or in-process llama.cpp. Use a cloud model, a local server, or a local GGUF model depending on the workflow and data boundary.

---

## Connect your stack

Anything you can reach over MCP, an OpenAPI spec, or a shell command can become a scoped tool in a chain:

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

## Use it from Zed (or any ACP client)

ACP/editor support is an optional way to run the same local chains inside an editor. Contenox speaks the [Agent Client Protocol](https://github.com/zed-industries/agent-client-protocol) over stdio. Drop this into `~/.config/zed/settings.json`:

```json
{
  "agent_servers": {
    "Contenox": {
      "type": "custom",
      "command": "contenox",
      "args": ["acp"]
    }
  }
}
```

Open Zed's agent panel and pick **Contenox**. Your chain runs inside the editor: tool calls render as cards with the actual command/path, HITL prompts route through Zed's permission UI, and session history replays when you reopen the project. Chain selection lives at `~/.contenox/default-acp-chain.json` (or set `CONTENOX_ACP_CHAIN_PATH`). Full guide → **[contenox.com/docs/guide/zed](https://contenox.com/docs/guide/zed/)**.

**JetBrains** (GoLand, IntelliJ IDEA, …) reads agent servers from `~/.jetbrains/acp.json` — same binary, different schema (no `"type"` field):

```json
{
  "default_mcp_settings": { "use_custom_mcp": true, "use_idea_mcp": false },
  "agent_servers": {
    "Contenox": {
      "command": "contenox",
      "args": ["acp"]
    }
  }
}
```

Verified with GoLand 2026.1.2. Full guide → **[contenox.com/docs/guide/jetbrains](https://contenox.com/docs/guide/jetbrains/)**.

**AionUi** — a free, local, open-source desktop chat UI for ACP agents. Add a Custom Agent: command `contenox`, args `["acp"]`. Verified with AionUi 2.0.0. Full guide → **[contenox.com/docs/guide/aionui](https://contenox.com/docs/guide/aionui/)**.

---

## Backends

The `local` backend (in-process llama.cpp) is registered automatically by `contenox init` and lives at `~/.contenox/models/`. Populate it with `contenox model pull <name>` — never type `backend add local` yourself. To add anything else:

```bash
# Other local servers
contenox backend add ollama    --type ollama
contenox backend add myvllm    --type vllm   --url http://gpu-host:8000

# Cloud providers
contenox backend add openai    --type openai    --api-key-env OPENAI_API_KEY
contenox backend add anthropic --type anthropic --api-key-env ANTHROPIC_API_KEY
contenox backend add mistral   --type mistral   --api-key-env MISTRAL_API_KEY
contenox backend add gemini    --type gemini    --api-key-env GEMINI_API_KEY
contenox backend add bedrock   --type bedrock   --url https://bedrock-runtime.us-east-1.amazonaws.com
contenox backend add vertex    --type vertex-google --url "https://us-central1-aiplatform.googleapis.com/v1/projects/$GOOGLE_CLOUD_PROJECT/locations/us-central1"

# Set your defaults
contenox config set default-model qwen2.5:7b
contenox config set default-provider ollama
```

---

## Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/contenox/runtime
cd runtime
make build-contenox
```

---

> Questions: **hello@contenox.com**

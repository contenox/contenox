# Contenox
**AI workflows you can run, review, and own.**

Contenox is an open-source AI workflow runtime for developers. It turns
repeatable coding and tool workflows into versioned Chains: files that declare
prompts, model/provider routing, tool allowlists, retries, branches, budgets,
and human approval gates.

Many coding workflows do not need a frontier model. Contenox gives you a way to
run that work where the code is, with a proper agent loop instead of hidden
prompt habits or one-off glue, and route to network or cloud models when the job
needs them.

Run the same workflow from the CLI, VS Code, or any ACP client. Use `modeld` for
the edge path, Ollama or vLLM on your network, or hosted providers, while
sessions, config, telemetry, and runtime state stay on your machine.

- **It speaks Unix:** Pipe data directly into your workflows. `git diff | contenox run commit-msg` or `git log | contenox run release-notes`.
- **It respects boundaries:** Human-in-the-loop isn't a UI toggle, it's a strict policy file. The AI pauses and asks for terminal approval before running destructive commands.
- **It routes inference:** Use edge `modeld`, private-network backends, or hosted providers per workflow. `modeld` is built for one active local model and resident coding context, not model multiplexing.

You own the workflow. The vendor doesn't decide how it behaves on your machine. You do.

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/github/v/release/contenox/runtime?label=version&logo=github)](https://github.com/contenox/runtime/releases)

It is built for specific, reviewable AI work, not vague promises of fully
autonomous agents.

📖 **[contenox.com](https://contenox.com)**

---

## What would I use this for?

Package a repeatable AI task as a chain, then run it the same way every time:

- **Review a diff** — run the tests, summarize the risk, and gate on your approval before it acts.
- **Draft release evidence** — turn git log, PRs, and CI output into a changelog and reviewer packet.
- **Wrap an internal API** — expose a safe, curated tool subset with approval required on mutating calls.
- **Automate repo chores** — take an issue, produce a patch, run the tests, write the PR description.
- **Ask an owned model** — codebase chat and one-off prompts through local modeld or a private inference endpoint.
- **Use edge autocomplete** — keep VS Code ghost text on a local or local-network coder model while chat uses a larger hosted model.

The same chains run from the CLI, VS Code, or any ACP client. Inference can sit
on the device, on your network, or with a cloud vendor, while sessions and state
stay local. Detailed examples are in [What it is good for](#what-it-is-good-for)
below.

---

## Install

<!-- Release tooling: keep next line in sync with runtime/version/version.txt (updated by `make -f Makefile.version bump-*`). -->
<!-- TAG=v0.32.5 -->

```bash
curl -fsSL https://contenox.com/install.sh | sh
```

---

## Quick Start

```bash
# Configure a provider/model for this machine
contenox setup

# Use it from the CLI
contenox "say hello world in python"
contenox chat -e                        # open $EDITOR to compose a prompt
```

For normal CLI/VS Code installs, choose local Ollama, a private network backend,
or a hosted provider in setup. Owned local GGUF/OpenVINO inference uses the
separate native `modeld` daemon, which is not bundled in release installs yet.
If you choose a local modeld provider, setup prints source-build commands. Full guide:
[modeld Source Build and Packaging](docs/modeld-source-build.md).

Resume past sessions with `contenox session list` and
`contenox session switch <name>`. Backends are summarized below.

Developing the source-built local backend? See
[modeld Source Build and Packaging](docs/modeld-source-build.md).

### VS Code autocomplete can use a different model

Inline autocomplete is intentionally separate from chat. That lets you run
low-latency ghost text at the edge, on a LAN Ollama box, or on a FIM/coder cloud
model while keeping chat and tool workflows on a larger provider.

```bash
# Chat can stay on a hosted model:
contenox config set default-provider openai
contenox config set default-model    gpt-5-mini

# Autocomplete can stay local via modeld:
contenox config set default-autocomplete-provider llama
contenox config set default-autocomplete-model    qwen3-coder-30b-a3b

# Or point autocomplete at a local-network Ollama coder model:
contenox config set default-autocomplete-provider ollama
contenox config set default-autocomplete-model    qwen2.5-coder:7b
```

In VS Code, enable it with `Contenox: Enable Autocomplete` and verify with
`Contenox: Test Autocomplete At Cursor`.

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

State lives locally in SQLite. Sessions persist across invocations. The AI
provider is a config line: local modeld (`llama`/`openvino`), Ollama, vLLM,
OpenAI, Anthropic, Mistral, Gemini, AWS Bedrock, OpenRouter, or Vertex. Use
edge inference, private network inference, or a hosted vendor depending on the
workflow, latency target, cost, and data boundary. Autocomplete has its own
provider/model defaults, so editor ghost text can stay local even when chat
uses the cloud.

---

## Where it fits

Contenox is the agent layer you control from terminal to editor. The category is
AI workflow runtime with edge, private network, and cloud inference routing; the
architecture is developer agent runtime.

| Nearby world | Why Contenox is different |
|--------------|---------------------------|
| Cursor / IDE copilots | Runtime-first, not editor-first. The same engine works from the terminal, VS Code, and ACP clients. |
| Aider / CLI coding agents | Broader workflow, session, tool policy, and provider scope than a single coding loop. |
| LangChain / agent frameworks | End-user executable product, not just a library you wire into an app. |
| Dify / n8n / web AI workflow tools | Local desktop/workspace-first, not web-app/SaaS-first. |
| Ollama wrappers | Provider-neutral and workflow/tool/HITL-oriented, spanning owned local inference, private network backends, and hosted vendors. |

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

## Local north star: long context on your own accelerator

Most of Contenox runs against whatever provider you choose. The native `modeld`
daemon exists for one specific bet: a local AI coding agent on a single consumer
accelerator that serves **real, long-context work** — an *effective* context far
beyond a model's native window (the goal is ~200k tokens) on limited hardware, by
treating context as resident state kept hot rather than a prompt resent every turn.

`modeld` is shaped entirely around that bet:

- **One model, one user, many sessions.** A single active model slot serves many
  persistent sessions for one owner, so the device's whole memory and KV budget go
  to making *that* model deep and fast instead of multiplexing several.
- **Warm-reuse sessions.** Each session keeps its stable prefix's KV hot and
  re-prefills only the changed suffix (`EnsurePrefix → PrefillSuffix → Decode`), so
  a long working context is paid for once, not resent on every turn.
- **Snapshot / restore.** Session state is durable and branchable, so effective
  context outlives a single live process.
- **Accelerator-driven, no knobs.** modeld detects the accelerator and derives
  offload and the effective window from the device at runtime — no per-model flags.

This is the direction the local backend is built toward, not a shipped guarantee on
every model and device. The workflow runtime above doesn't depend on it — use any
hosted or local provider today. How it maps onto the code (KV cache, warm reuse,
capacity, the latency budget, and what's still required):
[Effective Context North Star](docs/blueprints/effective-context-north-star.md).

---

## Backends

The `llama` and `openvino` backends are local modeld-backed inference providers.
`contenox init` registers them automatically and `contenox model pull <name>`
downloads artifacts into `~/.contenox/models/<backend>/`. The current CLI/VSIX
release assets do not bundle `modeld`, so local modeld providers require a
source build for now:
[modeld Source Build and Packaging](docs/modeld-source-build.md).

To add other backends:

```bash
# Private network / self-hosted inference
contenox backend add ollama    --type ollama
contenox backend add myvllm    --type vllm   --url http://gpu-host:8000

# Hosted AI vendors
contenox backend add openai    --type openai    --api-key-env OPENAI_API_KEY
contenox backend add anthropic --type anthropic --api-key-env ANTHROPIC_API_KEY
contenox backend add mistral   --type mistral   --api-key-env MISTRAL_API_KEY
contenox backend add gemini    --type gemini    --api-key-env GEMINI_API_KEY
contenox backend add bedrock   --type bedrock   --url https://bedrock-runtime.us-east-1.amazonaws.com
contenox backend add vertex    --type vertex-google --url "https://us-central1-aiplatform.googleapis.com/v1/projects/$GOOGLE_CLOUD_PROJECT/locations/us-central1"

# Set your defaults
contenox config set default-model qwen3-8b
contenox config set default-provider llama
```

---

## Build from source

Requires Go 1.25+.

```bash
git clone https://github.com/contenox/runtime
cd runtime
make build-contenox
```

Build and run local modeld for llama.cpp:

```bash
CONTENOX_MODELD_BACKEND=llama make run-modeld
```

Build and run local modeld for OpenVINO:

```bash
make deps-modeld
CONTENOX_MODELD_BACKEND=openvino make run-modeld
```

Build a relocatable Linux modeld bundle:

```bash
MODELD_DIST_DIR="$PWD/bin/modeld-linux-amd64" make package-modeld
tar -C bin -czf bin/modeld-linux-amd64.tar.gz modeld-linux-amd64
```

See [modeld Source Build and Packaging](docs/modeld-source-build.md) for the
complete local modeld flow.

---

## Built on

The `contenox` CLI is pure Go. Local inference lives in the separate `modeld`
daemon, which builds on these upstream projects (pinned in `mk/llama-flags.mk` and
`mk/openvino-flags.mk`):

| Project | Role | License |
| --- | --- | --- |
| [llama.cpp](https://github.com/ggml-org/llama.cpp) | GGUF inference and the ggml CPU/CUDA/HIP/Metal backends | MIT |
| [OpenVINO](https://github.com/openvinotoolkit/openvino) | Inference runtime (CPU / iGPU / NPU) | Apache-2.0 |
| [OpenVINO GenAI](https://github.com/openvinotoolkit/openvino.genai) | LLM pipeline over OpenVINO | Apache-2.0 |
| [OpenVINO Tokenizers](https://github.com/openvinotoolkit/openvino_tokenizers) | Tokenizer extension for OpenVINO GenAI | Apache-2.0 |
| [minja](https://github.com/google/minja) | Chat-template engine (vendored by OpenVINO GenAI) | MIT |
| [gguf-tools](https://github.com/Lourdle/gguf-tools) | GGUF parsing headers (vendored by OpenVINO GenAI) | see upstream |

Native backends are compiled, not embedded: `modeld` links these at build time and
ships their runtime libraries inside each release package. Upstream license texts
travel with the artifacts (`licenses/` in dependency bundles, `LICENSES/` in modeld
packages). Other Go dependencies are listed in `go.mod`.

Provider integrations contenox talks to over the network (Ollama, vLLM, and hosted
OpenAI-compatible vendors) are not built into contenox and are not listed here.

---

> Questions: **hello@contenox.com**

# Contenox

**AI workflows you can run, review, and own.**

Use AI for development without unlearning how to do it yourself.

Contenox is an open-source, local-first AI workflow runtime for engineers. It
packages repeatable AI-assisted work into versioned Chains: files that declare the
prompt, model route, tool allowlist, command policy, retry behavior, branches,
budgets, and human approval gates.

The agent loop does the work. The Chain is the contract.

Contenox is for work where AI may touch a terminal, repository, internal API,
ticket system, browser, or production-adjacent data, and where "the model decided"
is not an acceptable control boundary.

Run the same Chain from the CLI, VS Code, or any ACP client. Route inference to a
local model, a private-network backend, or a hosted provider. Sessions, config,
run logs, and runtime state stay on your machine. No hosted Contenox service
required.

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Version](https://img.shields.io/github/v/release/contenox/runtime?label=version&logo=github)](https://github.com/contenox/runtime/releases)

Docs: **[contenox.com](https://contenox.com)**

---

## Install

<!-- Release tooling: keep next line in sync with runtime/version/version.txt (updated by `make -f Makefile.version bump-*`). -->
<!-- TAG=v0.32.8 -->

```bash
curl -fsSL https://contenox.com/install.sh | sh
```

Prefer to inspect first?

```bash
curl -fsSLO https://contenox.com/install.sh
less install.sh
sh install.sh
```

Release downloads and source builds are available from the
[releases page](https://github.com/contenox/runtime/releases).

---

## Quick Start

```bash
contenox setup                          # choose a provider/model for this machine
contenox "say hello world in python"    # use it from the CLI
contenox chat -e                        # open $EDITOR to compose a prompt
```

Resume past work with `contenox session list` and `contenox session switch <name>`.

Inline editor autocomplete is intentionally a separate model from chat, so ghost
text can stay local and low-latency while chat uses a larger model:

```bash
contenox config set default-provider              openai          # chat on a hosted model
contenox config set default-model                 gpt-5-mini
contenox config set default-autocomplete-provider llama           # ghost text on local modeld
contenox config set default-autocomplete-model    qwen3-coder-30b-a3b
```

In VS Code, enable it with `Contenox: Enable Autocomplete`.

---

## Why Chains?

A naked agent loop is useful, but it is not enough when AI can touch real tools.

A Chain answers the questions a serious team has to ask before letting a model
act:

- What is the task?
- Which model or provider may be used?
- Which tools may the model call?
- Which commands or API operations are allowed?
- What must stop for human approval?
- What state, trace, and evidence does the run leave behind?
- Can the workflow be reviewed, committed, diffed, and run again?

In Contenox, a Chain is not a prompt pipeline. It is the reviewed execution
contract around an agent loop.

---

## What You Author

The unit of work is a Chain: a single versioned file where every decision is a
visible JSON key. Prompts, provider routing, tool scope, command policy, retry
policy, token limits, loop budgets, and branches are part of the artifact you
review.

```json
{
  "id": "review",
  "token_limit": 65536,
  "tasks": [
    {
      "id": "review",
      "handler": "chat_completion",
      "system_instruction": "You are a code reviewer. Analyze the diff, run tests if tools are available, then give a concise review.",
      "execute_config": {
        "model": "{{var:model}}",
        "provider": "{{var:provider}}",
        "tools": ["local_shell", "local_fs"],
        "tools_policies": {
          "local_shell": {
            "_allowed_commands": "go,make,npm,cargo,grep,cat",
            "_denied_commands": "sudo,su,dd,mkfs,fdisk,parted,shred"
          },
          "local_fs": {
            "_allowed_dir": ".",
            "_max_read_bytes": "262144"
          }
        },
        "retry_policy": {
          "max_attempts": 4,
          "initial_backoff": "1s",
          "max_backoff": "30s",
          "jitter": 0.25,
          "rate_limit_min_wait": "10s"
        }
      },
      "transition": {
        "branches": [
          {
            "operator": "edge_traversed_at_least",
            "edge": "review->run_tools",
            "when": "6",
            "goto": "end"
          },
          { "operator": "equals", "when": "tool_call", "goto": "run_tools" },
          { "operator": "default", "goto": "end" }
        ]
      }
    },
    {
      "id": "run_tools",
      "handler": "execute_tool_calls",
      "input_var": "review",
      "execute_config": {
        "tools": ["local_shell", "local_fs"]
      },
      "transition": {
        "branches": [
          { "operator": "default", "goto": "review" }
        ]
      }
    }
  ]
}
```

Save it, then pipe your work into it. It speaks Unix:

```bash
git diff | contenox run --chain ./review.json
```

HITL is not a hidden toggle. Gated tool calls route through policy files such as
`hitl-policy-default.json`, `hitl-policy-strict.json`, and editor-specific ACP
policies. The Chain defines what the workflow can ask for; the active policy
decides what must pause for approval before execution.

Walk through your first chain:
**[contenox.com/docs/guide/first-chain](https://contenox.com/docs/guide/first-chain/)**.

---

## What It Is Good For

Contenox is strongest when the workflow is specific and repeatable: known inputs,
known tools, known output shape, and explicit review gates.

- **Review a diff** - run tests, summarize risk, and gate on approval before
  anything destructive runs.
- **Draft release evidence** - turn git log, PRs, tickets, and CI output into a
  changelog, risk notes, deployment checklist, and reviewer packet.
- **Wrap an internal API** - expose a safe OpenAPI subset with hidden tenant/env
  args and approval required on mutating calls.
- **Automate repo chores** - take an issue, produce a patch, run checks, and write
  the PR description.
- **Inspect operational systems** - query dashboards, shell scripts, or MCP tools
  through scoped policies instead of broad credentials.
- **Use edge autocomplete** - keep VS Code ghost text on a local or local-network
  coder model while chat uses a larger hosted model.

The same Chain runs from the terminal, VS Code, Zed, JetBrains, AionUi, or any ACP
client. Provider choice is config: local `modeld`, Ollama, vLLM, OpenAI,
OpenRouter, Anthropic, Mistral, Gemini, AWS Bedrock, or Vertex.

---

## For Work That Touches Real Systems

Contenox is built for high-consequence engineering workflows: production repos,
internal APIs, infrastructure scripts, operational dashboards, release processes,
and systems of record.

In those environments, "AI agent" cannot mean "give a model broad credentials and
hope." It needs a runtime boundary: explicit tools, explicit policy, local state,
human approval, and reviewable evidence.

The problem is not that agents can act. The problem is agents acting outside a
boundary you authored.

The model can reason, inspect, and propose. The Chain decides what it may touch.
The operator decides what it may change.

---

## What the Runtime Protects

| Risk | Contenox mechanism |
| --- | --- |
| Agent behavior disappears into chat history | Chains are files: reviewable, versionable, repeatable |
| The model can touch too much | Tool allowlists, command policies, and scoped API specs |
| Human review happens after damage | Destructive actions stop at approval gates before execution |
| Internal APIs become broad agent tools | Curated OpenAPI subsets with hidden environment/tenant args |
| Vendor choice becomes workflow lock-in | Provider/model routing is config, not application logic |
| Routine work burns frontier-model budget | Route simple work to local or private-network models |
| Team knowledge leaves the workstation | Sessions, state, config, and run logs stay local |

---

## The Stance: Exoskeleton, Not Autopilot

AI is becoming part of software work. The question is whether it makes you sharper
or more dependent.

Naive AI use turns engineering judgment into rented fluency: useful while the
model is good, reachable, current, and affordable. But the durable value in
software was never the typing. It was knowing what to build, knowing what changed,
and being able to own the system when it breaks.

Contenox is built as an exoskeleton, not an autopilot. It amplifies the person
doing the work. You stay in the loop because the workflow, tools, state, and
approval policy are things you author and review.

Contenox is not an autonomous coding employee. It is not a hosted autopilot. It is
not a prompt habit hidden in your shell history.

It is a local runtime for AI-assisted work that still has an owner.

---

## Where Contenox Fits

Contenox is the agent layer you control from terminal to editor.

| Nearby world | Contenox stance |
| --- | --- |
| IDE copilots | Editor assistance is not enough. The workflow should run from terminal, VS Code, and ACP clients. |
| CLI coding agents | A single coding loop is not a runtime. Contenox adds sessions, tool policy, provider routing, and review gates. |
| LangChain / agent frameworks | Libraries are not the product. Contenox is an executable local runtime for end users and teams. |
| Dify / n8n / web workflow tools | AI workflows that touch local code and tools should not require a SaaS control plane. |
| Ollama wrappers | A model host is not a workflow boundary. Contenox adds Chains, tools, HITL policy, and routing across local, private, and hosted models. |

---

## Connect Your Stack

Anything reachable over MCP, an OpenAPI spec, or a shell command can become a
scoped tool in a Chain:

```bash
# Any MCP-compatible server
contenox mcp add notion https://mcp.notion.com/mcp --auth-type oauth

# Any HTTP API with an OpenAPI spec
contenox tools add erp_billing \
  --url https://erp.internal.example.com \
  --spec ./billing-subset.yaml

# The shell, with your own command policy declared in the Chain
contenox --shell "check Proxmox and flag anything red"
```

---

## Use It From Your Editor

Contenox speaks the [Agent Client Protocol](https://github.com/zed-industries/agent-client-protocol)
over stdio, so the same local Chains run inside Zed, JetBrains, AionUi, and other
ACP clients. For Zed, drop this into `~/.config/zed/settings.json`:

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

Tool calls render as cards with the real command/path, HITL prompts route through
the editor's permission UI, and session history replays when you reopen the
project.

Full guides: [Zed](https://contenox.com/docs/guide/zed/),
[JetBrains](https://contenox.com/docs/guide/jetbrains/),
[AionUi](https://contenox.com/docs/guide/aionui/).

---

## Backends

Provider/model routing is configuration, not application logic. Add local,
private-network, or hosted backends the same way:

```bash
# Private network / self-hosted inference
contenox backend add ollama --type ollama
contenox backend add myvllm --type vllm --url http://gpu-host:8000

# Hosted AI vendors
contenox backend add openai \
  --type openai \
  --api-key-env OPENAI_API_KEY
contenox backend add openrouter \
  --type openrouter \
  --api-key-env OPENROUTER_API_KEY
contenox backend add anthropic \
  --type anthropic \
  --api-key-env ANTHROPIC_API_KEY
contenox backend add mistral \
  --type mistral \
  --api-key-env MISTRAL_API_KEY
contenox backend add gemini \
  --type gemini \
  --api-key-env GEMINI_API_KEY
contenox backend add bedrock \
  --type bedrock \
  --url https://bedrock-runtime.us-east-1.amazonaws.com
contenox backend add vertex \
  --type vertex-google \
  --url "https://us-central1-aiplatform.googleapis.com/v1/projects/$GOOGLE_CLOUD_PROJECT/locations/us-central1"

# Set your defaults
contenox config set default-model    qwen3-8b
contenox config set default-provider llama
```

The `llama` and `openvino` backends are local `modeld`-backed providers.
`contenox init` registers them and `contenox model pull <name>` downloads
artifacts into `~/.contenox/models/<backend>/`. Current normal CLI and VS Code
release packages do not bundle `modeld` yet, so local `modeld` providers require
a source build:
[modeld Source Build and Packaging](docs/modeld-source-build.md).

---

## modeld North Star

Routine tokens should be local or private by default.

`modeld` is Contenox's local-inference north star: one owner, one active local
model, many persistent sessions, and resident coding context on a workstation
accelerator.

This is the direction of the local backend, not a guarantee for every model,
device, or release package today.

`modeld` is shaped around one specific bet: a local coding agent on a single
consumer accelerator that serves real, long-context work. The goal is an
effective context far beyond a model's native window on limited hardware by
treating context as resident state kept hot rather than a prompt resent every
turn.

- **One model, one user, many sessions.** The device's whole memory and KV budget
  go to making that model deep and fast instead of multiplexing several.
- **Warm-reuse sessions.** Each session keeps its stable prefix's KV hot and
  re-prefills only the changed suffix, so a long working context is paid for once.
- **Snapshot / restore.** Session state is durable and branchable, so effective
  context outlives a single live process.
- **Accelerator-driven, no knobs.** `modeld` detects the accelerator and derives
  offload and the effective window from the device at runtime.

Longer term, `modeld` is also where Contenox can make local models adapt to the
workstation: resident context, reusable sessions, and optional adaptation such as
LoRA where it makes sense.

How it maps onto the code:
[Effective Context North Star](docs/blueprints/effective-context-north-star.md).

---

## Build from Source

Requires Go 1.25+.

```bash
git clone https://github.com/contenox/runtime
cd runtime
make build-contenox

# Build and run local modeld (llama.cpp)
CONTENOX_MODELD_BACKEND=llama make run-modeld

# Build and run local modeld (OpenVINO)
make deps-modeld
CONTENOX_MODELD_BACKEND=openvino make run-modeld
```

See [modeld Source Build and Packaging](docs/modeld-source-build.md) for the
complete local modeld flow and relocatable bundles.

---

## Built on

The `contenox` CLI is pure Go. Local inference lives in the separate `modeld`
daemon, which links these upstream projects at build time (pinned in
`mk/llama-flags.mk` and `mk/openvino-flags.mk`) and ships their runtime libraries
inside each release package:

| Project | Role | License |
| --- | --- | --- |
| [llama.cpp](https://github.com/ggml-org/llama.cpp) | GGUF inference and the ggml CPU/CUDA/HIP/Metal backends | MIT |
| [OpenVINO](https://github.com/openvinotoolkit/openvino) | Inference runtime (CPU / iGPU / NPU) | Apache-2.0 |
| [OpenVINO GenAI](https://github.com/openvinotoolkit/openvino.genai) | LLM pipeline over OpenVINO | Apache-2.0 |
| [OpenVINO Tokenizers](https://github.com/openvinotoolkit/openvino_tokenizers) | Tokenizer extension for OpenVINO GenAI | Apache-2.0 |
| [minja](https://github.com/google/minja) | Chat-template engine (vendored by OpenVINO GenAI) | MIT |
| [gguf-tools](https://github.com/Lourdle/gguf-tools) | GGUF parsing headers (vendored by OpenVINO GenAI) | see upstream |

Upstream license texts travel with the artifacts (`licenses/` in dependency
bundles, `LICENSES/` in modeld packages). Other Go dependencies are in `go.mod`.

---

> Questions: **hello@contenox.com**

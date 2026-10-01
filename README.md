# contenox

**AI agents. Under your command.**

Contenox is built for demanding work — and the people accountable for it. The
same system carries work from an interactive terminal or editor into scripts,
CI and unattended missions.

Choose the models and infrastructure. Attach only the tools the work requires.
Keep policy, state and operational evidence under your control. No Contenox
account is required.

![contenox beam running a session](https://contenox-website-assets-573643652148.s3.amazonaws.com/media/hitl-approve.gif)

Docs: **[contenox.com](https://contenox.com)**

---

## Start here

```bash
curl -fsSL https://contenox.com/install.sh | sh
```

Prefer to read it first?

```bash
curl -fsSLO https://contenox.com/install.sh
less install.sh
sh install.sh
```

<!-- TAG=v1.2.0 -->

```bash
contenox auto                           # choose a local model, verify it, and open the TUI
```

Run it from the project directory you want to work in. `contenox auto --dry-run`
previews the choice. For an existing model server or a hosted API, use
`contenox setup`, then `contenox beam`. See the [quickstart](https://contenox.com/docs/guide/quickstart/)
for hardware and backend requirements.

To serve models to applications or other machines, follow
[Gateway: day one](https://contenox.com/docs/guide/tutorials/gateway-local/), then
[Gateway operations](https://contenox.com/docs/guide/gateway-operations/) for
service startup, access management, backups and upgrades.

`contenox doctor` reports anything missing; its first line is the verdict:

```
Ready: yes — run: contenox beam
```

`contenox init` marks the project — it writes `.contenox/workspace.id` here, and
seeds the agents, envelopes and config into `~/.contenox/` so every project on
the machine starts from them (`contenox init --local` seeds workspace copies
instead). `contenox vet` checks a policy before anything runs under it. Sessions
persist: `contenox session list` and `contenox session switch <name>` pick past
contexts back up.

---

## Ways in

**`contenox beam`** — the terminal client, first-party and in the box.
Full-screen, the transcript is your native scrollback, the composer takes `/` for
commands and `@` to put a file in front of the agent. Bare `contenox` on a
terminal opens it.

```bash
contenox beam                           # or just: contenox
```

**`contenox acp`** — the same agent over stdio to Zed, JetBrains, AionUi,
OpenClaw and anything else that speaks the Agent Client Protocol. No plugin
lock-in; per the protocol the editor owns the workspace, so the session works in
the project you already have open, and approvals route through the editor's own
permission UI.

```bash
contenox acp                            # speak ACP over stdio to any ACP client
```

**`contenox run`** — a program is the caller: CI, cron, another agent. It runs
the task with the tools on that machine and prints the report to stdout, exit 0
when the work landed and nonzero when it did not.

```bash
contenox run "summarise what changed under ./internal since Friday"
contenox run reviewer "review the payment retry change"
```

With no agent named it runs the preseeded `run` declaration.

---

## Agents are declared, not built

An agent is one Markdown file with a YAML frontmatter header:

```markdown
---
name: reviewer
description: Reviews a file for correctness problems
tools: Read, Glob, Grep
---

You are a code reviewer. Read the file you are asked about, then list the
problems you can point at in what you actually read.
```

Drop it in `.contenox/agents/` and the next run picks it up — no build step, no
plugin API. `.claude/agents/` and `.agents/agents/` are read where they are, so
declarations you already keep for Claude Code, Copilot, Cursor, OpenCode or
Antigravity import unchanged. Behind each one contenox compiles a **chain** that
says what happens and a **policy** that says what is permitted, both JSON
Schema-validated, both yours to read and neither yours to maintain.

[Declaring agents →](https://contenox.com/docs/guide/declarations/) ·
[Chain files →](https://contenox.com/docs/guide/chains/)

---

## No tools loaded without declaration

contenox owns the tool boundary and you decide what stands on the other side of
it. Every tool you do not need is tokens burned on every turn.

Tools have to be passed to agents via the declarations:

`cat ~/.contenox/agents/run.md`

```yaml
---
name: run
description: Carries out one stated task on this machine and reports what it did, for a caller that is a program rather than a person
tools: "*" # or a list of tools, or even nothing
---
```

This is especially useful when you have a lot of tools but not all workflows require them, this enforces a boundary to the agent and prevents confusing it with additional tool-descriptions.

Tools are added to contenox via the CLI:

```bash
# Connect any Model Context Protocol (MCP) server
contenox mcp add notion https://mcp.notion.com/mcp --auth-type oauth

# Wrap an internal HTTP API using its OpenAPI specification
contenox tools add erp_billing \
  --url https://erp.internal.example.com \
  --spec ./billing-subset.yaml
```

A declaration can also bring its own, scoped to that agent, reachable by no
other one, retired when you delete the file:

```yaml
mcpServers:
  filesystem:
    command: npx
    args: ["-y", "@modelcontextprotocol/server-filesystem", "/data"]
remoteTools:
  billing:
    url: https://internal.example.com
    spec: https://internal.example.com/openapi.json
```

---

## Guardrails

Every run is bounded by an **envelope**: a JSON policy naming what passes
silently, what stops for a human, and what is denied outright, plus hard ceilings
on tool calls and tokens. Anything no rule matches fails closed — it asks. Six
presets ship with `contenox init`, and the knobs a declaration cannot reach live
in `agents.toml`. Every session leaves reviewable local state on disk.

A run that stops for a person checkpoints where it stopped, saves the ask and
releases the process. Restart the box, close the laptop, let days pass: when the
answer arrives the run resumes from that exact point, exactly once.

```bash
contenox approvals list
contenox approvals respond 8f3c --answer "yes, send them"
```

The [sandbox](https://contenox.com/docs/guide/confinement/sandbox/) — Landlock
filesystem and exec confinement, scrubbed environment, Linux-only — confines
foreign agent code you choose to run locally.

---

## Inference

`contenox auto` detects the hardware, installs the native `modeld` worker and a
compatible model, verifies a real tool call, and opens Beam. Ollama, vLLM and
hosted providers remain available when they are the right operational boundary:

```bash
# Native local inference
contenox auto

# Existing local or private-network inference
contenox backend add ollama --type ollama
contenox backend add myvllm --type vllm --url http://gpu-host:8000

# Hosted providers
contenox backend add openai    --type openai    --api-key-env OPENAI_API_KEY
contenox backend add anthropic --type anthropic --api-key-env ANTHROPIC_API_KEY
contenox backend add gemini    --type gemini    --api-key-env GEMINI_API_KEY

# Defaults for an explicitly registered backend
contenox config set inference.provider ollama
contenox config set inference.model qwen2.5:7b
```

Also supported: Vertex AI and Amazon Bedrock. By default sessions, configuration,
run logs and captured execution state live in one local SQLite database. A
server-backed deployment moves the store, message bus and cache onto PostgreSQL,
NATS and Valkey. Telemetry is opt-in and off by default. Secrets resolve from
your environment at request time and never land in config on disk.

---

## Building from source

The `contenox` control-plane binary is pure Go. Native inference is a separate
`modeld` worker with llama.cpp and OpenVINO backends and therefore has its own
native build and packaging path.

```bash
git clone https://github.com/contenox/contenox
cd contenox
task build          # build the contenox binary
task modeld:build   # build modeld and its selected native backend
```

This repository is a release mirror: every commit on `main` is one release and
every tag a signed one, built into binaries by CI. Contributions are applied
upstream and credited — [CONTRIBUTING.md](CONTRIBUTING.md).

---

Questions: **hello@contenox.com**

**Advance with excellence.**

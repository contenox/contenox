---
title: Host an external ACP agent
description: Register Codex or another ACP agent, verify its connection, and dispatch work from the terminal or Contenox TUI.
order: 1
---

# Host an external ACP agent

To use subscription models with Contenox's own tools and agent loop instead of
launching a separate agent, see [ChatGPT subscription](/docs/integrations/providers/chatgpt/).

Contenox can act as an ACP client and drive another agent over stdio. The
external agent runs as a subprocess inside Contenox's Linux sandbox; Contenox
opens the ACP session, streams its updates, answers its reverse requests, and
tears it down when the turn ends.

This is the inverse of `contenox acp`:

```text
editor ──ACP client──> contenox acp
contenox agent check ──ACP client──> external ACP agent
```

## Register an agent

Install the ACP agent by its own documented method, then record the command
Contenox should spawn. Everything after `--` is preserved as its argv:

```bash
contenox agent add claude -- npx -y @zed-industries/claude-code-acp
contenox agent add goose -- goose acp
contenox agent add company-agent -- /opt/company/bin/agent --stdio
```

Contenox does not install the program. A launcher such as `npx` may download it
when started, so pin the package version when reproducibility matters.

Inspect the stored run specification:

```bash
contenox agent show claude
contenox agent list
```

The row has kind `external_acp`. Markdown declarations compiled into Contenox
chains have kind `chain`; both kinds appear in the same list.

### Register Codex

On Linux, install the [Codex ACP adapter](https://github.com/agentclientprotocol/codex-acp)
and confirm its executable is on your `PATH`:

```bash
npm install -g @agentclientprotocol/codex-acp
command -v codex-acp
```

Authenticate Codex before launching it through Contenox. If you use the Codex
CLI and have not signed in, run `codex login`. The sandbox grants access to
`~/.codex`, including the writes Codex needs for its state database and temporary
files; no manual sandbox carve-out is needed.

Register the adapter and check the connection from your project directory:

```bash
contenox agent add codex -- codex-acp
contenox agent show codex
contenox agent check codex "Reply with hello; do not use tools." --timeout 120s
```

Register once; reuse the name `codex` for subsequent checks and missions.
The command after `--` must speak ACP over stdio, which is what the adapter
provides. `agent add` only stores that command; it does not install the adapter.

## Verify the connection

`agent check` drives one real `initialize → session/new → session/prompt` turn:

```bash
contenox agent check claude
contenox agent check claude "Reply with your name and ACP version"
contenox agent check claude --timeout 30s
```

The reply streams to stdout. The final report names the agent implementation,
stop reason, and any MCP servers the peer could not accept. Add `--verbose` to
include the agent's advertised slash commands.

A check never approves an action. When the agent requests permission, Contenox
returns a proper ACP rejection and reports the operation. Use a mission for
work that needs permission approvals.

## Use an external agent as the main session agent

After registering Codex, start a conversation with it in the Contenox TUI:

```bash
contenox beam --agent codex
```

Your prompts go directly to Codex through the ACP host. Contenox provides the
TUI, session history, sandbox, and permission handling. `--agent` always starts
a fresh session, so it cannot be combined with `--session`.

Inside the TUI, `/new codex` starts another session with that registered agent.
Bare `/new` starts a native Contenox session. The agent is fixed for each
session; use `/sessions` or `contenox beam --session <id>` to reopen an existing
one. Other registered agents work the same way: substitute their name for
`codex`.

The TUI currently still requires Contenox's model setup to be complete even
when the selected external agent provides its own model.

## Dispatch work

Use the registered agent name to fire a mission under an authored envelope:

### From the TUI

Start the TUI in the project you want the agent to work on:

```bash
contenox beam
```

Enter this in the TUI's prompt:

```text
/mission --policy hitl-policy-strict.json codex Review the current changes for bugs. Do not edit files. Report actionable findings with file paths.
```

This dispatches Codex as a mission; it does not switch the agent answering your
main conversation. Reports stream back into the TUI while you continue working.
For local stdio agents, Contenox supplies a mission-scoped MCP server for reports,
questions, plans, and completion. Questions use the existing approvals inbox.
The agent cannot choose another mission's ID. Endpoint-connected agents are not
supported for mission dispatch.

Mission tools do not expand filesystem access. Repository metadata outside the
workspace remains outside the sandbox; a review requiring it needs an accessible
patch or a workspace selected with the appropriate scope. The MCP stdio helper
runs the Contenox executable, which must be executable within the existing
sandbox (for example, installed under `/usr/local/bin` or inside the workspace).

Keep the TUI open: the mission's subprocess stops when its host exits, while
the mission record and reports remain available.

Type `/mission` to open the agent and policy picker. Use Up/Down to choose
and Enter or Tab to insert a choice. Selecting `--policy` offers available
envelopes, then agents. After choosing an agent, type the intent and press
Enter to dispatch; selecting options alone never fires a mission. Esc closes
the picker and Ctrl+C clears the draft.
`contenox init` seeds `execution.missions.permissions.policy` with
`hitl-policy-default.json` when unset, preserving existing choices. This shipped
envelope permits ordinary reads and asks before writes or shell commands.
`contenox doctor` warns when the setting is missing and prints the repair command.
Without a configured default mission policy, `/mission` immediately lists
the available envelopes; selecting one inserts `--policy <envelope>`.

To see the command grammar, current defaults, and available envelopes as a
message, type `/mission`, press Esc, then Enter. Put `--policy` before the agent
name and intent. The TUI form does not take `--wait`; the session is already its
long-lived host.

### From the terminal

From the project directory, give the registered Codex agent a work order:

```bash
contenox mission fire codex \
  "Review the current uncommitted changes for bugs. Do not edit files. Report actionable findings with file paths." \
  --policy hitl-policy-strict.json \
  --wait --timeout 15m
```

Contenox launches Codex in the project workspace, supplies mission tools, and
records its reports and outcome. An explicit `cwd` in the agent's run config
overrides the project directory.

`--policy` selects the permission envelope. `--wait` keeps the CLI process
hosting Codex alive until the mission finishes; it is required for this path.
`--timeout 15m` bounds that wait. Closing the process or reaching the timeout
stops the child agent, but preserves the mission record and reports. When the
mission finishes, the command prints its verdict and report summaries.

To request implementation work, replace the intent with the change and
validation you want, such as "Fix the failing parser test and run the relevant
tests."

The mission host drives the ACP session and supplies its mission tools. The
envelope governs permission requests and compute bounds; the subprocess stays
inside the same sandbox used by `agent check`. See [Missions](/docs/guide/missions/)
for the lifecycle, reports, and approval flow.

### Approvals and results

Keep the mission host running. If it needs approval, use another terminal to
inspect the pending ask and approve the action you want to allow:

```bash
contenox approvals list
contenox approvals respond <ask-id> --approve
```

Replace `<ask-id>` with the ID from `approvals list`. Use `--deny` to refuse a
permission request, or `--answer "your response"` for a question.

Read the results afterwards, or inspect progress while the mission is running:

```bash
contenox mission list
contenox mission show <mission-id>
contenox mission reports <mission-id>
```

Use the mission ID printed by `mission fire` or shown in `mission list`.
`mission show` includes the status and report summaries; `mission reports`
shows the reports filed by the agent.

### Delegate from a Contenox agent

A Contenox agent with access to mission tools and permission to dispatch can
orchestrate a registered external agent. In `contenox beam`, ask:

> Delegate a review of the current uncommitted changes to the registered agent
> `codex`, using `hitl-policy-strict.json`. Tell it not to edit files and to
> report actionable findings with file paths. Read its reports and summarize
> the findings for me.

The supervisor calls `mission_start` with the agent, policy, and intent, waits
for its outcome, and uses the returned status and report summaries for the next
step. The child has a fresh context, so its instruction must be self-contained.
The supervisor can inspect its missions with `mission_list` and answer questions
with `mission_answer` when the envelope permits agent answers; permission
approvals remain separately governed.

## Configure the run

Open the persisted JSON in your editor:

```bash
contenox agent edit claude
```

For automation, provide the replacement document explicitly:

```bash
contenox agent edit claude --config-file ./claude-agent.json
printf '%s' '{"transport":"stdio","command":"goose","args":["acp"]}' |
  contenox agent edit goose --config-file -
```

The complete stdio shape is:

```json
{
  "transport": "stdio",
  "command": "my-acp-agent",
  "args": ["--stdio"],
  "cwd": "/absolute/workspace",
  "env": {"NAME": "value"},
  "mcp_servers": ["notion", "filesystem"]
}
```

`cwd` fixes the subprocess sandbox to one workspace. When it is absent,
Contenox uses the directory of the turn. Values in `env` are stored in the
local database as written; do not put credentials there. The sandbox already
provides read-only access to `~/.claude` and `~/.config/goose`. It provides
read-write access to `~/.codex` because Codex requires writable SQLite state
and temporary runtime files there. This grant includes Codex configuration and
credentials and applies to every external agent; no manual carve-out is needed.

`mcp_servers` is an exact allowlist of names from `contenox mcp list`. Contenox
resolves every name before spawning the agent and passes compatible servers in
ACP `session/new`. A missing name fails loudly. HTTP and SSE servers are
forwarded only when the external agent advertises that transport; stdio is the
protocol baseline.

Endpoint transport is represented in the stored format but is not implemented
by the host. Use `stdio`.

## Confinement

Foreign ACP processes run only where the [sandbox wall](/docs/guide/confinement/sandbox/)
is available. The filesystem and executable boundary fails closed, the
environment is scrubbed, and `~/.contenox` is excluded so an agent cannot reach
the policy and state controlling it. The current wall is Linux-only; an
external agent will be refused on an unsupported host.

The default wall permits network access because many agents need a model API.
Use the sandbox's network allowlist when the process also needs network
confinement.

## Lifecycle

```bash
contenox agent disable claude
contenox agent enable claude
contenox agent remove claude
```

Disabling prevents normal agent resolution while retaining the run
specification. `agent check` can still test a disabled entry and prints that it
is doing so. Removing a manually registered external agent deletes its local
record; it does not uninstall the program.

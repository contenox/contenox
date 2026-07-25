---
title: HITL Policies
description: Control which tool calls require human approval using named policy presets.
---

# HITL Policies

Human-in-the-loop (HITL) lets you intercept tool calls before they execute and decide — approve, block, or let them pass automatically — based on a named policy file.

## How it works

HITL is **on by default**. When the engine runs a tool call, it evaluates the active policy and takes one of three actions:

- **approve** — pause and prompt the user; execution continues only after explicit approval
- **allow** — pass through silently (no prompt)
- **deny** — reject the call immediately without prompting

In the **CLI**, approval prompts appear inline in the terminal (TTY):

![A destructive rm command stops at a human approval gate before it runs](/hitl-approve.gif)

To disable HITL entirely, pass `--auto`:

```bash
# HITL on (default) — the engine pauses before destructive tool calls:
contenox chat --shell "refactor main.go"

# HITL off — autonomous mode, no approval prompts:
contenox chat --shell --auto "refactor main.go"
contenox run  --auto --chain ./my-chain.json "do the thing"
```

> [!WARNING]
> `--auto` disables all approval prompts. Use only in trusted environments or non-interactive scripts.

## Policy file format

A policy is a JSON file with an optional `default_action` and a list of `rules`:

```json
{
  "default_action": "deny",
  "rules": [
    { "tools": "local_fs",    "tool": "write_file",  "action": "approve" },
    { "tools": "local_fs",    "tool": "sed",         "action": "approve" },
    { "tools": "local_shell", "tool": "local_shell", "action": "approve" },
    { "tools": "webtools",    "tool": "web_post",    "action": "approve" },
    { "tools": "webtools",    "tool": "web_put",     "action": "approve" },
    { "tools": "webtools",    "tool": "web_patch",   "action": "approve" },
    { "tools": "webtools",    "tool": "web_delete",  "action": "approve" }
  ]
}
```

| Field | Type | Description |
|---|---|---|
| `default_action` | `"allow"` \| `"approve"` \| `"deny"` | Action for tool calls that match no rule. Fail-closes to `"approve"` if omitted — an unaccounted-for tool call pauses for a human rather than running. |
| `rules[].tools` | string | Tools name (`local_fs`, `local_shell`, a remote tool name, …) |
| `rules[].tool` | string | Tool name within that tool (`write_file`, `sed`, `local_shell`, …) |
| `rules[].action` | `"approve"` \| `"allow"` \| `"deny"` | What to do when this rule matches |
| `rules[].when` | array | Optional conditions on the call's arguments; **all** must hold for the rule to match (AND). Each is `{ "key": …, "op": …, "value": … }`. Ops include `command_blacklist`, `command_ask_always`, and `no_command_substitution`. Omit for a name-only match. |
| `rules[].timeout_s` | int | Seconds to wait for a human response when `action` is `approve`. `0` (default) waits indefinitely until the context is cancelled. |
| `rules[].on_timeout` | `"approve"` \| `"deny"` | Fallback action when an approval window expires. `"allow"` is rejected (it would silently bypass approval). |

Rules are evaluated top-to-bottom; the first match wins.

A rule with `when` conditions gates a tool only for calls whose arguments match — for example, prompting only for shell commands in a blacklist:

```json
{ "tools": "local_shell", "tool": "local_shell", "action": "approve",
  "when": [{ "key": "command", "op": "command_ask_always", "value": "rm,sudo,dd,chmod" }] }
```

## Who may answer a unit's question (`attention`)

A mission unit that hits something it must not decide alone calls its
`mission.mission_ask_attention` tool. That question lands in the [approvals
queue](/docs/reference/contenox-cli/#contenox-approvals) **and** in the session
that fired the mission, where you answer it in place — your words go straight
back to the unit as the result of the call it is parked on, and it continues.

By default only a **human** may answer, because that is what the unit escalated
for. An envelope can hand routine questions to the **agent that fired the
mission** instead — it often already knows the answer from the conversation the
mission was fired in:

```json
{
  "default_action": "approve",
  "rules": [],
  "attention": { "allowAgentAnswers": true, "maxAgentAnswers": 2 }
}
```

| Field | Type | Description |
|---|---|---|
| `attention.allowAgentAnswers` | bool | Let the firing session's agent answer its own unit's questions. Omitted/`false` (the default) means a human must. You can always answer yourself either way — this only decides whether the agent is offered the question first. |
| `attention.maxAgentAnswers` | int | How many of this mission's questions the agent may answer before the rest wait for a human. Omitted uses a small default (3); `0` is **not** unlimited. The count is durable and actor-aware, so a restart does not refill it and your own answers do not consume it. |

The agent is also skipped when the firing session is busy with a turn you
started, or is not currently open — a question is never silently swallowed by an
agent-to-agent exchange you cannot see. When the agent does answer, it happens as
a visible turn in your transcript, and the durable ask records that an agent (not
a person) answered it.

## Built-in presets

Contenox ships five policy presets, written to `~/.contenox/` by `contenox init`. (A workspace `.contenox/` file with the same name overrides the global one.) The first three are the general-purpose postures; the last two are the profiles the ACP editor transports load.

| Name | Behaviour |
|---|---|
| `hitl-policy-default.json` | Prompts for filesystem writes, `sed`, shell commands, and the mutating webtools verbs (`web_post`, `web_put`, `web_patch`, `web_delete`); allows reads (`read_file`, `list_dir`, `grep`, `stat_file`, `count_stats`) and the safe webtools verbs (`web_get`, `web_head`); anything not matched by a rule fail-closes to approval (`default_action: "approve"`) |
| `hitl-policy-strict.json` | Deny-by-default; only the rules listed are prompted |
| `hitl-policy-dev.json` | `default_action: allow`, but explicit rules still gate `local_shell` (every shell call requires approval, and a fixed blacklist is always denied); useful for local development when you don't want prompts on filesystem/webtools calls |
| `hitl-policy-acp.json` | Profile for editor (ACP) sessions — gated tool calls route through the editor's own approval UI |
| `hitl-policy-acpx.json` | Hardened profile for headless / untrusted-driver (ACPX, e.g. OpenClaw) sessions — shell, writes, and network are denied outright rather than offered for approval |

## Selecting the active policy

```bash
contenox config set hitl-policy-name hitl-policy-strict.json
contenox config get hitl-policy-name   # verify
```

This writes to the KV store and takes effect immediately — no restart required. The setting applies to all subsequent invocations in the same workspace.

## Policy resolution order

When HITL is enabled and a tool call needs evaluation, the engine resolves the policy as follows:

1. Read the `hitl-policy-name` key from the KV store.
2. If set, load that file from the workspace `.contenox/` directory, falling back to `~/.contenox/`.
3. If the key is empty or the file is missing, fall back to `hitl-policy-default.json`.
4. If that file is also missing, use a built-in fail-closed policy (same shape as `hitl-policy-default.json`: reads and safe webtools verbs are allowed, everything else — including any tool call that matches no rule — requires approval).

## Previewing a verdict without running anything

`contenox workspace access` asks a running `contenox serve` what a policy would decide for a read and a write to one or more paths, without touching the filesystem or prompting for approval — useful for checking a policy's `rules` actually cover what you think they do before pointing an agent at a workspace:

```bash
contenox workspace access --policy hitl-policy-strict.json src/main.go .ssh/id_rsa
```

```
Policy: hitl-policy-strict.json
PATH         REACHABLE  READ   WRITE    REASON
src/main.go  true       allow  approve  write:matched_rule#30
.ssh/id_rsa  true       deny   deny     read:matched_rule#0 write:matched_rule#0
```

See [`contenox workspace`](/docs/reference/contenox-cli/#contenox-workspace) in the CLI reference for the full flag list.

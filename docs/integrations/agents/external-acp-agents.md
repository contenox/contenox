---
title: Host external ACP agents
description: Register a foreign Agent Client Protocol agent — Claude Code, Goose, or your own — and let contenox spawn, drive, and gate it from the CLI and Beam.
---

# Host external ACP agents

Contenox speaks the [Agent Client Protocol](https://agentclientprotocol.com/)
(ACP) in both directions. The [editor guides](/docs/integrations/editors/zed/)
cover the *upward* direction — contenox running as the agent your editor drives.
This page covers the inverse: contenox as the **client** that registers, spawns,
and drives another ACP agent, then hands you the same gated surface you already
use for your own chains.

An agent here is an external program that speaks ACP over stdio — Claude Code
(via `claude-code-acp`), Goose, or a binary of your own. Contenox never installs
one: it invokes a binary you already have on `PATH`, or lets a runtime fetcher
(`npx`/`uvx`) pull it down. Once registered, an agent is driveable three ways:

- `contenox agent check` — a one-shot live turn from the CLI to verify it works.
- **Beam** — chat with it in the browser, gated by your HITL policy (below).
- `GET /api/agents` — the read-only REST list, also rendered in the [`/docs`
  OpenAPI UI](/openapi.json).

This page assumes `contenox` is already on `PATH`. If not, do the
[Quickstart](/docs/guide/quickstart/) first.

---

## Register an agent

There are exactly two ways to register one, and no `--transport`/`--env`/`--args`
flag soup: seed from the ACP registry catalog, or give a bare command.

### From the registry catalog

Browse the catalog and register an entry by id. Contenox resolves the catalog
entry for your OS/arch into a run spec — `npx`/`uvx` entries become the fetcher
invocation; a binary entry becomes the binary basename (which you must already
have on `PATH`).

```bash
contenox agent search              # list the whole catalog
contenox agent search claude       # filter by id / name / description
contenox agent add claude-acp      # register it (named after the registry id)
contenox agent add goose --name my-goose   # …under an alias
```

The catalog is cached locally next to the database (`agent-registry.json`); pass
`--refresh` on `search` or `add` to force a re-fetch.

### From a bare command

Everything after `--` is the argv contenox will spawn. This is the only
raw-command path.

```bash
contenox agent add local-bot -- /usr/local/bin/my-acp-agent --stdio
```

The name is the positional *before* `--`; the command and its arguments come
*after* it.

### Customize later

Both forms register a JSON run spec (`config_json`). To change the command, args,
environment, working directory, transport, or the forwarded MCP servers, edit
that JSON directly — there is no per-field flag:

```bash
contenox agent edit my-goose                       # opens $EDITOR, validates on save
contenox agent edit my-goose --config-file cfg.json  # or non-interactively
```

The [CLI reference](/docs/reference/contenox-cli/#the-agent-run-config-config_json)
documents every field. Provenance (whether it came from the registry or a bare
command, and which catalog entry) is system-managed and shown by `agent show` /
`agent list`, but is never part of the editable JSON.

---

## Verify it with `agent check`

Right after registering, drive one live turn to confirm the agent actually
answers:

```bash
contenox agent check my-goose            # a plain connection check
contenox agent check claude Say hello    # everything after the name is the prompt
```

`check` spawns the agent as a subprocess and runs a full
`initialize → session/new → session/prompt` turn against it — the same
client-host path (`runtime/agenthost`) the runtime uses when you chat with the
agent in Beam, not a lighter fake — and streams the reply to stdout. It then
prints the stop reason and any slash-commands the agent advertised:

```
Checking agent "my-goose": npx -y @acme/goose-acp

Hello! I'm connected and ready.

Turn completed (agent goose 1.4.0, stopReason=end_turn).
Agent advertises 2 command(s): /compact /review
```

The turn is rooted in the current directory and drives one plain-text prompt.
Agent-initiated callbacks (file system, terminal, permission requests) are
declined, so an agent that insists on them may stop early — a simple prompt
should not need any. A normal turn that produces no displayable output fails the
check rather than reporting a false success on a silent agent. Use `--timeout`
(default `2m`) to bound a slow agent.

`agent check` is the user-facing twin of the `make acp-host-e2e` harness that
pins this path in CI — see [the ACP client library](/docs/development/acp-client/)
for the full verification story.

---

## Forwarding MCP servers

An agent's config can declare an `mcp_servers` allowlist: registered MCP server
names (`contenox mcp list`) that contenox forwards to the agent in ACP
`session/new`. This is the mirror of what contenox consumes when it is on the
agent side of the same exchange.

```json
{
  "transport": "stdio",
  "command": "claude-code-acp",
  "mcp_servers": ["filesystem", "notion"]
}
```

Forwarding is an explicit, per-agent **consent boundary**, named server by named
server:

- There is deliberately **no wildcard** — you list each server the agent may
  reach, one by one.
- Forwarding a server hands the agent everything it needs to connect to it: argv
  for a stdio server, the URL and configured headers (which may carry auth) for
  an http/sse server.
- Contenox-side auth *synthesis* — `authToken`/`authEnvKey`, OAuth, injected
  hidden params — is **never** translated into the forwarded payload. Forwarding
  the server at all is the consent; contenox does not additionally hand over its
  own credential machinery.
- Names resolve loudly: a missing name fails the turn rather than silently
  shrinking the agent's declared context. A server whose transport the agent's
  advertised capabilities cannot consume is *reported* as not-forwarded, not
  silently dropped.

`agent check` exercises this too — it prints a `Forwarding MCP servers:` line and
notes any the agent could not consume — so you can confirm the allowlist before
the agent runs for real.

---

## Drive it from Beam

Enabled agents appear in [Beam](/docs/guide/beam/#chat-with-a-registered-agent):
the sidebar's **New chat with an agent** chevron opens a picker listing the
native contenox chain plus every registered agent. Pick one and the session binds
to that agent — its turns stream into the transcript, its tool calls render as
cards, and any permission it requests raises the same inline
[approval card](/docs/guide/beam/#the-approval-gate) your own chains do, gated by
the session's contenox HITL policy. The session row is labelled `Agent: {name}`
so you can tell which sessions drove a foreign agent.

---

## Manage the roster

```bash
contenox agent list                # id, name, source, kind, enabled
contenox agent show my-goose       # provenance + run command + config_json
contenox agent disable my-goose    # keep it registered but hide it from the picker
contenox agent enable my-goose
contenox agent remove my-goose     # (alias: rm) — removes only the local registration
```

`remove` deletes the local registration only; it never touches the binary or
package the agent would spawn. `disable` keeps the registration but drops the
agent from Beam's picker.

---

## Next steps

- [`contenox agent` CLI reference](/docs/reference/contenox-cli/#contenox-agent) — every subcommand and config field
- [Beam guide](/docs/guide/beam/#chat-with-a-registered-agent) — chatting with a registered agent
- [MCP servers](/docs/integrations/tools/mcp/) — register the servers you forward with `mcp_servers`
- [The ACP client library](/docs/development/acp-client/) — how the host path and its harnesses are built

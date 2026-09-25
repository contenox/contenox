# The ACP client library and its verification harnesses

`libacp` implements the Agent Client Protocol (ACP) v1 for **both roles** of
the conversation:

- **Agent side** — `libacp.AgentSideConnection` serving a `libacp.Agent`.
  This is the upward direction: `internal/surfaces/acpsvc` implements the production
  agent that ACP clients (Zed, JetBrains, the terminal UI) drive via
  `contenox acp`.
- **Client side** — `libacp.ClientSideConnection` driving a `libacp.Client`.
  This is the downward direction: contenox itself opening sessions on other
  ACP agents.
  The client exposes every agent-bound method (`Initialize`, `NewSession`,
  `Prompt`, `CancelPrompt`, session config options, ext-method passthrough)
  as outbound calls, receives streamed `session/update` notifications in
  wire order, and answers the agent's reverse calls
  (`session/request_permission`, `fs/*`, `terminal/*`).

> **Rule:** session-level features — slash commands, session controls, the
> affordances an operator reaches for mid-conversation — belong in
> `internal/surfaces/acpsvc` (registry: `commands.go`), never in a client. A
> client consumes `AvailableCommands` from ACP and its local `/`-handling is
> input classification only; implementing a feature there would give that one
> client something `contenox acp` and every ACP editor silently lack. Verify
> the feature through `contenox acp` before calling it done.

The package documentation (`libacp/doc.go`) carries a compact end-to-end
client example. `libacp/acpexec` provides the subprocess-over-stdio
transport both roles use to reach a peer binary.

## E2E harnesses against the Rust reference SDK

The in-repo tests exercise both halves against each other (in-process fakes,
plus a production loopback in `internal/surfaces/acpsvc/client_loopback_test.go` that
runs the real `acpsvc` agent against the real `ClientSideConnection`). Two
additional, opt-in harnesses validate each role against **independently
implemented** peers from the reference Rust SDK
([github.com/agentclientprotocol/rust-sdk](https://github.com/agentclientprotocol/rust-sdk)):

| Target | Validates | Peer binary |
| --- | --- | --- |
| `task acp-conformance` | the **agent** side (`libacp/cmd/acp-stub-agent` via `AgentSideConnection`) | `acp-validator`, a conformance-checking ACP client |
| `task acp-client-e2e` | the **client** side (`ClientSideConnection` over `acpexec`) | `testy`, the SDK's deterministic test agent |

Both targets require their peer binary when invoked directly. `task test-all`
reports missing peers as skipped suites. Set the environment variables below,
or build the peers under `tools/` for automatic discovery:

- `ACP_TESTY_BIN` / `ACP_MCP_ECHO_BIN` — build from a rust-sdk checkout with
  `cargo build -p agent-client-protocol-test --bins`; the binaries land at
  `<checkout>/target/debug/testy` and `<checkout>/target/debug/mcp-echo-server`.
  (`ACP_MCP_ECHO_BIN` only gates the MCP pass-down test, which skips on its
  own if unset.)
- `ACP_VALIDATOR_BIN` — the validator is not part of the SDK; its source is
  vendored in [`tools/acp-validator/`](../../tools/acp-validator/README.md)
  in this repository. It depends on the SDK's `agent-client-protocol` crate
  by relative path, so copy it next to a rust-sdk checkout and `cargo build`
  there (the README has the exact steps). `ACP_YOPO_BIN` (optional,
  additional client) builds from the rust-sdk's `src/yopo`.

## The composed host e2e (registry → agenthost → live turn)

One layer above the wire-dispatch harnesses, `task acp-host-e2e` validates the
runtime's **client-host composition** end to end: an `agents` row created and
resolved through the real registry service, spawned and driven by
`internal/services/agenthost.DriveTurn` (initialize → session/new → session/prompt →
teardown), with the streamed reply asserted on the caller's harness.
Servers, each isolating something different:

| Server | Gate | Asserts |
| --- | --- | --- |
| `acp-stub-agent` (hermetic) | none — runs in plain `go test` | deterministic "ack" turn, update ordering through the harness seam |
| contenox self-loopback (`contenox acp` built in-test, driving a no-model chain fixture) | currently skipped: its isolated home conflicts with the confined spawn path | intended to check the byte-exact fixture reply; not established by this test |
| `testy` | `ACP_TESTY_BIN` | deterministic echo/greet through the composed path |
| Claude Code (via `claude-code-acp`) | `ACP_CLAUDE_ACP_BIN` — never CI; needs Claude credentials | turn **shape** only: `end_turn` plus displayable output from a real, foreign production agent |

The user-facing twin of this harness is `contenox agent check <name>`: it
drives the same DriveTurn path against any registered agent and streams the
reply — the way to verify an agent right after `contenox agent add`.

### MCP forwarding and the agent's command surface

Forwarding a registered MCP command does not grant it permission to execute
inside a foreign agent's sandbox. Its executable and dependencies must be
reachable under the [sandbox configuration](/docs/guide/confinement/sandbox/).
The composed-host test must exercise that confinement as well as the ACP
forwarding contract; a passing wire-only MCP test does not prove both.

An agent row's `mcp_servers` config field is an explicit, per-agent allowlist
of registered MCP server names (`contenox mcp list`) forwarded to that agent
in ACP `session/new` — the mirror of what `internal/surfaces/acpsvc` consumes when
contenox is on the *agent* side of the same exchange. The host
(`agenthost.ResolveForwardedMcpServers` + DriveTurn) resolves names loudly
(a missing name fails the turn rather than silently shrinking the agent's
declared context), filters by the agent's initialize-advertised
`mcpCapabilities` (stdio is baseline; http/sse gated), and reports
forwarded-vs-dropped on the TurnResult. Contenox-side auth synthesis
(authToken/authEnvKey/oauth/injectParams) is never translated into the
payload; forwarding a server at all is the consent boundary. The composed
pass-down is pinned by `TestHostE2E_Testy_McpPassDownThroughComposedPath`
(testy connects to the forwarded `mcp-echo-server` and lists its tools).

The slash commands a hosted agent advertises (`available_commands_update`)
are recorded by the harness (`RecordingHarness.AvailableCommands`) and
printed by `agent check`. *Merging* them with contenox's own acpsvc command
set is not implemented: acpsvc's leading-slash interception is the natural
merge point, and a collision policy is required since command sets can
overlap (claude-code-acp and acpsvc both advertise `/compact`).


## Native session settings

Read the session's returned `configOptions` instead of hard-coding names or
values. Contenox advertises dotted IDs corresponding to `contenox config` names, with
human-readable names, descriptions and current choices. The model select carries
a `provider/model` pair; saved defaults store `inference.provider` and
`inference.model` separately. Context and output
selects distinguish `inherit` from `0` (automatic). A context request remains a
request when the model changes; capacity is resolved again for each turn.

`session/set_config_option` returns the refreshed option set. Replace the
client's previous set, including descriptions: a model change can change
capacity. The model and reasoning selects retain ACP's `model` and
`thought_level` categories. Older IDs `model`, `think`, `hitl-policy` and
`token-limit` remain accepted for native sessions. The output select uses
`inference.generation.max_output_tokens` and also accepts `max-tokens`.

Changes are local to the live native session and do not save machine or
workspace defaults. The initialize metadata advertises the same inherited
choices before session creation. External agents retain their own option IDs;
Contenox's surrounding permission-policy select remains local to Contenox.

See [configuration](/docs/reference/config/#current-session) for the slash-command
equivalents and persistence rules.

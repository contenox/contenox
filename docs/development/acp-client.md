# The ACP client library and its verification harnesses

`libacp` implements the Agent Client Protocol (ACP) v1 for **both roles** of
the conversation:

- **Agent side** — `libacp.AgentSideConnection` serving a `libacp.Agent`.
  This is the upward direction: `runtime/acpsvc` implements the production
  agent that editors (Zed, beam, the VS Code bridge) drive via
  `contenox acp`.
- **Client side** — `libacp.ClientSideConnection` driving a `libacp.Client`.
  This is the downward direction: contenox itself opening sessions on other
  ACP agents (see the blueprint in
  [blueprints/acp/acp-client-engine.md](blueprints/acp/acp-client-engine.md)).
  The client exposes every agent-bound method (`Initialize`, `NewSession`,
  `Prompt`, `CancelPrompt`, session config options, ext-method passthrough)
  as outbound calls, receives streamed `session/update` notifications in
  wire order, and answers the agent's reverse calls
  (`session/request_permission`, `fs/*`, `terminal/*`).

The package documentation (`libacp/doc.go`) carries a compact end-to-end
client example. `libacp/acpexec` provides the subprocess-over-stdio
transport both roles use to reach a peer binary.

## E2E harnesses against the Rust reference SDK

The in-repo tests exercise both halves against each other (in-process fakes,
plus a production loopback in `runtime/acpsvc/client_loopback_test.go` that
runs the real `acpsvc` agent against the real `ClientSideConnection`). Two
additional, opt-in harnesses validate each role against **independently
implemented** peers from the reference Rust SDK
([github.com/agentclientprotocol/rust-sdk](https://github.com/agentclientprotocol/rust-sdk)):

| Target | Validates | Peer binary |
| --- | --- | --- |
| `make acp-conformance` | our **agent** side (`libacp/cmd/acp-stub-agent` via `AgentSideConnection`) | `acp-validator`, a conformance-checking ACP client |
| `make acp-client-e2e` | our **client** side (`ClientSideConnection` over `acpexec`) | `testy`, the SDK's deterministic test agent |

Both targets skip-or-fail cleanly when their binary env var is unset:

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

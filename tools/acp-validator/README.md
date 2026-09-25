# acp-validator

A conformance-checking ACP client used to validate ACP **agent** (server-side)
implementations — this is what `task acp-conformance` runs against
`libacp/cmd/acp-stub-agent` (see `libacp/agentconformance_test.go`).

It is not vendored as a buildable Go module; it's a standalone Rust binary
whose source lives here so it survives outside any one session's scratch
directory. It depends on the `agent-client-protocol` crate from
[agentclientprotocol/rust-sdk](https://github.com/agentclientprotocol/rust-sdk)
via a relative path, so build it as a sibling of a rust-sdk checkout:

```sh
git clone https://github.com/agentclientprotocol/rust-sdk ../rust-sdk   # sibling checkout
cp -r tools/acp-validator ../acp-validator                              # or symlink
cd ../acp-validator && cargo build
# binary at ../acp-validator/target/debug/acp-validator
```

Point `ACP_VALIDATOR_BIN` (and optionally `ACP_YOPO_BIN`, built from the same
rust-sdk's `src/yopo`) at the resulting binary and run `task acp-conformance`
from the Contenox repository root. The dependency version in `Cargo.toml` is
the Rust SDK version, not the ACP wire protocol version.

To build the other reference peers from the SDK checkout:

```sh
cargo build -p agent-client-protocol-test --bins
cargo build -p agent-client-protocol-yopo
```

Set `ACP_TESTY_BIN` to `target/debug/testy`, `ACP_MCP_ECHO_BIN` to
`target/debug/mcp-echo-server`, and `ACP_YOPO_BIN` to `target/debug/yopo`, using
absolute paths. Run `task acp-suites` from the Contenox repository root to
exercise both protocol roles and host composition. Read the skipped-test
output: missing peers and optional external agents are not passing checks.

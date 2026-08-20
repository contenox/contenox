# acp-validator

A conformance-checking ACP client used to validate ACP **agent** (server-side)
implementations — this is what `make acp-conformance` runs against
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
rust-sdk's `src/yopo`) at the resulting binary and run `make acp-conformance`.

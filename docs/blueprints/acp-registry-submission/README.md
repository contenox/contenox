# ACP Registry Submission Draft

This directory contains the files to copy into a fork of
`agentclientprotocol/registry`:

```text
contenox/
  agent.json
  icon.svg
```

The manifest targets the current release in `runtime/version/version.txt`.
Before opening the registry PR, verify the release asset URLs still return
`200` and run the registry validator from the registry checkout:

```bash
uv run --with jsonschema .github/workflows/build_registry.py
uv run --with jsonschema --with agent-client-protocol \
  .github/workflows/verify_agents.py --auth-check --agent contenox
```

Current local checks:

- `v0.28.1` release asset URLs returned `200` on 2026-06-06.
- `contenox-linux-amd64.tar.gz` contains a single executable named `contenox`.
- `icon.svg` is 16x16, square, and uses `fill="currentColor"`.


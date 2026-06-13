# Runtime Prune Plan: HTTP UI And Proxy Surface

Goal: make the V1 runtime smaller and clearer by removing surfaces that compete
with the CLI, ACP, and VS Code extension story.

## Remove

- `contenox serve`
- Beam/browser UI
- local HTTP API routing and middleware
- OpenAI/Ollama-compatible model proxy routes
- generated local OpenAPI docs/spec/SDK surface
- API smoke-test harness tied to the removed server
- Makefile targets that only exist for those surfaces
- public docs that tell users to run the deleted server

## Keep

- CLI commands used by local workflows
- ACP service and editor integration behavior
- VS Code `vscode-agent --stdio`
- MCP server configuration and worker sessions
- OpenAPI-backed remote tools used from chains
- local shell, local filesystem, web, echo, and print tools
- providers, model registry, sessions, HITL policy, and local SQLite state
- release binaries and VS Code package workflows

## Guardrails

- Keep a snapshot outside the active repo before deletion.
- Delete tests that only prove the removed HTTP surface.
- Preserve tests that cover runtime behavior independent of HTTP.
- Update README, CONTRIBUTING, CLI docs, site docs, and marketplace README.
- Run CLI help drift checks after deleting commands or flags.

## Validation

Minimum checks after the prune:

```bash
make build-contenox
make test-unit
make test-contenox-help
make package-vscode
```

Broader release checks live in `docs/blueprints/v1-feature-map.md` and
`docs/blueprints/vscode-marketplace-release.md`.

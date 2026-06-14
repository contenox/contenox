# Contenox V1 Feature Map

Date: 2026-06-13

This document maps the current Contenox feature surface so it can be converted
into a manual testing plan for a V1 release. It is intentionally broader than a
test checklist. The goal is to make every user-visible and release-sensitive
surface explicit before deciding what must be tested, documented, hidden, or
deferred.

## V1 Product Boundary

Contenox V1 is a local-first AI workflow runtime with three primary entry
points:

- `contenox` CLI for setup, chain execution, chat, sessions, model/provider
  configuration, tools, MCP servers, and local inspection.
- ACP stdio server for editor and desktop clients that speak Agent Client
  Protocol.
- VS Code extension that bundles or launches the local runtime and exposes
  native editor surfaces.

V1 is a desktop/workspace runtime for the three major OS families:

- Linux
- macOS
- Windows

That support promise applies to both the standalone CLI runtime and the bundled
runtime inside the VS Code extension. It has direct testing implications: a V1
release cannot be validated only on the maintainer's development OS.

The current V1 direction explicitly does not include:

- `contenox serve` as a local HTTP server.
- Beam or a bundled web UI.
- OpenAI/Ollama compatibility proxy endpoints.
- Generated local OpenAPI docs for the removed HTTP server.
- A TypeScript rewrite of the Contenox engine.

Manual testing should therefore focus on local runtime behavior, stdio bridge
behavior, editor integration, and packaged native binary distribution.

## Critical User Journeys

These are the release-defining journeys. A V1 build should not ship if any of
these fail on the target platform being released.

1. Fresh user installs Contenox, runs setup, configures a provider/model, and
   gets a successful `contenox doctor`.
2. Fresh project runs `contenox init`, then runs a default one-shot prompt with
   `contenox "..."`.
3. User runs `contenox chat`, continues the same session, switches sessions,
   and sees persisted history.
4. User enables local tools intentionally, sees HITL approval where expected,
   approves a safe operation, and sees a denied unsafe operation blocked.
5. User registers and uses at least one non-shell external tool path:
   OpenAPI remote tools or MCP.
6. User installs the VS Code extension, runs `Contenox: Run Setup`, opens
   `@contenox`, asks about selected code, and receives context-aware output.
7. User triggers VS Code inline autocomplete and can diagnose an explicit skip
   or see an insertion candidate.
8. User uses an ACP client through `contenox acp` and sees editor-mediated
   permission handling for gated tools.
9. Release package contains exactly the intended runtime artifacts for the
   target platform and no dev-only files, secrets, source maps, or stale UI
   packages.
10. Release artifacts install and run on Linux, macOS, and Windows, with
    platform-specific shell, filesystem, path, executable-bit, and editor-host
    behavior verified.

## Feature Area 1: Installation, Packaging, And Versioning

### User-visible features

- Install script from `https://contenox.com/install.sh`.
- Source build through `make build-contenox`.
- CLI version via `contenox version` and `contenox --version`.
- Self-update via `contenox update`.
- Shell completion generation via `contenox completion`.
- VS Code VSIX packaging in `packages/vscode`.
- Standalone native release binaries for supported OS/architecture targets.
- Platform-specific VS Code Marketplace artifacts:
  - `linux-x64`
  - `linux-arm64`
  - `darwin-arm64`
  - `darwin-x64`
  - `win32-x64`

### Manual test implications

- Test install on a clean machine or clean container with no prior
  `~/.contenox`.
- Test the standalone CLI on Linux, macOS, and Windows before V1.
- Verify `contenox --version` matches `runtime/version/version.txt`.
- Verify `contenox update` behavior both when current and when update metadata
  says a newer release exists.
- Verify packaged VSIX contains one native binary for the target platform.
- Verify macOS/Linux packaged binary has executable permission.
- Verify Windows packaged extension contains `bin/contenox.exe`.
- Verify VS Code Remote SSH/container scenario uses a runtime path inside the
  remote environment.
- Verify CLI and VSIX release artifacts are named predictably enough for users
  to select the correct platform package from GitHub Releases.

### Release risks

- Native CGO build dependencies differ across targets.
- macOS/Linux executable bits can be lost if packaged from Windows.
- Windows path separators, shell selection, executable suffixes, process
  spawning, and quoting rules differ from Unix.
- macOS signing/notarization expectations may become a distribution issue even
  when the binary itself runs locally.
- Remote VS Code workspaces install/run the extension on the workspace host, not
  necessarily the user's laptop OS.
- VS Code Marketplace publish is currently PAT-based and must later move away
  from global PATs before December 1, 2026.

## Feature Area 2: Runtime State And Configuration

### User-visible features

- Local SQLite-backed state.
- Global Contenox data under `~/.contenox` by default.
- Project-local `.contenox/` directory for chains and project configuration.
- `--data-dir` override for explicit state directory selection.
- `--db` override for explicit SQLite database path.
- Config keys via `contenox config get|set|list`.
- Workspace-scoped keys:
  - `default-chain`
  - `hitl-policy-name`
- Global keys:
  - `default-model`
  - `default-provider`
  - `default-alt-model`
  - `default-alt-provider`
  - `default-autocomplete-model`
  - `default-autocomplete-provider`
  - `default-max-tokens`
  - `default-think`
  - `telemetry-enabled`
  - `update-check`

### Manual test implications

- Verify global provider/model config is shared across projects.
- Verify project-local chain selection does not leak into another project.
- Verify `--data-dir` isolates state, chains, sessions, telemetry, and cached
  values.
- Verify config values survive process restart.
- Verify invalid config keys and invalid values produce useful errors.
- Verify autocomplete-specific defaults are respected separately from chat
  defaults.

### Release risks

- Users can confuse global backend/model config with local chain files.
- Editor integrations need the same state model but different workspace IDs.
- Docs must make "global state vs project chain files" very clear.

## Feature Area 3: Setup And Readiness

### User-visible features

- `contenox setup` interactive wizard.
- `contenox doctor` readiness report.
- `contenox doctor --json`.
- `contenox doctor --skip-cycle`.
- VS Code `Contenox: Run Setup`.
- VS Code `Contenox: Show Status`.
- VS Code bridge `health` request.

### Supported setup targets

- Embedded local llama.cpp model.
- Local Ollama daemon.
- Hosted Ollama Cloud.
- OpenAI.
- Gemini.
- OpenRouter.
- Other providers through manual backend registration.

### Manual test implications

- Test setup from a completely clean state.
- Test setup cancellation and rerun.
- Test setup inside VS Code terminal flow.
- Test `doctor` before any backend exists.
- Test `doctor` with configured provider but missing API key.
- Test `doctor` with configured provider and reachable model.
- Test `doctor --json` has stable machine-readable fields.

### Release risks

- Setup success depends on provider network availability.
- Users need clear recovery when auth expires or an API key env var is missing.
- VS Code setup must not leave the bridge in a stale state; restart/status must
  clearly reflect the new config.

## Feature Area 4: Providers, Backends, And Models

### User-visible backend commands

- `contenox backend add <name>`
- `contenox backend list`
- `contenox backend show <name>`
- `contenox backend remove <name>`

### Backend types in the CLI surface

- `local`: embedded llama.cpp, no external server.
- `ollama`: local Ollama daemon or hosted Ollama Cloud.
- `openai`: OpenAI API.
- `openrouter`: OpenRouter API.
- `anthropic`: Anthropic API.
- `mistral`: Mistral API.
- `bedrock`: AWS Bedrock Converse API.
- `gemini`: Google Gemini API.
- `vllm`: self-hosted OpenAI-compatible endpoint.
- `vertex-google`: Google Cloud Vertex AI / Gemini.

### Model commands

- `contenox model list`: query live backends.
- `contenox model registry-list`: list curated and user-added local model
  registry entries.
- `contenox model show <name>`.
- `contenox model add <name> --url ...`.
- `contenox model remove <name>`.
- `contenox model pull <name>`.
- `contenox model pull <name> --url ...`.
- `contenox model set-context <name> --context ...`.
- `contenox model capability set|show|unset <provider> <model>`.

### Runtime capabilities

- Chat.
- Prompt-style text generation.
- Embeddings where provider supports it.
- Streaming where provider supports it.
- Thinking/reasoning mode when provider/model capability allows it.
- Manual capability overrides for provider/model pairs.
- Manual local context overrides.

### Manual test implications

- For each provider released as supported, test add/list/show/remove.
- For cloud providers, test `--api-key-env` and confirm secrets are not printed.
- Test inline `--api-key` only as legacy behavior and confirm docs prefer env.
- Test a missing env var produces actionable output.
- Test `model list` before and after adding a backend.
- Test `cache clear` after changing provider state.
- Test `model capability set ... --think true|false` changes effective
  reasoning behavior.
- Test `model set-context` accepts bare numbers and shorthand such as `32k` and
  `1m`.
- Test first local `model pull` can become the default where intended.

### Release risks

- Provider catalogs and model names change over time.
- Some provider errors are string-classified for retry behavior.
- Bedrock and Vertex require ambient cloud auth, making manual testing harder.
- Large local GGUF downloads are slow and can dominate manual release cycles.

## Feature Area 5: Chain DSL

### User-visible files seeded by `contenox init`

- `chain-contenox.json`: default chat chain.
- `chain-run.json`: default stateless run chain.
- `chain-acp.json`: default ACP/editor chain.
- `chain-acpx.json`: headless/untrusted-driver ACP chain.
- `chain-compact.json`: history compaction chain.
- `chain-fim.json`: fill-in-middle chain for editor autocomplete.
- HITL policy JSON files.

### Chain-level fields

- `id`
- `description`
- `debug`
- `token_limit`
- `tasks`

### Task fields

- `id`
- `description`
- `handler`
- `system_instruction`
- `execute_config`
- `tools`
- `print`
- `prompt_template`
- `output_template`
- `input_var`
- `transition`
- `timeout`
- `retry_on_failure`

### Task handlers

- `chat_completion`: call a model with optional tools.
- `execute_tool_calls`: execute model-requested tool calls.
- `route`: force a model to choose a route label.
- `tools`: deterministic direct tool call.
- `noop`: no-op transition point.
- `raise_error`: error path.

### Transition tokens and operators

Handler transition outputs:

- `chat_completion`: `tool_call` or `executed`.
- `execute_tool_calls`: `noop`, `no_calls_found`, `tools_executed`, or
  `failed`.
- `tools`: `tools_executed`, `failed`, or rendered `output_template`.
- `noop`: `noop`.
- `route`: the selected route label.

Operators:

- `equals`
- `contains`
- `starts_with`
- `ends_with`
- `default`
- `edge_traversed_at_least`

### Execution config fields

- `model`
- `models`
- `provider`
- `providers`
- `temperature`
- `tools`
- `hide_tools`
- `tools_policies`
- `pass_clients_tools`
- `think`
- `max_tokens`
- `shift`
- `retry_policy`

### Macro surface

Supported chain macros:

- `{{toolservice:list}}`
- `{{toolservice:tools}}`
- `{{toolservice:tools <tools_name>}}`
- `{{var:<name>}}`
- `{{var:<name>|<fallback>}}`
- `{{var:<name>|var:<fallback-name>}}`
- `{{date}}`
- `{{date:<layout>}}`
- `{{now}}`
- `{{now:<layout>}}`
- `{{chain:id}}`

Step-time macro:

- `{{edge_count:from->to}}`

The engine does not expand `env:VAR` macros. Callers must explicitly populate
`var:*`.

### Manual test implications

- Test JSON chain loading and validation errors.
- Test handler validation rejects misspelled handlers.
- Test transition validation rejects unknown operators and dangling targets.
- Test route labels are normalized enough for intended use.
- Test `edge_traversed_at_least` bounds a tool loop.
- Test `timeout` and global `--timeout`.
- Test `retry_on_failure` for deterministic task failures.
- Test `retry_policy` for classified model failures where practical.
- Test macro expansion in `system_instruction`, `prompt_template`,
  `output_template`, `execute_config.model`, `execute_config.provider`,
  `execute_config.think`, and `execute_config.max_tokens`.
- Test missing `{{var:*}}` errors are understandable.
- Test `shift` behavior on context overflow with a small context window.

### Release risks

- Chain files are the core "engineering artifact"; failures here are high
  severity even when the UI works.
- Tool allowlist semantics must be tested against code, not old docs.

## Feature Area 6: CLI Execution Modes

### One-shot and stateless

- Bare `contenox "prompt"` injects `run` when the first arg is not a reserved
  subcommand.
- `contenox run` executes a chain without chat session history.
- Inputs can come from positional args, `--input`, stdin, or `@file`.
- Supported input types:
  - `string`
  - `chat`
  - `json`
  - `int`
  - `float`
  - `bool`

### Stateful chat

- `contenox chat "prompt"` uses persistent session history.
- `contenox chat -e` opens `$EDITOR` or `$VISUAL`.
- Piped stdin can preload editor/reference content.
- `--chain` overrides the configured default chain.

### Output and diagnostics

- `--raw`: print full output.
- `--steps`: print execution steps after result.
- `--trace`: stream task-step events to stderr.

### Manual test implications

- Test bare prompt injection.
- Test stdin with no args.
- Test `--input`.
- Test `--input @file`.
- Test each input type with a small chain.
- Test editor mode with `$EDITOR`.
- Test output modes separately from model success.
- Test run mode does not persist chat messages.
- Test chat mode persists messages.

## Feature Area 7: Sessions And History

### User-visible commands

- `contenox session new [name]`
- `contenox session list`
- `contenox session list --workspace <id>`
- `contenox session list --namespace <name>`
- `contenox session list --all`
- `contenox session switch <name>`
- `contenox session delete <name>`
- `contenox session show [name|id]`
- `contenox session fork [name]`
- `contenox session fork --summary`
- `contenox session workspaces`

### Related features

- Persistent conversation messages in SQLite.
- Session workspace and namespace scoping.
- History compaction through `chain-compact.json`.
- VS Code sessions view.
- VS Code slash command `/compact [keep]`.

### Manual test implications

- Test session creation with and without explicit names.
- Test active-session marker in list output.
- Test show by active, name, and ID.
- Test delete active and non-active sessions.
- Test fork preserves messages.
- Test fork with summary uses compaction chain.
- Test session scoping across two workspaces.
- Test VS Code sessions view follows CLI/runtime state.

## Feature Area 8: Tool System

### Tool selection semantics

For `execute_config.tools` in the current code:

- omitted, `null`, or `[]`: no registry tools exposed.
- `["*"]`: all registered tools exposed.
- `["name"]`: only exact named tools exposed.
- `["*", "!name"]`: all tools except excluded names.
- exclusion-only lists expose no tools.

Release documentation must be reconciled with this behavior. Older text in
`docs/contenox-cli.md` still describes absent tool fields as exposing all
registered tools.

### Local filesystem tools: `local_fs`

Functions:

- `read_file`
- `read_file_range`
- `write_file`
- `list_dir`
- `grep`
- `find_files`
- `search_repo`
- `sed`
- `count_stats`
- `stat_file`

Safety behavior:

- Paths are restricted to an allowed directory.
- Existing files require read-before-write.
- Full overwrite requires full-file read, not only range read.
- Stale reads are rejected if the file changed after the read.
- Symlink escapes are blocked.
- Read/output size limits are enforced.
- Policy can deny sensitive paths.

Policy keys include:

- `_allowed_dir`
- `_denied_path_substrings`
- `_max_read_bytes`
- `_max_output_bytes`
- `_max_list_depth`
- `_skip_dir_names`
- `_list_extensions`
- `_max_grep_matches`
- `_max_find_results`

### Local shell tools: `local_shell`

Features:

- Disabled unless the CLI/bridge enables local shell tooling.
- Supports direct command execution.
- Supports optional shell execution for pipes, globs, redirects, and env
  expansion.
- Detects platform shell:
  - POSIX `sh` on Unix.
  - PowerShell or `cmd.exe` on Windows.
- Returns structured result: exit code, stdout, stderr, success, error,
  duration, command, shell, OS.

Policy keys include:

- `_allowed_commands`
- `_denied_commands`
- `_allowed_dir`

### Web tools: `webtools`

Functions:

- `web_get`
- `web_head`
- `web_post`
- `web_put`
- `web_patch`
- `web_delete`

Safety behavior:

- Scheme allowlist defaults to `http,https`.
- Optional host allowlist.
- Optional host denylist.
- Size and timeout limits.
- Redirect and retry policy.
- Mutating verbs are HITL-gated by default policy.

### OpenAPI remote tools

Commands:

- `contenox tools add <name> --url ...`
- `contenox tools list`
- `contenox tools show <name>`
- `contenox tools update <name>`
- `contenox tools remove <name>`

Features:

- Fetches OpenAPI v3 spec from `<url>/openapi.json` by default.
- Supports `--spec` as URL or local file.
- Supports static headers.
- Supports hidden injected params.
- Supports HTTP login handshake:
  - login URL/method/body
  - cookie extraction
  - JSONPath token extraction
  - header injection and formatting
- Supports per-tools TLS skip for internal/self-signed services.

### MCP tools

Commands:

- `contenox mcp add <name>`
- `contenox mcp list`
- `contenox mcp show <name>`
- `contenox mcp update <name>`
- `contenox mcp remove <name>`
- `contenox mcp auth <name>`

Transports:

- `stdio`
- `http`
- `sse`

Auth:

- bearer token literal or env var.
- OAuth 2.1 + PKCE.
- dynamic client registration where supported.
- pre-issued OAuth client credentials for vendors without DCR.
- token storage for subsequent connections.

### Utility and less-prominent tools

- `echo`: simple echo for test chains.
- `print`: deterministic message output/system message helper.
- SSH remote command execution exists in `runtime/localtools`. V1 should decide
  whether this is public, experimental, or hidden before documenting it as a
  supported user feature.

### Manual test implications

- Test tool allowlists and exclusions.
- Test local_fs read-before-write and stale-read denial.
- Test local_fs path escape, symlink escape, and sensitive-path denial.
- Test local_shell with allowed, denied, and unknown commands.
- Test local_shell on Windows separately because shell semantics differ.
- Test webtools GET/HEAD allow and POST/PUT/PATCH/DELETE approval.
- Test OpenAPI public no-auth service.
- Test OpenAPI local spec file.
- Test OpenAPI static header and hidden injected params.
- Test OpenAPI auth handshake with a local mock service.
- Test MCP stdio server.
- Test MCP HTTP server.
- Test MCP OAuth renewal/re-auth user experience.

## Feature Area 9: HITL Policies And Approval UX

### Policy files

- `hitl-policy-default.json`
- `hitl-policy-dev.json`
- `hitl-policy-strict.json`
- `hitl-policy-acp.json`
- `hitl-policy-acpx.json`

### Policy actions

- `allow`: run without prompt.
- `approve`: ask user before running.
- `deny`: block and return denial.

### Policy matching

Rules match tools, tool names, and conditions over arguments such as:

- file paths
- command names
- command args
- hosts/URLs

Sensitive locations are denied in strict/editor policies, including common
secret, keychain, browser-profile, cloud-credential, and wallet paths.

### Approval surfaces

- CLI/TTY approval for command-line runs.
- ACP `session/request_permission` for editor clients.
- VS Code approval prompts and diff flows through the extension.

### Manual test implications

- Test each policy file can be selected.
- Test default policy allows read-only filesystem work.
- Test default policy prompts for writes and mutating web calls.
- Test strict policy denies by default.
- Test dev policy allows broadly but still blocks destructive commands.
- Test ACP policy routes approval to the client.
- Test ACPX policy denies untrusted/headless operations.
- Test denial messages are visible to the model and the user.
- Test approving and denying the same proposed action.

## Feature Area 10: ACP And Editor Protocol Runtime

### `contenox acp`

Features:

- Speaks Agent Client Protocol over stdio.
- Designed for Zed, JetBrains, AionUi, and other ACP clients.
- Zed is the primary V1 ACP smoke target because it is the clearest native
  editor-client path for stdio ACP.
- Loads `~/.contenox/default-acp-chain.json` by default.
- `CONTENOX_ACP_CHAIN_PATH` overrides the chain.
- Reads global default model/provider.
- HITL is on by default.
- `--auto` disables HITL prompts for unattended/testing mode.
- `--setup` runs setup and exits.
- `--workspace-id` sets workspace identity for new sessions.

### `contenox acpx`

Features:

- Same ACP server profile for non-owner/headless drivers.
- Loads hardened `hitl-policy-acpx.json`.
- Loads `~/.contenox/headless-acp-chain.json`.
- `CONTENOX_ACPX_CHAIN_PATH` overrides the chain.
- Intended for OpenClaw-style untrusted drivers, not normal editor clients.

### Manual test implications

- Test Zed with a real `agent_servers` entry:
  - command: `contenox`
  - args: `["acp"]`
- Test `initialize`/session startup in Zed.
- Test at least one secondary ACP client if time allows: JetBrains or AionUi.
- Test setup flow through ACP.
- Test normal prompt turn.
- Test file read approval/denial through editor UI.
- Test tool call rendering in the ACP client.
- Test session persistence after reconnect.
- Test `--auto` only in controlled test mode.
- Test `acpx` cannot mutate or shell out under default policy.

## Feature Area 11: VS Code Extension

### Extension identity

- Publisher: `contenox`
- Extension name: `contenox-runtime`
- Extension ID: `contenox.contenox-runtime`
- Display name: `Contenox`
- Extension kind: `workspace`

### Runtime bridge

- Extension starts `contenox vscode-agent --stdio`.
- Bridge uses framed JSON-RPC over stdio.
- Stdout is reserved for protocol.
- Logs and diagnostics go to stderr/output channel.
- Extension can use bundled binary or configured `contenox.binaryPath`.
- `contenox.dataDir` can point at an existing Contenox state directory.

### Commands

- Open chat and proposed agent session.
- Diagnose agent sessions.
- Ask/fix selected code.
- Add selection to chat.
- Fix/explain diagnostics.
- Review workspace changes.
- Draft commit message.
- Refresh/open/delete sessions.
- Show status.
- Restart bridge.
- Run setup.
- Select provider/model/chat model.
- Select autocomplete provider/model.
- Select HITL policy.
- Select thinking level.
- Trigger/test/enable/disable/toggle autocomplete.
- Show output.
- Show/clear telemetry log.
- Test language model provider.
- Show/refresh MCP servers.
- Open tool diff.

### Chat participant

Participant:

- `@contenox`

Native participant commands:

- `/fix`
- `/explain`
- `/doctor`
- `/review`
- `/commit`
- `/compact`
- `/policy`
- `/websearch`

Bridge slash commands:

- `/help`
- `/doctor`
- `/clear`
- `/compact [keep]`
- `/model [model-name]`
- `/provider [provider-name]`
- `/autocomplete-model [model-name]`
- `/autocomplete-provider [provider-name]`
- `/max-tokens [count]`
- `/think [auto|off|minimal|low|medium|high|xhigh]`
- `/policy [policy-name]`
- `/capability set|show|unset <provider> <model> [--think true|false]`
- `/websearch <query>`

### Editor integrations

- Editor context menu actions for selected code.
- Chat code action menu entries.
- Lightbulb diagnostics quick fixes.
- Active editor/file context attachment.
- Git status and diff collection for review/commit workflows.
- Native diff editor for proposed tool diffs.

### Inline autocomplete

Features:

- Uses `chain-fim.json`.
- Enabled by `contenox.autocomplete.enabled`.
- Provider/model can be set through VS Code settings or runtime config.
- Prefix/suffix bounds:
  - `maxPrefixChars`
  - `maxSuffixChars`
  - `maxDocumentChars`
  - `maxTokens`
  - `debounceMs`
- Test command shows raw completion or explicit skip/error reason.

### Language model provider

- Contributes VS Code language model provider vendor `contenox`.
- Provides a test command using VS Code model selection.
- Should not require GitHub Copilot sign-in for Contenox-owned paths.

### MCP provider

- Contributes MCP server definition provider `contenox.mcpServers`.
- Should expose only VS Code-supported MCP server definitions.
- OAuth and unsupported transports/types should remain runtime-only or be
  clearly filtered.

### Workspace trust

- Untrusted workspaces are limited.
- Restricted configurations:
  - `contenox.binaryPath`
  - `contenox.dataDir`
  - `contenox.autocomplete.enabled`
- Runtime actions that read or mutate workspace files should stay disabled
  until the workspace is trusted.

### Local telemetry

- Local diagnostic JSONL events.
- No remote telemetry service.
- Output and protocol logs are local and explicit.

### Manual test implications

- Test activation and bridge start on clean VS Code.
- Test `Contenox: Show Status` before setup.
- Test setup, restart bridge, and status after setup.
- Test provider/model selectors update runtime config.
- Test `@contenox hello`.
- Test every participant command.
- Test selected-code ask/fix/add flows.
- Test diagnostics quick fixes from the lightbulb.
- Test git review and commit prompt creation.
- Test session view refresh/open/delete.
- Test HITL approval and denial.
- Test opening a proposed diff.
- Test autocomplete trigger and debounce behavior.
- Test disabled autocomplete state.
- Test local telemetry log and clear command.
- Test MCP server list/refresh with stdio, HTTP, SSE, OAuth, and unsupported
  combinations.
- Test remote workspace binary path behavior.
- Test restricted workspace behavior.

## Feature Area 12: Observability, Traceability, And Debugging

### CLI features

- `--trace` task-step event stream.
- `--steps` execution summary.
- `--raw` full output.
- `contenox state list`.
- `contenox state show <request-id>`.
- `contenox state show <request-id> --raw`.
- `contenox cache clear`.

### Runtime internals visible to tests

- Captured state units survive process restart.
- Request IDs tie execution state to a run.
- Tool results are capped to preserve context.
- Thinking traces are stored separately from message content and are not sent
  back into history as normal text.

### VS Code features

- Output channel.
- Protocol logging when enabled.
- Local JSONL telemetry.
- Status bar state.

### Manual test implications

- Run a chain with `--trace` and confirm events stream as steps happen.
- Run a chain with `--steps` and confirm final summary.
- Confirm state list/show after a successful and failed run.
- Confirm cache clear changes model-list fetch behavior after backend changes.
- Confirm VS Code telemetry includes bridge spawn, chat, autocomplete, and
  code-action events.

## Feature Area 13: Security And Privacy

### V1 guarantees to preserve

- Local-first runtime state.
- No required daemon.
- No required hosted Contenox service.
- API keys should be provided through env vars, not inline literals.
- Tool policies are explicit in chains.
- Local shell is opt-in.
- Workspace trust limits VS Code behavior.
- HITL approval gates unsafe operations.
- Local telemetry is not sent remotely.

### Manual test implications

- Search packaged artifacts for `.env`, tokens, source maps, and node_modules.
- Confirm API key env var names can be shown but values are not printed.
- Confirm remote tool headers are redacted in display output.
- Confirm local_fs cannot access outside allowed dir.
- Confirm local_shell cannot run denied commands.
- Confirm webtools reject disallowed schemes and hosts.
- Confirm VS Code untrusted workspace blocks runtime actions.
- Confirm Marketplace package contains no deleted Beam/UI packages.

## Feature Area 14: CI And Release Automation

### Current CI surfaces

- `make test-unit`.
- `make test-contenox-help`.
- Go package tests.
- VS Code extension type-check.
- VS Code extension package check.
- Site type-check/build/lint in the enterprise site repo.
- GitHub Release workflow that builds standalone CLI artifacts.
- GitHub Release workflow should upload platform-specific VSIX artifacts for
  manual VS Code/VSCodium installation from a release.
- VS Code Marketplace workflow that verifies metadata, builds platform-specific
  VSIX artifacts, checks contents, and optionally publishes.

### Manual test implications

- CI is not enough for V1 because provider auth, editor UI, platform-specific
  binaries, and HITL UX need real interaction.
- CI artifacts should be inspected before publish.
- GitHub Release assets should be inspected for CLI binaries and
  `contenox-runtime-<target>-<version>.vsix` files.
- Pre-release Marketplace install should be tested before stable tag publish.
- Release testing should include at least:
  - Linux x64.
  - macOS arm64.
  - Windows x64.
  - one remote workspace scenario.

## Feature Area 15: Cross-Platform Support

### Supported OS families

Contenox V1 should be treated as a Linux, macOS, and Windows product. Platform
support is not just packaging; it affects runtime behavior.

Required OS-family coverage:

- Linux: local CLI, shell tools, local filesystem tools, ACP stdio, VS Code
  extension, VSIX install.
- macOS: local CLI, shell tools, local filesystem tools, ACP stdio, VS Code
  extension, VSIX install.
- Windows: local CLI, PowerShell/`cmd.exe` shell tools, local filesystem tools,
  ACP stdio where supported by the client, VS Code extension, VSIX install.

### Release targets to track

Standalone CLI release artifacts:

- `linux-amd64`
- `linux-arm64`
- `darwin-arm64`
- `windows-amd64`

VS Code/VSCodium VSIX release artifacts:

- `linux-x64`
- `linux-arm64`
- `darwin-arm64`
- `darwin-x64`
- `win32-x64`

If the release workflow adds or removes targets, update this section and the
manual test plan in the same change.

### Platform-sensitive behavior

- Binary naming: `contenox` on Unix, `contenox.exe` on Windows.
- Executable bits: required on Linux/macOS packaged binaries.
- Path handling: `/tmp/x`, `~/x`, drive letters, backslashes, spaces in paths,
  and symlink behavior.
- Shell behavior: POSIX `sh` on Unix, PowerShell or `cmd.exe` on Windows.
- Quoting and argument splitting for `local_shell`.
- Process lifecycle and stdio framing for ACP and VS Code bridge.
- File permissions and read-before-write tracking.
- Line endings in generated files and docs.
- Default browser/OAuth callback behavior for MCP OAuth.
- VS Code extension host location for Remote SSH, containers, and Codespaces.

### Manual test implications

Minimum V1 platform test matrix:

| Area | Linux x64 | macOS arm64 | Windows x64 |
|------|-----------|-------------|-------------|
| Install CLI from release | Required | Required | Required |
| `contenox init` + `doctor` | Required | Required | Required |
| One-shot run/chat/session | Required | Required | Required |
| Local model or one hosted provider | Required | Required | Required |
| `local_fs` read/write/read-before-write | Required | Required | Required |
| `local_shell` allowed/denied command | Required | Required | Required |
| MCP stdio server | Required | Required | Required |
| ACP client smoke | Required on one Unix target | Required with Zed | Optional if no stable Windows ACP client |
| VS Code VSIX manual install | Required | Required | Required |
| `@contenox /doctor` in VS Code | Required | Required | Required |
| VS Code autocomplete smoke | Required | Required | Required |

Additional target checks:

- Linux ARM64 and macOS Intel can be package/binary smoke tests until real
  hardware or runners are available.
- Windows requires a real editor smoke, not just a cross-built binary.
- Remote SSH/container behavior should be tested at least once with a Linux
  remote workspace from a non-Linux client.

### Release risks

- Cross-compilation can produce a binary that packages cleanly but fails at
  runtime because of CGo, libc, or C++ dependency differences.
- Windows command execution bugs often hide behind tests written on Unix.
- VSIX target selection is easy for users to get wrong unless release docs and
  asset names are clear.
- Marketplace and GitHub Release artifacts can drift unless both workflows use
  the same target matrix and package cleanliness checks.

## Remaining Documentation And Release Gaps

- Tool allowlist docs have been reconciled on the public site. Keep a release
  test for the runtime behavior: omitted/null/empty tools exposes no registry
  tools, and `["*"]` exposes all registered tools.
- Decide whether SSH localtools are public V1, experimental, or hidden.
- Decide whether `acpx` is documented publicly or kept as integration-only.
- Decide whether VS Code language model provider and MCP provider are stable V1
  surfaces or preview surfaces.
- Document auth renewal UX for MCP OAuth and provider API-key expiry.
- Document remote VS Code workspace behavior clearly.
- Public site/docs were scanned and cleaned for `contenox serve`, Beam UI,
  stale MCP test server, stale repo links, and proxy-current-feature language.
  Re-run the stale-string scan before release.
- Create a short Marketplace release checklist from this feature map.

## Manual Test Plan Skeleton

This skeleton is not the finished test plan. It is the structure the test plan
should use.

1. Install and package integrity.
2. Fresh setup and doctor.
3. Global config and project-local chain state.
4. Backend/provider matrix.
5. Local model registry and download.
6. Chain DSL validation and execution.
7. Stateless run modes.
8. Stateful chat and sessions.
9. HITL policy behavior.
10. Local filesystem tools.
11. Local shell tools.
12. Web tools.
13. OpenAPI remote tools.
14. MCP transports and auth.
15. Observability and state inspection.
16. Cross-platform CLI smoke: Linux, macOS, Windows.
17. ACP clients, with Zed as the primary required smoke target.
18. VS Code bridge health/config.
19. VS Code chat participant.
20. VS Code editor actions and diagnostics.
21. VS Code autocomplete.
22. VS Code sessions view.
23. VS Code LM provider.
24. VS Code MCP provider.
25. Remote workspace behavior.
26. GitHub Release VSIX manual install.
27. Marketplace pre-release install.

## V1 Release Gate Recommendation

For V1, treat these as required pass/fail gates:

- Clean install and setup on Linux x64, macOS arm64, and Windows x64.
- `contenox init`, one-shot run, chat, session persistence, and doctor pass on
  Linux, macOS, and Windows.
- At least one local model path or one hosted provider path passes end to end.
- Tool safety gates pass for local_fs, local_shell, and webtools, including a
  real Windows local_shell smoke.
- One OpenAPI tools registration and one MCP stdio registration pass.
- ACP works in Zed with a real stdio `agent_servers` configuration.
- VS Code extension works from Marketplace/pre-release artifact for:
  - setup
  - chat
  - selection explain
  - diagnostics action
  - autocomplete
  - HITL approval
  - session view
- GitHub Release VSIX assets install manually in VS Code or VSCodium on Linux,
  macOS, and Windows.
- Platform package contents are clean and match the target OS/architecture.
- Public docs and Marketplace README describe the same product users actually
  receive.

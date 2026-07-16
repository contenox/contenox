# Session Workspaces: shared file context for agent and user

Status: draft blueprint, 2026-07-16. Grounded in a full code audit (file:line
references below are current as of that date). Not started.

## Definition

One root directory per chat session — the **workspace root** — is the shared
filesystem context for both parties:

- **The agent** operates in it: `local_fs` (read/write/grep/find) resolves
  against it and is sandboxed to it.
- **The user** sees it: an explorable file tree in the chat UI, file peek,
  and **@-mentions** — typing `@` in the composer autocompletes files from
  the workspace and attaches them to the prompt as ACP resource blocks.

Same root, two views. The root is **runtime-defined per session** (picked at
session start alongside model/think), with a serve-level default from an arg
(`contenox serve [dir]`) or env, and an **allowlist** guarding what a browser
client may choose.

## What the audit found (the punchlines)

1. **Per-session cwd is already protocol-plumbed but DECORATIVE in the beam
   path.** ACP `session/new` requires an absolute cwd; acpsvc persists it
   (`session.go:904-920`) — but only for `session/list` display. Beam always
   sends `cwd: '/'` (`acpWorkspaceController.ts:53-54`, never overridden).
   In `contenox serve`, `local_fs` is fixed-rooted at the serve process's
   directory (`serve_cmd.go:153`, no cwdResolver) — the session cwd is
   ignored for file access.
2. **The stdio path already proves the target pattern.** `contenox acp`
   wires `NewLocalFSToolsWith("", db, acpFileIO, "local_fs", NewACPCwdResolver(...))`
   (`acp_cmd.go:238-244`): empty fixed root, per-session cwd resolution, and
   file IO proxied to the client when it advertises `fs/read_text_file`
   (with server-side `os` fallback — `runtime/acpsvc/fileio.go`). The serve
   path needs the same wiring, minus the client-FS proxy (browsers have no
   local FS to offer).
3. **The browse API already exists and is unused.** `GET/PUT/POST /files`
   (list/stat/content/download/write/move) is registered in serve behind the
   `/api` token, rooted at the workspace root
   (`runtime/internal/localfileapi/routes.go`, `serverapi/server.go:128-134`).
   Beam never calls it. This is the file-explorer data source, already built.
4. **The @-mention carrier exists.** libacp has `resource_link` and embedded
   `resource` content blocks (`libacp/content.go`), initialize advertises
   `embeddedContext: true`, and `flattenPromptBlocks` (`acpsvc/content.go:9-58`)
   already consumes them: inline resource text is appended to the prompt;
   a `resource_link` becomes a `name: uri` line the agent can follow with
   `local_fs`. No new protocol needed.
5. **UI kickstart components:** `FileTree` (packages/ui, built + storybook,
   currently dangling — caller supplies the node tree; `directoryClickMode:
   'navigate'` was designed for cwd changes), `InlineAttachmentRenderer`'s
   `file_view` (already wired for agent-produced file views), and
   `SlashCommandMenu` (the in-repo pattern for composer autocomplete — same
   mechanic as @, different trigger).

## Security model

A browser choosing arbitrary host paths is a capability grant; treat it so:

- Serve holds an **allowlist of workspace roots**: the serve arg/env default,
  plus roots explicitly registered (config or API). "Runtime-defined" =
  choosing within the allowlist; free-form absolute paths only when the
  operator explicitly opens it (flag), mirroring `--local-exec-allowed-dir`.
- `session/new` validates the requested cwd against the allowlist and
  refuses otherwise (typed error, actionable message).
- `local_fs` containment (`checkPath`/symlink-escape guards, `fs.go:195-287`)
  already enforces the sandbox once rooted; the `/files` API has its own
  root containment (`localfileservice.go:285` `isWithinRoot`) — but it is
  rooted at ONE fixed root today and must become per-root (resolve against
  the session's workspace root, still allowlist-bound).

## Decision points

- **@-mention semantics: DECIDED (maintainer, 2026-07-16): reference only.**
  The composer emits `resource_link` blocks, never embedded resources or
  attachments — the agent must use its tools to read anything. This is a
  principle, not a default: prompts stay lean, reads are always fresh, every
  file access goes through the same sandboxed, policy-visible tool path
  (HITL can see and gate it), and there is exactly one way content enters
  context. The protocol keeps *accepting* embedded resources from external
  ACP clients for conformance (`flattenPromptBlocks` behavior unchanged),
  but beam never emits them and no embed/attach affordance is built.
- **Root picker UX:** pre-session config-options row (same surface as
  model/think, shipped 2026-07-16) listing allowlisted roots; free-text only
  when the operator enabled it.
- **File explorer placement: DECIDED (maintainer, 2026-07-16): an IDE-style
  panel with a toggle.** A dedicated workspace panel on the chat page —
  toggled like an IDE's explorer (own toggle affordance, persists collapsed/
  expanded state), not a transient popover and not crammed into the session
  sidebar. Fed by `/files`; it doubles as the @-mention picker's data
  source, and `FileTree`'s `directoryClickMode: 'navigate'` maps onto it.

## Walking skeleton

1. **Wire cwd → tools in serve** (the one real plumbing gap): construct
   serve's `local_fs` like the stdio path — empty fixed root + cwdResolver
   reading the session's cwd (`NewACPCwdResolver` already exists), with the
   serve-level default root used when a session has none. Allowlist check in
   `session/new`. This alone makes the agent workspace-scoped.
2. **Beam sends a real cwd**: workspace-root picker in the pre-session
   controls (options served like config_options, from the allowlist);
   `newSession(cwd)` already accepts it end to end.
3. **File explorer panel**: `FileTree` fed by `GET /files` (extended to
   resolve against the session's root), file peek via `GET /files/content`
   rendered with the existing `file_view` presentation.
4. **@-mentions**: composer `@` autocomplete over the same `/files` listing;
   selection attaches a `resource_link` block — reference only, no embed
   variant. Server-side consumption already works.
5. Later: multi-root sessions (ACP `AdditionalDirectories`, deliberately
   unset today — `initialize.go:85-96`), client-FS proxy for editor-hosted
   beam variants, write-path UX (agent edits surfacing as diffs vs the tree).

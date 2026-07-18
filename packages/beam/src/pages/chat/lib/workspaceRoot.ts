/**
 * Resolves the composer's workspace root — the directory the `/files` REST is
 * rooted at, which backs BOTH the workspace file panel and the `@`-mention
 * candidate list (see `useWorkspaceFiles`). This is contenox runtime-side data,
 * INDEPENDENT of which agent drives the session (the native chain or a bound
 * external agent): the files are the runtime's own, and an `@`-mention is sent
 * as a reference-only `resource_link` the agent reads under the same cwd (see
 * `promptBlocksFromDraft` / acpsvc `externalDriver.Prompt`). So a session backed
 * by an external agent resolves its root exactly like a native one — the earlier
 * "drop the file panel for external sessions" special-case was the bug this
 * removes.
 *
 *  - A LIVE session uses its persisted cwd (`activeSessionCwd`).
 *  - The EMPTY chat uses the user's staged workspace-root pick, else the
 *    workspace default root. An external empty chat exposes no root PICKER (an
 *    external session advertises no native config options), so `stagedRoot` is
 *    always absent for it and it falls back to the default root — which is the
 *    very cwd `session/new` creates it under (acpsvc resolves the "/" sentinel to
 *    that default root).
 *
 * `null` means "no root yet" (no default configured, or the session cwd is
 * unknown): the caller hides the file panel and the mention menu stays empty.
 */
export function resolveWorkspaceRoot(args: {
  /** Whether this is the empty/new-chat surface (no session created yet). */
  onEmptyChat: boolean;
  /** The user's staged workspace-root pick on the empty chat, if any (empty string counts as "none"). */
  stagedRoot: string | undefined;
  /** The workspace-level default root advertised by the runtime, if configured. */
  defaultRoot: string | undefined;
  /** A live session's persisted cwd (`null` when unknown). */
  activeSessionCwd: string | null;
}): string | null {
  const { onEmptyChat, stagedRoot, defaultRoot, activeSessionCwd } = args;
  if (!onEmptyChat) return activeSessionCwd;
  if (stagedRoot) return stagedRoot;
  return defaultRoot ?? null;
}

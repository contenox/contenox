import type { SessionInfo } from '../../../lib/acp';

/**
 * `session/list` currently reports `Title` as a copy of the session id itself
 * (see `acpsvc/session.go`'s `ListSessions` — ACP sessions have no distinct
 * display name yet), so a title that merely echoes the id carries no
 * information; treat it as absent and let the caller render a short-id fallback.
 * Forward-compatible: a future backend that sends a real friendly title just
 * works. Returns `null` when there is no meaningful title (the caller renders
 * the i18n'd short-id fallback, kept out of here so this stays `t()`-free).
 */
export function meaningfulTitle(session: SessionInfo): string | null {
  const title = session.title?.trim();
  if (title && title !== session.sessionId) return title;
  return null;
}

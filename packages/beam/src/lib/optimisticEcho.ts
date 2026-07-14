/**
 * Matches an optimistic (client-side) user message against its persisted echo.
 *
 * The server may rewrite the stored user message by *prepending* a context
 * block when artifacts are attached (agentservice.ComposeUserInput keeps the
 * original input as a strict suffix). Plain sends persist byte-identical.
 */
export function isOptimisticEcho(persistedContent: string, optimisticContent: string): boolean {
  if (persistedContent === optimisticContent) return true;
  if (!optimisticContent) return false;
  return persistedContent.endsWith(optimisticContent);
}

/** Tolerance for client/server clock skew when matching by content + sentAt. */
export const OPTIMISTIC_ECHO_WINDOW_MS = 5 * 60_000;

/**
 * Matches a persisted user message against the optimistic turn it may echo.
 *
 * Provenance is authoritative: a persisted message stamped with a `requestId`
 * matches iff it equals the optimistic turn's. Only unstamped (legacy)
 * messages fall back to content matching, and then only within a sentAt
 * window — otherwise re-running an identical command would match its own
 * earlier persisted copy and suppress the fresh optimistic turn.
 */
export function matchesOptimisticEcho(
  persisted: { content: string; sentAt: string; requestId?: string },
  optimistic: { content: string; sentAt: string; requestId: string },
): boolean {
  if (persisted.requestId) return persisted.requestId === optimistic.requestId;
  if (!isOptimisticEcho(persisted.content, optimistic.content)) return false;
  return (
    Math.abs(Date.parse(persisted.sentAt) - Date.parse(optimistic.sentAt)) <
    OPTIMISTIC_ECHO_WINDOW_MS
  );
}

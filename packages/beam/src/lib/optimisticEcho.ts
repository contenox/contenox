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

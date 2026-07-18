import { externalAgentFromMeta } from '../../../lib/acp';

/**
 * Resolves the agent name the chat surface should attribute the CURRENT view to,
 * in priority order:
 *
 *  1. the active session's own agent, read back from its `_meta` echo (an
 *     external session carries `{ "contenox.agent": name }` — see AGENT_META_KEY);
 *  2. on the empty/new-chat surface (no session yet), the STAGED agent the
 *     sessions rail seeded — this is what makes an agent picked from the rail
 *     visible on the empty surface, and it updates reactively as the staged
 *     value changes (the surface re-renders on the `StagedAgent` context);
 *  3. otherwise the workspace-level ("contenox") agent.
 *
 * Pure (no React) so the "empty surface shows the staged pick / a live session
 * shows its own agent / native clears to the workspace agent" contract is
 * unit-testable without a DOM — see `activeAgent.test.ts`.
 */
export function resolveActiveAgentName(input: {
  /** The active session's `_meta` (the `/chat/:id` session), or null/undefined on the empty surface. */
  sessionMeta: Record<string, unknown> | null | undefined;
  /** True when the empty/new-chat surface is showing (no `:sessionId` in the route). */
  isEmptySurface: boolean;
  /** The staged agent for the next new chat (`null` = native contenox). */
  stagedAgent: string | null;
  /** The workspace-level agent name, if any. */
  workspaceAgentName: string | null | undefined;
}): string | null {
  return (
    externalAgentFromMeta(input.sessionMeta) ??
    (input.isEmptySurface ? input.stagedAgent : null) ??
    input.workspaceAgentName ??
    null
  );
}

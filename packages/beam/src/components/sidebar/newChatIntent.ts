/**
 * THE single "start a fresh chat" action for the sessions rail.
 *
 * Every "start a fresh chat" affordance funnels through here so they can never
 * drift:
 *  - the plain "New session" button  →  `startNewChat(null, …)`  (native contenox)
 *  - the agent picker's `onSelect`    →  `startNewChat(name, …)`  (a registered agent,
 *                                        or `null` for its native "Contenox (default)" entry)
 *  - a Projects-page launcher row     →  `startNewChat(null, …, { cwd })`  (open a
 *                                        session already scoped to that project)
 *
 * The action is deliberately order-fixed and route-independent:
 *  1. STAGE the picks first. `null` clears any previously staged agent, so the
 *     next new chat is native; a name stages that external agent. The empty
 *     `/chat` surface reads this staged value from the shared `StagedAgent`
 *     context and re-renders reactively — which is what makes the pick visible
 *     even when we are ALREADY on the empty surface and the navigate below is a
 *     no-op (React Router does not re-run location effects for the current URL).
 *     The launch cwd is staged the same way through the parallel `StagedRoot`
 *     context: `opts.cwd` stages that project as the next chat's workspace. Its
 *     ABSENCE clears the context stage — defensive hygiene so a launcher stage
 *     that was never consumed (its chat never mounted) can't silently apply to a
 *     later fresh chat. It does NOT reset an already-open empty chat's chosen
 *     workspace: that pick is sticky within the tab (as in a real editor, a new
 *     chat stays in your current project) until another launch or a manual pick.
 *  2. DRIVE the workspace tab-model to the empty/new-chat surface via
 *     `focusEmptyTab` — the SOURCE OF TRUTH for this transition. A bare
 *     `navigate('/chat')` on its own is NOT enough from a focused session tab:
 *     the tab↔route sync (see `WorkspaceTabs`) still had the session as the
 *     active tab and reverted the URL straight back to `/chat/:id`. Re-pointing
 *     focus first means the route below merely follows a decision the tab-model
 *     has already made. `focusEmptyTab` is additive — it never closes an open
 *     session tab.
 *  3. ROUTE to the bare `/chat` surface so the URL reflects the empty surface
 *     (and, from a non-chat page, actually mounts the chat page). Already there,
 *     it is a harmless no-op and steps 1–2 carry the update.
 *  4. COLLAPSE the mobile rail overlay.
 *
 * Extracted as a pure function (no React, no router) so the staging + focus
 * contract is unit-testable without a DOM — see `newChatIntent.test.ts`.
 */
export interface NewChatDeps {
  /** Stage the next chat's agent (`null` = native contenox / clear any prior stage). */
  setStagedAgent: (name: string | null) => void;
  /**
   * Stage the next chat's workspace cwd (a launcher's project), or `null` to
   * clear the context stage. Optional: a caller that never launches into a
   * specific project (an isolated test) may omit it; when present it is ALWAYS
   * called, so a non-launcher path clears an un-consumed launcher stage rather
   * than letting it leak into a later fresh chat.
   */
  setStagedRoot?: (cwd: string | null) => void;
  /**
   * Re-point the workspace tab-model's focus to the empty/new-chat surface
   * (the controller's `focusEmptyTab` from `useAcpWorkspace` — the SAME action
   * `useWorkspaceTabs` drives when its active tab becomes the empty surface).
   * Additive: it does NOT close any open session tab, it just makes the empty
   * surface the focused one so the route sync agrees with the intent.
   */
  focusEmptyTab: () => void;
  /** Route to a path (react-router's `navigate`). */
  navigate: (to: string) => void;
  /** Collapse the rail (the mobile sidebar overlay); a no-op on desktop. */
  closeSidebar: () => void;
}

/** Options for a fresh chat beyond its agent — today just the launch cwd. */
export interface NewChatOptions {
  /** Open the chat already scoped to this project cwd (a Projects-page launcher). */
  cwd?: string | null;
}

export function startNewChat(
  agent: string | null,
  deps: NewChatDeps,
  opts?: NewChatOptions,
): void {
  deps.setStagedAgent(agent);
  // Reconcile the shared project stage: a launcher stages its cwd; every other
  // path clears the context so an un-consumed launcher stage can't leak into a
  // later fresh chat. (An already-open empty chat keeps its own sticky pick.)
  deps.setStagedRoot?.(opts?.cwd ?? null);
  deps.focusEmptyTab();
  deps.navigate('/chat');
  deps.closeSidebar();
}

/**
 * THE single "start a fresh chat" action for the sessions rail.
 *
 * Both rail affordances funnel through here so they can never drift:
 *  - the plain "New session" button  →  `startNewChat(null, …)`  (native contenox)
 *  - the agent picker's `onSelect`    →  `startNewChat(name, …)`  (a registered agent,
 *                                        or `null` for its native "Contenox (default)" entry)
 *
 * The action is deliberately order-fixed and route-independent:
 *  1. STAGE the pick first. `null` clears any previously staged agent, so the
 *     next new chat is native; a name stages that external agent. The empty
 *     `/chat` surface reads this staged value from the shared `StagedAgent`
 *     context and re-renders reactively — which is what makes the pick visible
 *     even when we are ALREADY on the empty surface and the navigate below is a
 *     no-op (React Router does not re-run location effects for the current URL).
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

export function startNewChat(agent: string | null, deps: NewChatDeps): void {
  deps.setStagedAgent(agent);
  deps.focusEmptyTab();
  deps.navigate('/chat');
  deps.closeSidebar();
}

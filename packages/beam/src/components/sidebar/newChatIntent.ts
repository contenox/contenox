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
 *  2. ROUTE to the bare `/chat` surface. From a session tab or a non-chat page
 *     this focuses the empty/new-chat surface (the tab-model's param sync opens
 *     it); when already there it is a harmless no-op and step 1 carries the update.
 *  3. COLLAPSE the mobile rail overlay.
 *
 * Extracted as a pure function (no React, no router) so the staging contract is
 * unit-testable without a DOM — see `newChatIntent.test.ts`.
 */
export interface NewChatDeps {
  /** Stage the next chat's agent (`null` = native contenox / clear any prior stage). */
  setStagedAgent: (name: string | null) => void;
  /** Route to a path (react-router's `navigate`). */
  navigate: (to: string) => void;
  /** Collapse the rail (the mobile sidebar overlay); a no-op on desktop. */
  closeSidebar: () => void;
}

export function startNewChat(agent: string | null, deps: NewChatDeps): void {
  deps.setStagedAgent(agent);
  deps.navigate('/chat');
  deps.closeSidebar();
}

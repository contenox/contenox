import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

/**
 * Workspace-scoped staging for "the next chat should ADOPT this running unit"
 * (see `adoptMeta.ts` for the wire contract). It is the sibling of
 * `stagedAgent.tsx`: the fleet board / mission detail seed the intent from a
 * DIFFERENT surface than the one that consumes it (the chat workspace's
 * `WorkspaceTabs`, which eagerly adopts once the connection is ready), so it
 * lives in a small shared context rather than being threaded through the route.
 *
 * Unlike the staged agent — consumed lazily on the first prompt — an adopt
 * intent is consumed EAGERLY: the operator wants to watch the running session's
 * replay immediately, without typing anything. `WorkspaceTabs` fires
 * `adoptSession` the moment it is set and the workspace is `ready`, then clears
 * it (one-shot).
 *
 * `cwd` is optional — the downstream session's working directory was fixed at
 * dispatch and cannot be re-rooted (see acpsvc/adopt.go "Honest limitations"),
 * so this only governs the upstream session's own workspace bookkeeping.
 */
export type AdoptIntent = {
  /** The running instance to adopt (agentinstance instance id). */
  instanceId: string;
  /** The downstream ACP session id on that instance to bind to. */
  sessionId: string;
  /** Optional upstream cwd; defaults to the workspace root. */
  cwd?: string;
};

export interface AdoptIntentContextValue {
  /** The staged adopt intent, or `null` when nothing is pending. */
  adoptIntent: AdoptIntent | null;
  /** Stage an adopt intent, or `null` to clear it (consumed one-shot). */
  setAdoptIntent: (intent: AdoptIntent | null) => void;
}

const AdoptIntentContext = createContext<AdoptIntentContextValue | null>(null);

export function AdoptIntentProvider({ children }: { children: ReactNode }) {
  const [adoptIntent, setAdoptIntent] = useState<AdoptIntent | null>(null);
  const value = useMemo<AdoptIntentContextValue>(
    () => ({ adoptIntent, setAdoptIntent }),
    [adoptIntent],
  );
  return <AdoptIntentContext.Provider value={value}>{children}</AdoptIntentContext.Provider>;
}

/**
 * Reads the adopt-intent context. Returns a safe no-op default when rendered
 * outside a provider (e.g. the fleet/mission page tests, which render to static
 * markup without the app shell — see FleetPage.test.tsx), so an entry point can
 * call it unconditionally without pulling in the ACP workspace provider (whose
 * own hook throws off-provider).
 */
export function useAdoptIntent(): AdoptIntentContextValue {
  return useContext(AdoptIntentContext) ?? NOOP_ADOPT_INTENT;
}

const NOOP_ADOPT_INTENT: AdoptIntentContextValue = {
  adoptIntent: null,
  setAdoptIntent: () => {},
};

/**
 * THE single "open this running session in chat" action, shared by every entry
 * point (the fleet board's per-session sub-row, the mission detail) so they can
 * never drift. Deliberately order-fixed and route-independent:
 *
 *  1. STAGE the intent — the chat workspace reads it reactively and adopts.
 *  2. ROUTE to the bare `/chat` surface, where `WorkspaceTabs` (gated on a ready
 *     connection) fires the eager `adoptSession` and promotes the result to a
 *     tab (its URL then follows to `/chat/:newUpstreamId`).
 *
 * It does NOT drive `focusEmptyTab` the way `startNewChat` does: entry points
 * live on non-chat pages (/fleet, /missions/:id) where the tab↔route sync is not
 * mounted, so there is no active-tab revert to defend against — and depending on
 * the ACP workspace controller here would make those pages (and their DOM-less
 * tests) require the provider that `startNewChat`'s callers already have.
 *
 * Pure (no React, no router) so the staging contract is unit-testable — see
 * `adoptIntent.test.ts`.
 */
export interface StartAdoptDeps {
  /** Stage the adopt intent (the context's `setAdoptIntent`). */
  setAdoptIntent: (intent: AdoptIntent) => void;
  /** Route to a path (react-router's `navigate`). */
  navigate: (to: string) => void;
}

export function startAdoptSession(intent: AdoptIntent, deps: StartAdoptDeps): void {
  deps.setAdoptIntent(intent);
  deps.navigate('/chat');
}

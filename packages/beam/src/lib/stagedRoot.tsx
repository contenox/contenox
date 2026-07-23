import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

/**
 * Workspace-scoped staging for "which project (cwd) the next new chat should
 * open in" — the sibling of {@link ./stagedAgent.tsx stagedAgent}.
 *
 * A new ACP session is minted lazily on the first prompt, so — exactly like the
 * agent choice — a workspace-root pick made OUTSIDE the empty chat surface has
 * to be staged before any session exists and applied at `session/new` time. The
 * empty chat already stages the root LOCALLY when the pick and the consumer are
 * the same component (its own Workspace picker). This context covers the other
 * case: a DIFFERENT component seeds the pick — today the Projects page, whose
 * rows are launchers ("open a session in this project"). The empty chat consumes
 * it one-shot into its local staged config, so the Workspace picker then shows
 * and can still override it.
 *
 * `null` means "no launch root staged" — the empty chat falls back to its normal
 * default root. A non-null value is an absolute path already offered by the
 * workspace-root allowlist (the server still does the real bounds check at
 * `session/new`; this only pre-selects, it never widens what is reachable).
 */
export interface StagedRootContextValue {
  /** The staged launch cwd, or `null` when nothing is staged. */
  stagedRoot: string | null;
  /** Stage a launch cwd, or `null` to clear a stale stage. */
  setStagedRoot: (cwd: string | null) => void;
}

const StagedRootContext = createContext<StagedRootContextValue | null>(null);

export function StagedRootProvider({ children }: { children: ReactNode }) {
  const [stagedRoot, setStagedRoot] = useState<string | null>(null);
  const value = useMemo<StagedRootContextValue>(
    () => ({ stagedRoot, setStagedRoot }),
    [stagedRoot],
  );
  return <StagedRootContext.Provider value={value}>{children}</StagedRootContext.Provider>;
}

/**
 * Reads the staged-root context. Returns a safe no-op default when rendered
 * outside a `StagedRootProvider` (e.g. an isolated component test that doesn't
 * mount the app shell), so consumers never have to guard for it — mirroring
 * {@link ./stagedAgent.tsx useStagedAgent}.
 */
export function useStagedRoot(): StagedRootContextValue {
  return useContext(StagedRootContext) ?? NOOP_STAGED_ROOT;
}

const NOOP_STAGED_ROOT: StagedRootContextValue = {
  stagedRoot: null,
  setStagedRoot: () => {},
};

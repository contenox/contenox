import { createContext, useContext, useMemo, useState, type ReactNode } from 'react';

/**
 * Workspace-scoped staging for "which agent the next new chat should talk to".
 *
 * A new ACP session is minted lazily on the first prompt (see
 * `ChatSessionTab`/`acpWorkspaceController.newSession`), so the agent choice —
 * like the `workspace-root` config pick — has to be *staged* before any session
 * exists and applied at `session/new` time. Unlike `workspace-root` (a native
 * `SessionConfigOption` staged locally in `ChatSessionTab`), the agent choice is
 * seeded from a DIFFERENT component (the sessions sidebar's "new chat with an
 * agent" menu) than the one that consumes it (the empty chat surface), so it
 * lives in this small shared context rather than local component state.
 *
 * `null` means the native "contenox" chain (no external agent). A non-null value
 * is a registered external agent's name; the empty chat passes it to
 * `newSession(cwd, agentName)`, which puts `{ "contenox.agent": name }` in the
 * `session/new` `_meta` (see `AGENT_META_KEY`). Consumed one-shot: the empty
 * chat resets it to `null` right after spawning the session.
 */
export interface StagedAgentContextValue {
  /** The staged external agent's name, or `null` for the native chain. */
  stagedAgent: string | null;
  /** Stage an external agent by name, or `null` to reset to the native chain. */
  setStagedAgent: (name: string | null) => void;
}

const StagedAgentContext = createContext<StagedAgentContextValue | null>(null);

export function StagedAgentProvider({ children }: { children: ReactNode }) {
  const [stagedAgent, setStagedAgent] = useState<string | null>(null);
  const value = useMemo<StagedAgentContextValue>(
    () => ({ stagedAgent, setStagedAgent }),
    [stagedAgent],
  );
  return <StagedAgentContext.Provider value={value}>{children}</StagedAgentContext.Provider>;
}

/**
 * Reads the staged-agent context. Returns a safe no-op default when rendered
 * outside a `StagedAgentProvider` (e.g. an isolated component test that doesn't
 * mount the app shell), so consumers never have to guard for it.
 */
export function useStagedAgent(): StagedAgentContextValue {
  return useContext(StagedAgentContext) ?? NOOP_STAGED_AGENT;
}

const NOOP_STAGED_AGENT: StagedAgentContextValue = {
  stagedAgent: null,
  setStagedAgent: () => {},
};

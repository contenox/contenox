import { useCallback, useEffect } from 'react';
import type { SessionConfigOptionValue } from '../lib/acp';
import type { AdoptRef } from '../lib/adoptMeta';
import type { WorkspaceFileRef } from '../pages/chat/lib/mentions';
import { useAcpWorkspaceContext } from '../lib/acp/AcpWorkspaceProvider';
import type { AcpSessionState } from './acpSessionState';
import { selectOpenSessionIds, type AcpSessionsState, type AcpWorkspaceState } from './acpWorkspaceState';

export interface UseAcpWorkspaceResult {
  /** Connection status + `session/list` roster — see `acpWorkspaceState.ts`. */
  workspace: AcpWorkspaceState;
  /** The FOCUSED session's live timeline — see `acpSessionState.ts`. Follows `workspace.activeSessionId`. */
  session: AcpSessionState;
  /** The full multiplexed sessions store: a live slice per open session + the focused pointer (workspace-tabs Slice 1). Slice 2's tab UI reads open sessions/slices from here. */
  sessions: AcpSessionsState;
  /** Ids of all currently-open (subscribed, live) sessions — several can be open at once (Slice 2). */
  openSessionIds: string[];
  /** Pages `session/list` to completion and replaces the roster. */
  refreshSessions: () => void;
  /** Lazy-creation primitive (D5): creates a session, subscribes to it, and makes it active. Call this on first prompt submit, not on mount — see acpWorkspaceController.ts. `cwd` sets the session's workspace root (the user's pre-session pick). `agentName` binds the session to a registered external agent via the `session/new` `_meta` extension (null/omitted = native chain). */
  newSession: (cwd?: string, agentName?: string | null) => Promise<string>;
  /** Adopts an already-running instance+session into a new upstream chat session (additive — does not close open sessions). See `acpWorkspaceController.ts`'s `adoptSession()`. */
  adoptSession: (ref: AdoptRef, cwd?: string) => Promise<string>;
  /** Single-view switch: opens `id` and closes whichever session was focused. */
  openSession: (id: string) => void;
  /** Multi-session (Slice 2): opens/focuses `id` as a tab WITHOUT closing others — several sessions stay open and live. See `acpWorkspaceController.ts`'s `openSessionTab()`. */
  openSessionTab: (id: string) => Promise<void>;
  /** Multi-session (Slice 2): closes `id`'s tab (`session/close`, not delete) and drops its slice; other tabs stay live. See `closeSessionTab()`. */
  closeSessionTab: (id: string) => void;
  /** Multi-session (Slice 2): re-points the rendered session to an already-open `id`, no wire traffic. See `focusSession()`. */
  focusSession: (id: string) => void;
  /** Multi-session (Slice 2): re-points focus to the empty/new-chat surface WITHOUT closing any open session. See `focusEmptyTab()`. */
  focusEmptyTab: () => void;
  deleteSession: (id: string) => void;
  /** Client-side reset of "which session is open" — no server-side deletion. Call before navigating to bare `/chat` from any "new session" affordance so the next lazy `newSession()` call mints a genuinely new session. See acpWorkspaceController.ts's doc comment. */
  clearActiveSession: () => void;
  /** No-ops with no active session — call `newSession()` first if `workspace.activeSessionId` is null. `mentions` become `resource_link` blocks (reference only). */
  sendPrompt: (text: string, mentions?: WorkspaceFileRef[]) => void;
  /** `!` passthrough: runs one user line in the session's shell without an LLM turn. No-op with no active session. See `acpWorkspaceController.ts`'s `runTerminal()`. */
  runTerminal: (command: string) => Promise<void>;
  respondPermission: (optionId: string) => void;
  cancel: () => void;
  setConfigOption: (configId: string, value: string | boolean) => void;
  /** Flushes the empty-chat's staged config choices to the just-created session, awaiting each, so they win over server defaults for the first turn. See `acpWorkspaceController.ts`'s `applyConfigOptions()`. */
  applyConfigOptions: (options: Array<{ configId: string; value: SessionConfigOptionValue }>) => Promise<void>;
  /** Manual reconnect — cancels any pending automatic backoff and retries immediately. See `acpWorkspaceController.ts`'s `reconnect()` doc comment. */
  reconnect: () => void;
}

/**
 * The ergonomic entry point for consuming the app-wide ACP workspace: reads
 * `AcpWorkspaceProvider`'s context and connects LAZILY — the connection is
 * only opened once the first component actually calls this hook, not when
 * the provider itself mounts (see `AcpWorkspaceProvider.tsx`'s doc comment).
 * `controller.connect()` is idempotent, so multiple components calling this
 * hook simultaneously still share one connection.
 *
 * Must be rendered under `<AcpWorkspaceProvider>` — which in practice means
 * under an authenticated route, since the provider is only mounted once
 * `ProtectedRoute` has confirmed a session/token exists; see
 * `components/ProtectedRoute.tsx`.
 */
export function useAcpWorkspace(): UseAcpWorkspaceResult {
  const { workspace, session, sessions, controller } = useAcpWorkspaceContext();

  useEffect(() => {
    void controller.connect();
  }, [controller]);

  const openSessionIds = selectOpenSessionIds(sessions);

  const refreshSessions = useCallback(() => {
    void controller.refreshSessions();
  }, [controller]);

  const newSession = useCallback(
    (cwd?: string, agentName?: string | null) => controller.newSession(cwd, agentName),
    [controller],
  );

  const adoptSession = useCallback(
    (ref: AdoptRef, cwd?: string) => controller.adoptSession(ref, cwd),
    [controller],
  );

  const openSession = useCallback(
    (id: string) => {
      void controller.openSession(id);
    },
    [controller],
  );

  const openSessionTab = useCallback((id: string) => controller.openSessionTab(id), [controller]);

  const closeSessionTab = useCallback(
    (id: string) => {
      controller.closeSessionTab(id);
    },
    [controller],
  );

  const focusSession = useCallback(
    (id: string) => {
      controller.focusSession(id);
    },
    [controller],
  );

  const focusEmptyTab = useCallback(() => {
    controller.focusEmptyTab();
  }, [controller]);

  const deleteSession = useCallback(
    (id: string) => {
      void controller.deleteSession(id);
    },
    [controller],
  );

  const clearActiveSession = useCallback(() => {
    controller.clearActiveSession();
  }, [controller]);

  const sendPrompt = useCallback(
    (text: string, mentions?: WorkspaceFileRef[]) => {
      controller.sendPrompt(text, mentions);
    },
    [controller],
  );

  const runTerminal = useCallback(
    (command: string) => controller.runTerminal(command),
    [controller],
  );

  const respondPermission = useCallback(
    (optionId: string) => {
      controller.respondPermission(optionId);
    },
    [controller],
  );

  const cancel = useCallback(() => {
    controller.cancel();
  }, [controller]);

  const setConfigOption = useCallback(
    (configId: string, value: string | boolean) => {
      void controller.setConfigOption(configId, value);
    },
    [controller],
  );

  const applyConfigOptions = useCallback(
    (options: Array<{ configId: string; value: SessionConfigOptionValue }>) =>
      controller.applyConfigOptions(options),
    [controller],
  );

  const reconnect = useCallback(() => {
    void controller.reconnect();
  }, [controller]);

  return {
    workspace,
    session,
    sessions,
    openSessionIds,
    refreshSessions,
    newSession,
    adoptSession,
    openSession,
    openSessionTab,
    closeSessionTab,
    focusSession,
    focusEmptyTab,
    deleteSession,
    clearActiveSession,
    sendPrompt,
    runTerminal,
    respondPermission,
    cancel,
    setConfigOption,
    applyConfigOptions,
    reconnect,
  };
}

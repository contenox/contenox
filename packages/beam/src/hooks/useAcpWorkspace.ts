import { useCallback, useEffect } from 'react';
import type { SessionConfigOptionValue } from '../lib/acp';
import { useAcpWorkspaceContext } from '../lib/acp/AcpWorkspaceProvider';
import type { AcpSessionState } from './acpSessionState';
import type { AcpWorkspaceState } from './acpWorkspaceState';

export interface UseAcpWorkspaceResult {
  /** Connection status + `session/list` roster — see `acpWorkspaceState.ts`. */
  workspace: AcpWorkspaceState;
  /** The currently-open session's live timeline — see `acpSessionState.ts`. Reset whenever `workspace.activeSessionId` changes. */
  session: AcpSessionState;
  /** Pages `session/list` to completion and replaces the roster. */
  refreshSessions: () => void;
  /** Lazy-creation primitive (D5): creates a session, subscribes to it, and makes it active. Call this on first prompt submit, not on mount — see acpWorkspaceController.ts. */
  newSession: () => Promise<string>;
  /** Switches the open session (closing whichever was open). */
  openSession: (id: string) => void;
  deleteSession: (id: string) => void;
  /** Client-side reset of "which session is open" — no server-side deletion. Call before navigating to bare `/chat` from any "new session" affordance so the next lazy `newSession()` call mints a genuinely new session. See acpWorkspaceController.ts's doc comment. */
  clearActiveSession: () => void;
  /** No-ops with no active session — call `newSession()` first if `workspace.activeSessionId` is null. */
  sendPrompt: (text: string) => void;
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
  const { workspace, session, controller } = useAcpWorkspaceContext();

  useEffect(() => {
    void controller.connect();
  }, [controller]);

  const refreshSessions = useCallback(() => {
    void controller.refreshSessions();
  }, [controller]);

  const newSession = useCallback(() => controller.newSession(), [controller]);

  const openSession = useCallback(
    (id: string) => {
      void controller.openSession(id);
    },
    [controller],
  );

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
    (text: string) => {
      controller.sendPrompt(text);
    },
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
    refreshSessions,
    newSession,
    openSession,
    deleteSession,
    clearActiveSession,
    sendPrompt,
    respondPermission,
    cancel,
    setConfigOption,
    applyConfigOptions,
    reconnect,
  };
}

import { createContext, useContext, useEffect, useMemo, useReducer, useRef, type ReactNode } from 'react';
import {
  acpSessionReducer,
  initialAcpSessionState,
  type AcpSessionState,
} from '../../hooks/acpSessionState';
import {
  createAcpWorkspaceController,
  type AcpWorkspaceController,
} from '../../hooks/acpWorkspaceController';
import {
  acpWorkspaceReducer,
  initialAcpWorkspaceState,
  type AcpWorkspaceState,
} from '../../hooks/acpWorkspaceState';
import { getStoredApiToken } from '../fetch';
import { WebSocketTransport } from './transport';

/**
 * Context value: the ONE workspace controller for the whole app, plus the two
 * pieces of `useReducer` state it drives. `useAcpWorkspace.ts` is the
 * intended consumer — it reads this context and triggers `connect()` lazily
 * on its own mount (see that file's doc comment), so mounting this provider
 * high in the tree (e.g. in `App.tsx`) does not by itself open a WebSocket
 * for users who never visit an ACP-backed page.
 */
export interface AcpWorkspaceContextValue {
  workspace: AcpWorkspaceState;
  session: AcpSessionState;
  controller: AcpWorkspaceController;
}

const AcpWorkspaceContext = createContext<AcpWorkspaceContextValue | null>(null);

/** `ws(s)://<host>/acp[?token=...]` — the browser can't set a WS Authorization header, so the stored API token (if any) travels as a query param instead. Re-read on every call (not cached) so a reconnect after a token refresh picks up the new one — see acpWorkspaceController.ts's `createTransport` doc comment. */
function buildAcpWsUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const base = `${protocol}://${window.location.host}/acp`;
  const token = getStoredApiToken();
  return token ? `${base}?token=${encodeURIComponent(token)}` : base;
}

export interface AcpWorkspaceProviderProps {
  children: ReactNode;
}

/**
 * Owns exactly one `AcpWorkspaceController` (and its two reducers) for
 * however much of the app is mounted under it. Place this once, high in the
 * tree (e.g. wrapping the authenticated app shell) — `useAcpWorkspace()` is
 * safe to call from any number of components below it; they all share the
 * same connection and session state.
 */
export function AcpWorkspaceProvider({ children }: AcpWorkspaceProviderProps) {
  const [workspace, workspaceDispatch] = useReducer(acpWorkspaceReducer, initialAcpWorkspaceState);
  const [session, sessionDispatch] = useReducer(acpSessionReducer, initialAcpSessionState);

  // Constructed once per Provider instance (useRef, not useEffect) — building
  // the controller itself has no side effects (it doesn't connect); only
  // `controller.connect()`, called lazily by useAcpWorkspace()'s mount
  // effect, does.
  const controllerRef = useRef<AcpWorkspaceController | null>(null);
  if (!controllerRef.current) {
    controllerRef.current = createAcpWorkspaceController(
      { createTransport: () => new WebSocketTransport(buildAcpWsUrl()) },
      workspaceDispatch,
      sessionDispatch,
    );
  }

  // The controller (and its connection) outlives individual consumer
  // mounts/unmounts by design ("one controller app-wide") — only tear it
  // down when the provider itself unmounts, which in practice means app
  // teardown, not route navigation.
  useEffect(() => {
    return () => controllerRef.current?.dispose();
  }, []);

  const value = useMemo<AcpWorkspaceContextValue>(
    () => ({ workspace, session, controller: controllerRef.current! }),
    [workspace, session],
  );

  return <AcpWorkspaceContext.Provider value={value}>{children}</AcpWorkspaceContext.Provider>;
}

/** Internal: read the raw context, throwing a clear error outside a provider. `useAcpWorkspace.ts` (hooks/) is the ergonomic entry point apps should use. */
export function useAcpWorkspaceContext(): AcpWorkspaceContextValue {
  const ctx = useContext(AcpWorkspaceContext);
  if (!ctx) {
    throw new Error('useAcpWorkspace/useAcpWorkspaceContext must be used within an <AcpWorkspaceProvider>');
  }
  return ctx;
}

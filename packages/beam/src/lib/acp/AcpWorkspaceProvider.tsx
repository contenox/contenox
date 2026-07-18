import { createContext, useContext, useEffect, useMemo, useReducer, useRef, type ReactNode } from 'react';
import type { AcpSessionState } from '../../hooks/acpSessionState';
import {
  createAcpWorkspaceController,
  type AcpWorkspaceController,
} from '../../hooks/acpWorkspaceController';
import {
  acpSessionsReducer,
  acpWorkspaceReducer,
  initialAcpSessionsState,
  initialAcpWorkspaceState,
  selectFocusedSession,
  type AcpSessionsState,
  type AcpWorkspaceState,
} from '../../hooks/acpWorkspaceState';
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
  /** The FOCUSED session's live slice (backward-compatible single-view accessor) — derived from `sessions` via `selectFocusedSession`. */
  session: AcpSessionState;
  /** The full multiplexed sessions store: one live slice per open session, plus the focused pointer (workspace-tabs Slice 1). Slice 2's tab UI reads open sessions from here. */
  sessions: AcpSessionsState;
  controller: AcpWorkspaceController;
}

const AcpWorkspaceContext = createContext<AcpWorkspaceContextValue | null>(null);

/**
 * React 18/19 StrictMode (dev only, see `main.tsx`) mounts every component
 * twice: it runs an effect's setup, then its cleanup, then setup again — all
 * synchronously, on the SAME fiber/refs, to simulate an unmount+remount and
 * catch effects that don't tolerate it (see the React docs on Strict Mode).
 * Critically, no re-render happens between that cleanup and the following
 * setup call, so a controller recreated purely inside the render body's
 * `if (!controllerRef.current)` guard would never actually get rebuilt: the
 * guard doesn't run again until *something else* re-renders this component.
 *
 * Disposing the controller synchronously in the effect cleanup — as if this
 * were a genuine unmount — was exactly that bug: the simulated remount's
 * setup call then reused the same, now-permanently-disposed instance
 * forever (`connect()` etc. all reject with "disposed").
 *
 * The fix: defer the real `dispose()` by one macrotask, and let a same-tick
 * re-setup (StrictMode's simulated remount) cancel it outright — the
 * controller is never actually torn down, so there's nothing to rebuild. A
 * genuine unmount (e.g. logout, see `AuthenticatedAcpProvider` in
 * `App.tsx`) has no following setup call to cancel the timer, so it still
 * disposes for real, just one tick later — harmless, since nothing is using
 * the connection by then.
 */
export function createDeferredDisposer(dispose: () => void): { armForCleanup: () => void; cancelPendingDispose: () => void } {
  let timer: ReturnType<typeof setTimeout> | null = null;
  return {
    armForCleanup: () => {
      timer = setTimeout(() => {
        timer = null;
        dispose();
      }, 0);
    },
    cancelPendingDispose: () => {
      if (timer !== null) {
        clearTimeout(timer);
        timer = null;
      }
    },
  };
}

/**
 * `ws(s)://<host>/acp`. The HttpOnly `auth_token` session cookie authenticates
 * the upgrade automatically — it rides on the same-origin WebSocket handshake,
 * so no query param or header is needed and the UI never handles the token in
 * JS. (Programmatic clients that cannot send the cookie may pass `?token=<TOKEN>`
 * themselves; the browser never does.)
 */
function buildAcpWsUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${protocol}://${window.location.host}/acp`;
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
  const [sessions, sessionsDispatch] = useReducer(acpSessionsReducer, initialAcpSessionsState);

  // Constructed once per Provider instance (useRef, not useEffect) — building
  // the controller itself has no side effects (it doesn't connect); only
  // `controller.connect()`, called lazily by useAcpWorkspace()'s mount
  // effect, does.
  const controllerRef = useRef<AcpWorkspaceController | null>(null);
  if (!controllerRef.current) {
    controllerRef.current = createAcpWorkspaceController(
      { createTransport: () => new WebSocketTransport(buildAcpWsUrl()) },
      workspaceDispatch,
      sessionsDispatch,
    );
  }

  // The controller (and its connection) outlives individual consumer
  // mounts/unmounts by design ("one controller app-wide") — only tear it
  // down when the provider itself unmounts, which in practice means app
  // teardown, not route navigation. See `createDeferredDisposer`'s doc
  // comment for why the actual `dispose()` call is deferred+cancellable
  // rather than synchronous.
  const disposerRef = useRef(createDeferredDisposer(() => controllerRef.current?.dispose()));
  useEffect(() => {
    const disposer = disposerRef.current;
    disposer.cancelPendingDispose();
    return () => disposer.armForCleanup();
  }, []);

  // The single-view `session` accessor is the focused slice — a stable
  // reference while the focused slice is unchanged, so consumers reading only
  // `session` aren't forced to re-render by a background session's updates.
  const session = useMemo(() => selectFocusedSession(sessions), [sessions]);

  const value = useMemo<AcpWorkspaceContextValue>(
    () => ({ workspace, session, sessions, controller: controllerRef.current! }),
    [workspace, session, sessions],
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

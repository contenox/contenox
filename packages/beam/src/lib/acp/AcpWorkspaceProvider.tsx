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
  // teardown, not route navigation. See `createDeferredDisposer`'s doc
  // comment for why the actual `dispose()` call is deferred+cancellable
  // rather than synchronous.
  const disposerRef = useRef(createDeferredDisposer(() => controllerRef.current?.dispose()));
  useEffect(() => {
    const disposer = disposerRef.current;
    disposer.cancelPendingDispose();
    return () => disposer.armForCleanup();
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

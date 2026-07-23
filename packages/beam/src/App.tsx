import { ErrorBoundary, Spinner } from '@contenox/ui';
import { Suspense, lazy, useContext, type ReactNode } from 'react';
import { Route, HashRouter as Router, Routes } from 'react-router-dom';
import './app.css';
import { Layout } from './components/Layout';
import { NavbarSlotProvider } from './components/NavbarSlot';
import { CommandPalette } from './components/palette/CommandPalette';
import { ProtectedRoute } from './components/ProtectedRoute';
import { AcpSessionSidebar } from './components/sidebar/AcpSessionSidebar';
import { routes } from './config/routes';
import { AuthProvider } from './lib/AuthProvider';
import { AcpWorkspaceProvider } from './lib/acp/AcpWorkspaceProvider';
import { StagedAgentProvider } from './lib/stagedAgent';
import { StagedRootProvider } from './lib/stagedRoot';
import { AdoptIntentProvider } from './lib/adoptIntent';
import { AuthContext } from './lib/authContext';

const AuthPage = lazy(() => import('./pages/public/login/AuthPage'));

/**
 * The remote-access gate. While /ui/auth-status is loading it shows a spinner;
 * when the server requires login and this browser has no valid session cookie
 * it renders the login page in place of the whole app; otherwise it renders the
 * app. Locally (no TOKEN) `authRequired` is false, so this is a transparent
 * pass-through — the app appears with no prompt.
 */
function AuthGate({ children }: { children: ReactNode }) {
  const { isLoading, authRequired, authenticated } = useContext(AuthContext);
  if (isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <Spinner />
      </div>
    );
  }
  if (authRequired && !authenticated) {
    return (
      <Suspense
        fallback={
          <div className="flex min-h-screen items-center justify-center">
            <Spinner />
          </div>
        }>
        <AuthPage />
      </Suspense>
    );
  }
  return <>{children}</>;
}

/**
 * Hoists the one app-wide `AcpWorkspaceProvider` above `Layout` (sidebar +
 * routed page content both need it — see AcpSessionSidebar.tsx and
 * pages/chat/AcpChatPage.tsx) — gated on auth so an unauthenticated visitor
 * (e.g. sitting on `/login`) never triggers a connection attempt. Mounting
 * the provider still doesn't itself open a WebSocket: `useAcpWorkspace()`
 * connects lazily on its own mount (see AcpWorkspaceProvider.tsx's doc
 * comment) — this only controls *whether* that becomes possible.
 */
function AuthenticatedAcpProvider({ children }: { children: ReactNode }) {
  const { user } = useContext(AuthContext);
  if (!user) return <>{children}</>;
  return <AcpWorkspaceProvider>{children}</AcpWorkspaceProvider>;
}

/**
 * Mounts the goto-anything command palette only for authenticated users — it
 * reads the ACP session roster and the query cache, both of which only exist
 * under `AuthenticatedAcpProvider`. Rendered as a sibling of the shell so its
 * Cmd/Ctrl+K overlay (a portal to <body>) floats above every route.
 */
function AuthedCommandPalette() {
  const { user } = useContext(AuthContext);
  if (!user) return null;
  return <CommandPalette />;
}

export default function App() {
  return (
    <Router>
      <AuthProvider>
        <AuthGate>
          <AuthenticatedAcpProvider>
            {/* Shares the "next new chat's agent" staging between the sessions
                sidebar (which seeds it) and the empty chat surface (which shows,
                changes, and consumes it). See lib/stagedAgent.tsx. */}
            <StagedAgentProvider>
            {/* Shares "the next new chat's PROJECT (cwd)" between the Projects
                admin page (whose launcher rows seed it) and the empty chat
                surface (which consumes it one-shot into its Workspace pick). The
                sibling of StagedAgentProvider. See lib/stagedRoot.tsx. */}
            <StagedRootProvider>
            {/* Shares "the next chat should ADOPT this running unit" between the
                fleet board / mission detail (which stage it) and the chat
                workspace (which eagerly adopts once connected). See
                lib/adoptIntent.tsx. */}
            <AdoptIntentProvider>
            {/* Global command palette (Cmd/Ctrl+K). Sibling of the shell so its
                portal overlay floats above every route; inert until opened. */}
            <AuthedCommandPalette />
            {/* Lets a routed page (today the chat surface) project chrome into
                the shell's navbar center; wraps Layout so both the navbar and
                the routed pages inside `mainContent` share the one slot. */}
            <NavbarSlotProvider>
            <Layout
              sidebarContent={({ setIsOpen }) => <AcpSessionSidebar setIsOpen={setIsOpen} />}
              defaultOpen={true}
              mainContent={
                <ErrorBoundary>
                  <Suspense
                    fallback={
                      <div className="flex min-h-screen items-center justify-center">
                        <Spinner />
                      </div>
                    }>
                    <Routes>
                      {routes.map((route, index) => {
                        const Element = route.element;
                        const wrappedElement =
                          route.protected !== false ? (
                            <ProtectedRoute>
                              <Element />
                            </ProtectedRoute>
                          ) : (
                            <Element />
                          );
                        return <Route key={index} path={route.path} element={wrappedElement} />;
                      })}
                    </Routes>
                  </Suspense>
                </ErrorBoundary>
              }
            />
            </NavbarSlotProvider>
            </AdoptIntentProvider>
            </StagedRootProvider>
            </StagedAgentProvider>
          </AuthenticatedAcpProvider>
        </AuthGate>
      </AuthProvider>
    </Router>
  );
}

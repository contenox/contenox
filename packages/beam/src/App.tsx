import { ErrorBoundary, Spinner } from '@contenox/ui';
import { Suspense, useContext, type ReactNode } from 'react';
import { Route, HashRouter as Router, Routes } from 'react-router-dom';
import './app.css';
import { Layout } from './components/Layout';
import { ProtectedRoute } from './components/ProtectedRoute';
import { AcpSessionSidebar } from './components/sidebar/AcpSessionSidebar';
import { routes } from './config/routes';
import { AuthProvider } from './lib/AuthProvider';
import { AuthContext } from './lib/authContext';
import { AcpWorkspaceProvider } from './lib/acp/AcpWorkspaceProvider';

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

export default function App() {
  return (
    <Router>
      <AuthProvider>
        <AuthenticatedAcpProvider>
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
        </AuthenticatedAcpProvider>
      </AuthProvider>
    </Router>
  );
}

import { useQuery, useQueryClient } from '@tanstack/react-query';
import React, { useCallback, useMemo } from 'react';
import { api } from './api';
import { AuthContext } from './authContext';
import { authKeys } from './queryKeys';
import type { AuthenticatedUser } from './types';

const localUser: AuthenticatedUser = {
  id: 'local-user',
  subject: 'local-user',
  email: 'local@localhost',
  friendlyName: 'Local user',
  username: 'local-user',
};

/**
 * Queries the server's remote-access status (GET /ui/auth-status) and exposes it
 * on AuthContext. When a TOKEN is configured and this browser has no valid
 * session cookie, `user` is withheld so the gate (see App.tsx's AuthGate) can
 * render the login page instead of the app. Locally (no TOKEN), the query
 * returns {required:false, authenticated:true} and the app is usable with zero
 * prompts — matching the loopback-no-TOKEN baseline. The status endpoint is
 * always readable, so this never depends on already being authenticated.
 */
export const AuthProvider = ({ children }: { children: React.ReactNode }) => {
  const queryClient = useQueryClient();
  const { data, isLoading, isError, error } = useQuery({
    queryKey: authKeys.status(),
    queryFn: api.getAuthStatus,
    staleTime: 0,
    retry: false,
  });

  const refresh = useCallback(() => {
    void queryClient.invalidateQueries({ queryKey: authKeys.status() });
  }, [queryClient]);

  const value = useMemo(() => {
    // If the status query itself fails, fail safe toward the gate: treat access
    // as required-and-unauthenticated so a misconfigured/remote client sees the
    // login page rather than a broken app spraying 401s.
    const authRequired = isError ? true : (data?.required ?? false);
    const authenticated = isError ? false : (data?.authenticated ?? false);
    const granted = !authRequired || authenticated;
    return {
      user: granted ? localUser : undefined,
      isLoading,
      isError,
      error: error instanceof Error ? error : null,
      authRequired,
      authenticated,
      refresh,
    };
  }, [data, isLoading, isError, error, refresh]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
};

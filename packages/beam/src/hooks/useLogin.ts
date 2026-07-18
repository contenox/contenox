import {
  UseMutationOptions,
  UseMutationResult,
  useMutation,
  useQueryClient,
} from '@tanstack/react-query';
import { useContext } from 'react';
import { api } from '../lib/api';
import { AuthContext } from '../lib/authContext';
import { authKeys } from '../lib/queryKeys';
import { AuthStatus } from '../lib/types';

/**
 * Exchanges the shared access token for an HttpOnly session cookie via
 * POST /ui/login. On success it re-queries /ui/auth-status (both by invalidating
 * the query and via AuthContext.refresh), so App.tsx's AuthGate swaps the login
 * page for the app. A wrong token rejects with a 401 ApiError surfaced as
 * `error` for the form to display.
 */
export function useLogin(
  options?: UseMutationOptions<AuthStatus, Error, string>,
): UseMutationResult<AuthStatus, Error, string, unknown> {
  const queryClient = useQueryClient();
  const { refresh } = useContext(AuthContext);

  return useMutation<AuthStatus, Error, string, unknown>({
    mutationFn: (token: string) => api.loginWithToken(token),
    onSuccess: (data, variables, context, onMutateResult) => {
      void queryClient.invalidateQueries({ queryKey: authKeys.status() });
      refresh();
      options?.onSuccess?.(data, variables, context, onMutateResult);
    },
  });
}

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import { useCallback } from 'react';
import { api } from '../lib/api';
import { ApiError } from '../lib/fetch';
import { terminalKeys } from '../lib/queryKeys';
import type { TerminalSession, TerminalSessionCreate } from '../lib/types';

const TERMINAL_PROBE_STALE_MS = 5 * 60 * 1000;

export function isTooManyTerminalSessionsError(error: unknown): boolean {
  if (!(error instanceof ApiError) || error.status !== 422) return false;
  const msg = error.message.toLowerCase();
  return msg.includes('too many') || msg.includes('concurrent');
}

/** True when GET /api/terminal/sessions succeeds (terminal routes are enabled). */
export function useTerminalAvailable(): UseQueryResult<TerminalSession[], Error> {
  return useQuery<TerminalSession[], Error>({
    queryKey: terminalKeys.probe(),
    queryFn: api.listTerminalSessions,
    retry: false,
    staleTime: TERMINAL_PROBE_STALE_MS,
  });
}

export function useTerminalSessions(): UseQueryResult<TerminalSession[], Error> {
  return useQuery<TerminalSession[], Error>({
    queryKey: terminalKeys.sessions(),
    queryFn: api.listTerminalSessions,
  });
}

export function useTerminalSession(id: string, options?: { enabled?: boolean }) {
  return useQuery<TerminalSession>({
    queryKey: terminalKeys.session(id),
    queryFn: () => api.getTerminalSession(id),
    enabled: options?.enabled ?? !!id,
  });
}

export function useCreateTerminalSession(): UseMutationResult<
  TerminalSessionCreate,
  Error,
  { cwd: string; cols?: number; rows?: number }
> {
  const queryClient = useQueryClient();
  return useMutation<TerminalSessionCreate, Error, { cwd: string; cols?: number; rows?: number }>({
    mutationFn: body => api.createTerminalSession(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: terminalKeys.sessions() });
      queryClient.invalidateQueries({ queryKey: terminalKeys.probe() });
    },
  });
}

export function useDeleteTerminalSession(): UseMutationResult<void, Error, string> {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: api.deleteTerminalSession,
    onSuccess: (_, deletedId) => {
      queryClient.invalidateQueries({ queryKey: terminalKeys.sessions() });
      queryClient.invalidateQueries({ queryKey: terminalKeys.probe() });
      queryClient.removeQueries({ queryKey: terminalKeys.session(deletedId) });
    },
  });
}

/** Deletes every listed session (best-effort). Used when the server returns 422 too-many. */
export function usePruneTerminalSessions() {
  const queryClient = useQueryClient();
  const deleteSession = useDeleteTerminalSession();

  return useCallback(async () => {
    const list = await queryClient.fetchQuery({
      queryKey: terminalKeys.sessions(),
      queryFn: api.listTerminalSessions,
    });
    await Promise.all(
      list.map(session => deleteSession.mutateAsync(session.id).catch(() => undefined)),
    );
  }, [queryClient, deleteSession]);
}
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';
import { workspaceRootKeys } from '../lib/queryKeys';

/**
 * Registers a folder as a managed workspace root. The POST returns the full
 * roots list, but the projects page reads the roots through `useWorkspaceRoots`
 * (the shared, cached read), so on success we just invalidate that key rather
 * than threading the response back — a single source of truth for the list.
 */
export function useAddWorkspaceRoot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (body: { path: string; name: string }) => api.addWorkspaceRoot(body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workspaceRootKeys.all });
    },
  });
}

/**
 * Forgets a managed workspace root (idempotent server-side). Same invalidation
 * shape as the add mutation so the projects list re-reads from `useWorkspaceRoots`.
 */
export function useForgetWorkspaceRoot() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (path: string) => api.removeWorkspaceRoot(path),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: workspaceRootKeys.all });
    },
  });
}

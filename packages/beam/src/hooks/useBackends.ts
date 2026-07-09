import {
  useMutation,
  UseMutationResult,
  useQueryClient,
  useSuspenseQuery,
} from '@tanstack/react-query';
import { api } from '../lib/api';
import { backendKeys, setupKeys } from '../lib/queryKeys';
import { Backend, PushModelResult } from '../lib/types';

export function useBackends() {
  return useSuspenseQuery<Backend[]>({
    queryKey: backendKeys.all,
    queryFn: api.getBackends,
    // Backend list + merged runtime fields; avoid tight polling (see TabbedPage mountActivePanelOnly).
    refetchInterval: 15_000,
  });
}

export function useCreateBackend(): UseMutationResult<Backend, Error, Partial<Backend>, unknown> {
  const queryClient = useQueryClient();
  return useMutation<Backend, Error, Partial<Backend>>({
    mutationFn: api.createBackend,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: backendKeys.all });
      queryClient.invalidateQueries({ queryKey: setupKeys.status() });
    },
  });
}

export function useDeleteBackend(): UseMutationResult<void, Error, string, unknown> {
  const queryClient = useQueryClient();
  return useMutation<void, Error, string>({
    mutationFn: api.deleteBackend,
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: backendKeys.all });
      queryClient.invalidateQueries({ queryKey: setupKeys.status() });
    },
  });
}

export function useUpdateBackend() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<Backend> }) =>
      api.updateBackend(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: backendKeys.all });
      queryClient.invalidateQueries({ queryKey: setupKeys.status() });
    },
  });
}

export function usePushModel(
  backendId: string,
): UseMutationResult<PushModelResult, Error, { name: string; file: File }, unknown> {
  const queryClient = useQueryClient();
  return useMutation<PushModelResult, Error, { name: string; file: File }>({
    mutationFn: ({ name, file }) => api.pushModel(backendId, name, file),
    onSuccess: () => {
      // The pushed model only shows up in this backend's pulledModels once the
      // runtime reconciles the node again; invalidating just prompts an earlier refetch.
      queryClient.invalidateQueries({ queryKey: backendKeys.all });
    },
  });
}


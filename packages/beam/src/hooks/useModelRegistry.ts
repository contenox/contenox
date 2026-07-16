import { useMutation, useQueryClient, useSuspenseQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { backendKeys, modeldKeys, modelRegistryKeys } from '../lib/queryKeys';
import { ModelRegistryEntry } from '../lib/types';

export function useModelRegistry() {
  return useSuspenseQuery({
    queryKey: modelRegistryKeys.all,
    queryFn: api.listModelRegistry,
  });
}

export function useCreateModelRegistryEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (data: Omit<ModelRegistryEntry, 'id' | 'createdAt' | 'updatedAt'>) =>
      api.createModelRegistryEntry(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: modelRegistryKeys.all });
    },
  });
}

export function useDeleteModelRegistryEntry() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.deleteModelRegistryEntry(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: modelRegistryKeys.all });
    },
  });
}

export function useDownloadModel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (name: string) => api.downloadModel(name),
    onSuccess: () => {
      // A downloaded model shows up in a backend's pulledModels and in the local
      // modeld runtime's model list once the runtime reconciles it — invalidate
      // both alongside the registry so those lists (and the ACP model dropdown
      // built from them) refresh instead of staying stale until an unrelated
      // page happens to trigger a refetch.
      queryClient.invalidateQueries({ queryKey: modelRegistryKeys.all });
      queryClient.invalidateQueries({ queryKey: backendKeys.all });
      queryClient.invalidateQueries({ queryKey: modeldKeys.all });
    },
  });
}

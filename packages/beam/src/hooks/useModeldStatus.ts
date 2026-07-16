import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';
import { backendKeys, modeldKeys } from '../lib/queryKeys';

export function useModeldStatus() {
  return useQuery({
    queryKey: modeldKeys.status(),
    queryFn: () => api.getModeldStatus(),
    refetchInterval: 15_000,
  });
}

export function useModeldModels() {
  return useQuery({
    queryKey: modeldKeys.models(),
    queryFn: () => api.getModeldModels(),
    refetchInterval: 30_000,
  });
}

export function useModeldCapacity(model: string) {
  return useQuery({
    queryKey: modeldKeys.capacity(model),
    queryFn: () => api.getModeldCapacity(model),
    // Gated behind an explicit user action (see LocalRuntimeSection's "Load
    // estimate" button) — this probe runs an expensive provider.Describe fit
    // check on the runtime, so it must never auto-fire just because a model
    // got selected. staleTime keeps a loaded estimate from refetching on
    // window focus once the user has asked for it.
    enabled: false,
    staleTime: 60_000,
  });
}

export function useUnloadModeld() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (expectedGeneration: number) => api.unloadModeld(expectedGeneration),
    onSuccess: () => {
      // The local modeld backend's entry in the backend list (active model,
      // pulledModels) reflects the slot state too — invalidate it alongside
      // modeld's own queries so both the backends page and the ACP model
      // dropdown pick up the change.
      void queryClient.invalidateQueries({ queryKey: modeldKeys.all });
      void queryClient.invalidateQueries({ queryKey: backendKeys.all });
    },
  });
}

export function useLoadModeld() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ model, expectedGeneration }: { model: string; expectedGeneration?: number }) =>
      api.loadModeld(model, expectedGeneration),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modeldKeys.all });
      void queryClient.invalidateQueries({ queryKey: backendKeys.all });
    },
  });
}

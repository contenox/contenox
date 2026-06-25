import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { api } from '../lib/api';
import { modeldKeys } from '../lib/queryKeys';

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
    enabled: model.trim().length > 0,
  });
}

export function useUnloadModeld() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (expectedGeneration: number) => api.unloadModeld(expectedGeneration),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: modeldKeys.all });
    },
  });
}

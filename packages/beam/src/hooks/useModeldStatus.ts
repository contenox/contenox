import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { modeldKeys } from '../lib/queryKeys';

export function useModeldStatus() {
  return useQuery({
    queryKey: modeldKeys.status(),
    queryFn: () => api.getModeldStatus(),
    refetchInterval: 15_000,
  });
}

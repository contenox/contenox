import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { agentKeys } from '../lib/queryKeys';
import { Agent } from '../lib/types';

const defaultListParams = {
  limit: 100 as number | undefined,
  cursor: undefined as string | undefined,
};

export function useAgents(params?: { limit?: number; cursor?: string }) {
  const p = { ...defaultListParams, ...params };
  return useQuery<Agent[]>({
    queryKey: agentKeys.list(p),
    queryFn: () => api.getAgents(p),
  });
}

export function useAgentByName(name: string, options?: { enabled?: boolean }) {
  return useQuery<Agent>({
    queryKey: agentKeys.byName(name),
    queryFn: () => api.getAgentByName(name),
    enabled: options?.enabled ?? !!name,
  });
}

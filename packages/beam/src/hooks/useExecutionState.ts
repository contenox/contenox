import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { activityKeys } from '../lib/queryKeys';
import type { CapturedStateUnit } from '../lib/types';

/**
 * Hydrates a past turn's execution evidence from the durable KV state via
 * GET /api/execution-state. Completed state is immutable, so it never goes
 * stale; `enabled` gates the fetch to turns that actually need hydration
 * (no retained live run this session).
 */
export function useExecutionState(requestId: string | undefined, enabled: boolean) {
  return useQuery<CapturedStateUnit[]>({
    queryKey: activityKeys.state(requestId ?? ''),
    queryFn: async () => (await api.getExecutionState(requestId!)).state,
    enabled: enabled && !!requestId,
    staleTime: Infinity,
    gcTime: 10 * 60_000,
  });
}

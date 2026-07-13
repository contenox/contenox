import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { activityKeys } from '../lib/queryKeys';
import type { TaskEvent } from '../lib/types';

/**
 * Hydrates a past turn's full work log (tool calls, diffs, approvals) from
 * the durable event journal via GET /api/execution-events. Journaled events
 * are immutable once the run completed, so they never go stale.
 */
export function useExecutionEvents(requestId: string | undefined, enabled: boolean) {
  return useQuery<TaskEvent[]>({
    queryKey: activityKeys.events(requestId ?? ''),
    queryFn: async () => (await api.getExecutionEvents(requestId!)).events,
    enabled: enabled && !!requestId,
    staleTime: Infinity,
    gcTime: 10 * 60_000,
  });
}

import { useQuery, type UseQueryResult } from '@tanstack/react-query';
import { api } from '../lib/api';
import { ApiError } from '../lib/fetch';
import { missionKeys } from '../lib/queryKeys';
import type { MissionChangesResponse, MissionFileDiff } from '../lib/types';

/**
 * `null` is the deliberate "feature absent" sentinel, distinct from a present
 * response with an empty `files`: the mission-changes route group is nil-gated
 * server-side and 404s when a serve does not expose change tracking (see
 * runtime/internal/missionchangesapi + api.getMissionChanges). Both the Changes
 * tab and the scope/anomaly surfacing hide on absent, exactly like the workspace
 * picker's roots sentinel.
 */
export type MissionChangesData = MissionChangesResponse | null;

/**
 * Fetches a mission's changes, folding the 404 into the absent sentinel rather
 * than an error — a serve without change tracking is a quiet "no Changes tab",
 * never a red banner. Extracted from the hook so the present / 404 / other-error
 * branches are unit-testable against a mocked `api`.
 */
export async function fetchMissionChanges(id: string): Promise<MissionChangesData> {
  try {
    return await api.getMissionChanges(id);
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null;
    throw err;
  }
}

export interface MissionChangesState {
  /** The changes response, or undefined while loading / when absent. */
  data?: MissionChangesResponse;
  /** True when serve exposes no change tracking (404) — hide the tab + scope surfacing. */
  isAbsent: boolean;
  isLoading: boolean;
  error: Error | null;
  refetch: () => void;
}

/** Pure state derivation from the query result, testable without react-query. */
export function deriveMissionChangesState(
  data: MissionChangesData | undefined,
  isLoading: boolean,
  error: Error | null,
  refetch: () => void,
): MissionChangesState {
  return {
    data: data ?? undefined,
    isAbsent: data === null,
    isLoading,
    error,
    refetch,
  };
}

/**
 * A mission's aggregated changed-files list + scope — the attention layer's
 * read surface. Polled on the same short interval as the other mission feeds so
 * a change landing while the page is open appears without a manual refresh;
 * shared by cache key with the Changes tab and the header's scope/anomaly chips,
 * so opening the tab reuses this fetch (the scope surfacing is async enhancement
 * that never blocks the header — the Sublime-nature law). Disabled on an empty
 * id so a still-resolving route param never fires a request.
 */
export function useMissionChanges(id: string, opts?: { enabled?: boolean }): MissionChangesState {
  const query = useQuery<MissionChangesData, Error>({
    queryKey: missionKeys.changes(id),
    queryFn: () => fetchMissionChanges(id),
    enabled: !!id && opts?.enabled !== false,
    refetchInterval: 5000,
    retry: false,
  });
  return deriveMissionChangesState(query.data, query.isLoading, query.error ?? null, () =>
    query.refetch(),
  );
}

/**
 * One changed file's diff (`GET /api/missions/{id}/changes/diff?path=`), fetched
 * lazily — `enabled` is the row's expanded state, so Monaco never loads and no
 * diff is fetched until a file is opened. 404 (path not a changed file) and 422
 * (missing path) surface as errors in the diff panel; they are real request
 * faults, not the feature-absent 404 the list folds.
 */
export function useMissionChangeDiff(
  id: string,
  path: string | null,
  opts?: { enabled?: boolean },
): UseQueryResult<MissionFileDiff, Error> {
  return useQuery<MissionFileDiff, Error>({
    queryKey: missionKeys.changeDiff(id, path ?? ''),
    queryFn: () => api.getMissionChangeDiff(id, path as string),
    enabled: !!id && !!path && opts?.enabled !== false,
    staleTime: 30_000,
    retry: false,
  });
}

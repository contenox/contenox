import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { ApiError } from '../lib/fetch';
import { workspaceRootKeys } from '../lib/queryKeys';
import type { WorkspaceRoot } from '../lib/types';
import { activeWorkspaceRoot } from '../lib/workspaceRoots';

/**
 * `null` is the deliberate "feature absent" sentinel, distinct from `[]` (an
 * empty but configured allowlist): `GET /workspace/roots` is nil-gated
 * server-side and 404s when serve has no workspace-root allowlist at all (see
 * localfileapi.AddWorkspaceRootsRoutes). The picker hides its affordances on
 * absent, but an empty configured allowlist is a real — if unusual — state.
 */
export type WorkspaceRootsData = WorkspaceRoot[] | null;

/**
 * Fetches the allowlist, folding the 404 into the absent sentinel rather than
 * an error so a serve that predates the endpoint (or has no allowlist) is a
 * quiet "no picker", never a red error banner. Extracted from the hook so the
 * present / 404 / empty branches are unit-testable against a mocked `api`.
 */
export async function fetchWorkspaceRoots(): Promise<WorkspaceRootsData> {
  try {
    const resp = await api.getWorkspaceRoots();
    return resp.roots ?? [];
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null;
    throw err;
  }
}

export interface WorkspaceRootsState {
  /** The configured roots (empty array when absent or genuinely empty). */
  roots: WorkspaceRoot[];
  /** The default root (or first), when any; undefined otherwise. */
  defaultRoot?: WorkspaceRoot;
  /** True when serve exposes no allowlist (404) — hide picker affordances. */
  isAbsent: boolean;
  isLoading: boolean;
  error: Error | null;
}

/** Pure state derivation from the query result, testable without react-query. */
export function deriveWorkspaceRootsState(
  data: WorkspaceRootsData | undefined,
  isLoading: boolean,
  error: Error | null,
): WorkspaceRootsState {
  const isAbsent = data === null;
  const roots = data ?? [];
  return {
    roots,
    defaultRoot: activeWorkspaceRoot(roots),
    isAbsent,
    isLoading,
    error,
  };
}

/**
 * The workspace-root allowlist as a picker-ready state. Cached for a minute —
 * the allowlist is serve configuration, not live telemetry, so it does not need
 * the short poll the fleet/mission feeds use. `retry: false` because the one
 * error we expect (404) is already folded into the absent sentinel, and a
 * genuine failure should surface at once rather than after three silent retries.
 */
export function useWorkspaceRoots(): WorkspaceRootsState {
  const query = useQuery<WorkspaceRootsData, Error>({
    queryKey: workspaceRootKeys.list(),
    queryFn: fetchWorkspaceRoots,
    staleTime: 60_000,
    retry: false,
  });
  return deriveWorkspaceRootsState(query.data, query.isLoading, query.error ?? null);
}

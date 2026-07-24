import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { workspaceAccessKeys } from '../lib/queryKeys';
import {
  dedupeSortPaths,
  fetchWorkspaceAccess,
  verdictsToMap,
  HITL_POLICY_DEFAULT_VALUE,
  type PathVerdict,
} from '../lib/workspaceAccess';

/**
 * The workspace verdict source: batches the currently-loaded workspace paths and
 * POSTs them to `POST /workspace/access?root=&policy=`, returning a
 * `path → verdict` map the panel merges into the raw `/files` (or `/workspace/find`)
 * listing. Both the lazy tree (paths = the loaded-directory union) and the
 * recursive-find filter (paths = the match set) feed the SAME hook — it is
 * agnostic to where the path list comes from.
 *
 * The lazy tree grows its path union as directories expand; the hook keys the
 * react-query cache on the sorted union (see `workspaceAccessKeys.batch`) and
 * uses `keepPreviousData` so an expand shows the previous verdicts until the new
 * (superset) batch resolves, rather than flashing empty. Kept a thin wrapper over
 * the pure helpers in `lib/workspaceAccess` (which the tests exercise directly).
 */
export interface UseWorkspaceAccessOptions {
  /** Workspace root the paths are relative to; a nullish root disables the query. */
  root: string | null | undefined;
  /** HITL policy to evaluate against; the Default sentinel / nullish omits `policy=`. */
  policy?: string | null;
  /** The loaded-path union to evaluate (files AND directories). Empty disables the query. */
  paths: readonly string[];
  /** Master gate — the agent-view toggle. Off ⇒ no request, empty verdict map. */
  enabled: boolean;
}

export interface WorkspaceAccessResult {
  /** `path → verdict` lookup; empty until the first batch resolves (or when disabled). */
  verdicts: Map<string, PathVerdict>;
  /** The RESOLVED policy name the server evaluated (may differ from the requested name). */
  policyName?: string;
  /** True while a batch is in flight (including a `keepPreviousData` refetch). */
  isLoading: boolean;
  /** The batch failure message, if the last request failed. */
  error: string | null;
}

const EMPTY_VERDICTS: Map<string, PathVerdict> = new Map();

export function useWorkspaceAccess({
  root,
  policy,
  paths,
  enabled,
}: UseWorkspaceAccessOptions): WorkspaceAccessResult {
  const sortedPaths = useMemo(() => dedupeSortPaths(paths), [paths]);
  const pathsKey = sortedPaths.join('\n');
  const trimmedPolicy = policy?.trim();
  const policyKey =
    trimmedPolicy && trimmedPolicy !== HITL_POLICY_DEFAULT_VALUE ? trimmedPolicy : '';
  const active = enabled && !!root && sortedPaths.length > 0;

  const query = useQuery({
    queryKey: workspaceAccessKeys.batch(root ?? '', policyKey, pathsKey),
    queryFn: () => fetchWorkspaceAccess(root as string, policy, sortedPaths),
    enabled: active,
    placeholderData: keepPreviousData,
    staleTime: 30_000,
    retry: false,
  });

  // Gated on `active` so a disabled overlay (agent view off, or no paths yet)
  // reports an empty map even if a prior batch is still warm in the cache.
  const verdicts = useMemo(
    () => (active && query.data ? verdictsToMap(query.data.verdicts) : EMPTY_VERDICTS),
    [active, query.data],
  );

  return {
    verdicts,
    policyName: active ? query.data?.policyName : undefined,
    isLoading: active && query.isFetching,
    error: active && query.error instanceof Error ? query.error.message : null,
  };
}

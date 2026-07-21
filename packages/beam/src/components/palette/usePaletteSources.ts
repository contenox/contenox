import { useQueryClient, type QueryClient } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useAcpWorkspace } from '../../hooks/useAcpWorkspace';
import { startAdoptSession, useAdoptIntent } from '../../lib/adoptIntent';
import {
  agentKeys,
  approvalKeys,
  fleetKeys,
  missionKeys,
  workspaceRootKeys,
} from '../../lib/queryKeys';
import { buildPaletteItems } from '../../lib/palette/providers';
import type { PaletteItem } from '../../lib/palette/types';
import type { Agent, FleetEntry, HITLApproval, Mission, WorkspaceRoot } from '../../lib/types';

/**
 * Assembles the palette's item set from data the app ALREADY has — react-query
 * cache snapshots plus the ACP session roster — with no fetching of its own.
 * This is a deliberate latency choice (the Sublime-nature law): reads are
 * `getQueryData`, never `useQuery`, so the palette mounts no background pollers
 * and adds no observers to the short-poll fleet/mission/approval feeds. The
 * consequence is intentional and cheap: a source whose cache is still cold
 * (never-visited page) simply contributes nothing until the cache fills — the
 * palette never blocks, never spins, and never waits on a network round-trip.
 *
 * The whole set is rebuilt only while `open` is true, and re-memoized when the
 * (reactive) session roster changes; cache staleness between opens is fine for
 * a palette.
 */
export function usePaletteSources(open: boolean): PaletteItem[] {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { t } = useTranslation();
  const { setAdoptIntent } = useAdoptIntent();
  const { workspace } = useAcpWorkspace();
  const sessions = workspace.sessions;

  return useMemo<PaletteItem[]>(() => {
    if (!open) return [];
    return buildPaletteItems({
      t,
      navigate,
      startAdopt: ref => startAdoptSession(ref, { setAdoptIntent, navigate }),
      missions: queryClient.getQueryData<Mission[]>(missionKeys.list()) ?? [],
      fleet: queryClient.getQueryData<FleetEntry[]>(fleetKeys.list()) ?? [],
      agents: readAgentsFromCache(queryClient),
      approvals: queryClient.getQueryData<HITLApproval[]>(approvalKeys.list()) ?? [],
      sessions,
      workspaceRoots: readWorkspaceRootsFromCache(queryClient),
    });
    // `t` changes identity on language switch (re-labels items); `sessions` is
    // the one reactive source. The cache reads are snapshotted per open.
  }, [open, t, navigate, setAdoptIntent, queryClient, sessions]);
}

/**
 * The agent list is cached under a params-keyed query (limit/cursor), so match
 * the family rather than guessing the exact params object: take the first
 * populated agent-list snapshot, else the first present (possibly empty) one.
 */
function readAgentsFromCache(queryClient: QueryClient): Agent[] {
  const entries = queryClient.getQueriesData<Agent[]>({ queryKey: agentKeys.all });
  for (const [, data] of entries) if (Array.isArray(data) && data.length > 0) return data;
  for (const [, data] of entries) if (Array.isArray(data)) return data;
  return [];
}

/**
 * Workspace roots cache holds `WorkspaceRoot[] | null` — `null` is the
 * "feature absent" sentinel (serve 404). Either way, absent/cold reads to an
 * empty array so the workspace source just yields nothing.
 */
function readWorkspaceRootsFromCache(queryClient: QueryClient): WorkspaceRoot[] {
  const data = queryClient.getQueryData<WorkspaceRoot[] | null>(workspaceRootKeys.list());
  return Array.isArray(data) ? data : [];
}

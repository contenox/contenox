import { useQuery, UseQueryResult } from '@tanstack/react-query';
import { api } from '../lib/api';
import { missionKeys } from '../lib/queryKeys';
import { Mission, MissionReport } from '../lib/types';

/**
 * Mission list feed — the durable half of the fleet manager's headless
 * interaction model (see docs/development/blueprints/acp/
 * fleet-consolidation.md, "Mission mode"). Polls like useFleet: a mission's
 * LastHeartbeat/LastError are liveness facts an operator watching this list
 * wants current without a manual refresh.
 */
export function useMissions(): UseQueryResult<Mission[], Error> {
  return useQuery<Mission[], Error>({
    queryKey: missionKeys.list(),
    queryFn: () => api.listMissions(),
    refetchInterval: 5000,
  });
}

/**
 * One mission's facts (`GET /api/missions/{id}`), polled for the same
 * liveness reason as {@link useMissions}. Disabled on an empty id so a
 * still-resolving route param never fires a request for `/api/missions/`.
 */
export function useMission(id: string): UseQueryResult<Mission, Error> {
  return useQuery<Mission, Error>({
    queryKey: missionKeys.detail(id),
    queryFn: () => api.getMission(id),
    enabled: !!id,
    refetchInterval: 5000,
  });
}

/**
 * A mission's reports, newest-first (`GET /api/missions/{id}/reports`) — the
 * record of unattended work an operator reads to find out what happened (see
 * missionservice's package doc, "Reports"). Polled so a report filed while
 * the detail page is open appears without a manual refresh.
 */
export function useMissionReports(missionId: string): UseQueryResult<MissionReport[], Error> {
  return useQuery<MissionReport[], Error>({
    queryKey: missionKeys.reports(missionId),
    queryFn: () => api.listMissionReports(missionId),
    enabled: !!missionId,
    refetchInterval: 5000,
  });
}

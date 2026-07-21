import {
  useMutation,
  UseMutationResult,
  useQueries,
  useQuery,
  useQueryClient,
  UseQueryResult,
} from '@tanstack/react-query';
import { api } from '../lib/api';
import { approvalKeys, fleetKeys, missionKeys } from '../lib/queryKeys';
import { HITLApproval, Mission, MissionReport } from '../lib/types';
import {
  joinInboxReports,
  mergeOperatorInboxReports,
  missionsById,
  type InboxReportItem,
} from '../pages/admin/inbox/inboxPresentation';
import { useMissions } from './useMissions';
import { useOperatorInbox } from './useOperatorInbox';

/**
 * Pending-approval feed — the attention inbox's primary half (see
 * docs/development/blueprints/acp/fleet-consolidation.md, slice C2). Polls
 * `GET /api/approvals` on the same short interval as the fleet board: an ask
 * an unattended unit raised is a "a unit needs me" signal an operator watching
 * this page wants current without a manual refresh. The list is pending-only
 * and newest-first server-side, so the hook renders it as received.
 */
export function useApprovals(): UseQueryResult<HITLApproval[], Error> {
  return useQuery<HITLApproval[], Error>({
    queryKey: approvalKeys.list(),
    queryFn: () => api.listApprovals(),
    refetchInterval: 3000,
  });
}

/** Variables for {@link useAnswerApproval}: the ask id and the yes/no answer. */
export type AnswerApprovalInput = { id: string; approved: boolean };

/**
 * Answers a pending ask (`POST /api/approvals/{id}`).
 *
 * Deliberately NOT optimistic, for the same reason the fleet lifecycle hooks
 * are not: the ask's terminal state is decided server-side (a human's answer
 * can race the expiry sweeper — whichever write lands first wins), so the
 * browser has nothing safe to predict. The invalidate is what makes the
 * answered row leave the inbox promptly; on failure the row correctly stays and
 * the caller surfaces `error`.
 *
 * Invalidates the fleet and mission lists too: answering an ask can unblock the
 * unit that raised it, moving it on the board and advancing its mission, so a
 * caller watching either surface sees the consequence without waiting out a
 * poll.
 */
export function useAnswerApproval(): UseMutationResult<
  string,
  Error,
  AnswerApprovalInput,
  unknown
> {
  const queryClient = useQueryClient();
  return useMutation<string, Error, AnswerApprovalInput>({
    mutationFn: ({ id, approved }) => api.answerApproval(id, approved),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: approvalKeys.list() });
      queryClient.invalidateQueries({ queryKey: fleetKeys.list() });
      queryClient.invalidateQueries({ queryKey: missionKeys.list() });
    },
  });
}

/**
 * The inbox's second half: what units reported back, joined across every
 * mission and flattened newest-first. There is no dedicated inbox-reports
 * endpoint yet, so this is a CLIENT-SIDE JOIN — the mission list, plus one
 * `GET /api/missions/{id}/reports` per mission via useQueries — assembled by
 * the pure {@link joinInboxReports}, then enriched with the operator inbox
 * (runtime/operatorinbox — reports that reached NO live supervisor) via
 * {@link mergeOperatorInboxReports}: a report both feeds know about renders
 * once, marked with its operator-inbox provenance.
 *
 * It is structured so a future dedicated endpoint drops straight in: replace
 * the useQueries fan-out with a single `api.getInboxReports()` query and hand
 * its result to the SAME join/consumer shape. The returned `items` and the
 * loading/error aggregates are the contract the page depends on — not the
 * fan-out behind them.
 */
export function useInboxReports(limit?: number): {
  items: InboxReportItem[];
  isLoading: boolean;
  error: Error | null;
} {
  const missionsQuery = useMissions();
  const missions: Mission[] = missionsQuery.data ?? [];

  const reportQueries = useQueries({
    queries: missions.map(m => ({
      queryKey: missionKeys.reports(m.id),
      queryFn: () => api.listMissionReports(m.id),
      refetchInterval: 5000,
    })),
  });

  const reportsByMissionId = new Map<string, MissionReport[]>();
  missions.forEach((m, i) => {
    const data = reportQueries[i]?.data;
    if (data) reportsByMissionId.set(m.id, data);
  });

  const joined = joinInboxReports(missions, reportsByMissionId, limit);

  // The operator inbox is additive enrichment, not a gating fetch: it is
  // nil-gated server-side (404 on an older/unconfigured serve, folded into
  // `null` by useOperatorInbox) and its own loading/error never blocks or
  // fails this feed — a report it does not (yet, or ever) know about still
  // renders exactly as the plain join produced it.
  const operatorInboxQuery = useOperatorInbox();
  const items = mergeOperatorInboxReports(joined, operatorInboxQuery.data, missionsById(missions));

  // Loading only while the mission list itself is still resolving OR any of its
  // per-mission report fetches has not returned its first result — a report
  // feed refetching in the background is not "loading". The error is the
  // mission list's (the join's root); a single mission's report fetch failing
  // degrades to "that mission contributes no rows" rather than failing the feed.
  const isLoading =
    missionsQuery.isLoading ||
    (missions.length > 0 && reportQueries.some(q => q.isLoading));
  const error = missionsQuery.error ?? null;

  return { items, isLoading, error };
}

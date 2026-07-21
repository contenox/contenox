import { useQuery, UseQueryResult } from '@tanstack/react-query';
import { api } from '../lib/api';
import { ApiError } from '../lib/fetch';
import { operatorInboxKeys } from '../lib/queryKeys';
import type { OperatorInboxItem } from '../lib/types';

/**
 * `null` is the deliberate "feature absent" sentinel, the same idiom
 * {@link fetchWorkspaceRoots} uses: `GET /api/operator-inbox` is nil-gated
 * server-side (runtime/serverapi/server.go registers it only when serve
 * builds an operator-inbox service) and 404s on an older or unconfigured
 * serve. Folded here rather than surfaced as an error so the inbox's report
 * feed (see useInboxReports) degrades to "no operator-inbox enrichment" —
 * every report it already knew about still renders — never a red banner.
 */
export type OperatorInboxData = OperatorInboxItem[] | null;

/**
 * Fetches the operator inbox, folding a 404 into the absent sentinel rather
 * than an error. Extracted from the hook so present/404/failure are
 * unit-testable against a mocked `api`, mirroring fetchWorkspaceRoots.
 */
export async function fetchOperatorInbox(): Promise<OperatorInboxData> {
  try {
    return await api.getOperatorInbox();
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null;
    throw err;
  }
}

/**
 * The operator inbox as a live-polled feed (runtime/operatorinbox — reports
 * that reached no live supervisor). Polled on the same short interval as the
 * mission-reports feed it enriches (see useInboxReports): a report landing
 * here is exactly the "what came back overnight" signal an operator watching
 * the inbox wants current. `retry: false` because the one error this fetch
 * expects — 404 — is already folded into the absent sentinel by
 * {@link fetchOperatorInbox}; a genuine failure should surface at once rather
 * than after three silent retries, and (being additive enrichment, not the
 * report feed's primary source) never blocks or fails that feed either.
 */
export function useOperatorInbox(): UseQueryResult<OperatorInboxData, Error> {
  return useQuery<OperatorInboxData, Error>({
    queryKey: operatorInboxKeys.list(),
    queryFn: fetchOperatorInbox,
    refetchInterval: 5000,
    retry: false,
  });
}

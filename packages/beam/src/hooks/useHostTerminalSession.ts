import { useQuery } from '@tanstack/react-query';
import { api } from '../lib/api';
import { ApiError } from '../lib/fetch';
import { terminalKeys } from '../lib/queryKeys';
import type { TerminalSession } from '../lib/types';

/** `null` is the "feature absent" sentinel: the terminal route group 404s when serve did not enable it. */
export type HostTerminalSessionData = TerminalSession | null;

/**
 * Opens a PTY (`POST /api/terminal/sessions`), folding a 404 into the absent
 * sentinel — a serve without the terminal feature is a graceful "unavailable"
 * state, not an error. A 422 (too many sessions, or a cwd outside the roots) is
 * a REAL refusal and propagates so the caller can surface it. Extracted from the
 * hook so the present / 404 / refusal branches are unit-testable.
 */
export async function createHostTerminalSession(
  cwd: string,
  cols: number,
  rows: number,
): Promise<HostTerminalSessionData> {
  try {
    return await api.createTerminalSession({
      ...(cwd ? { cwd } : {}),
      cols,
      rows,
    });
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null;
    throw err;
  }
}

export interface HostTerminalSessionState {
  /** The opened session (id + wsPath), once created. */
  session?: TerminalSession;
  /** True when the terminal feature is absent (404). */
  isAbsent: boolean;
  isLoading: boolean;
  error: Error | null;
  /** Re-run the create (after a transient failure). */
  retry: () => void;
}

export interface UseHostTerminalSessionOptions {
  /** Lazy: only POST once the terminal tab is actually opened. */
  enabled: boolean;
  cwd?: string;
  cols?: number;
  rows?: number;
}

/**
 * A memoized host-terminal session keyed by `ownerKey` (e.g. the mission id).
 *
 * Modeled as a query, not a mutation, precisely so react-query MEMOIZES the
 * PTY-opening POST: with `staleTime`/`gcTime` Infinity, leaving the Terminal tab
 * (unmount) and returning (remount) reuses the SAME cached session — its WS
 * reattaches to the still-live PTY — rather than spawning a fresh shell each
 * time. `enabled` keeps it lazy so merely having a Terminal tab present never
 * opens a shell until the operator selects it. 404 → absent; other failures
 * surface via `error`.
 */
export function useHostTerminalSession(
  ownerKey: string,
  options: UseHostTerminalSessionOptions,
): HostTerminalSessionState {
  const { enabled, cwd = '', cols = 80, rows = 24 } = options;
  const query = useQuery<HostTerminalSessionData, Error>({
    queryKey: terminalKeys.session(ownerKey),
    queryFn: () => createHostTerminalSession(cwd, cols, rows),
    enabled,
    staleTime: Infinity,
    gcTime: Infinity,
    retry: false,
  });

  return {
    session: query.data ?? undefined,
    isAbsent: query.data === null,
    isLoading: query.isLoading && enabled,
    error: query.error ?? null,
    retry: () => void query.refetch(),
  };
}

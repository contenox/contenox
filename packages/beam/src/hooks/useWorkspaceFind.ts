import { useEffect, useState } from 'react';
import {
  runWorkspaceFind,
  type FindFetch,
  type WorkspaceFindMatch,
} from '../lib/workspaceFind';

/**
 * The workspace-filter data source: streams `GET /api/workspace/find` and
 * accumulates the matching file entries. The filename sibling of
 * `useWorkspaceSearch` — same shape (per-run AbortController supersedes the
 * previous stream, a coalescing flush keeps renders bounded, terminal
 * done/refusal/error states) — but keyed on the compiled glob list rather than a
 * text query. The panel owns the debounce of the raw filter value (so typing
 * stays instant); this hook owns the stream.
 *
 * `globs` empty ⇒ idle (no request): the filter is inactive and the panel shows
 * the ordinary lazy tree.
 */

export type FindStatus = 'idle' | 'searching' | 'done' | 'refusal' | 'error';

export interface WorkspaceFindState {
  status: FindStatus;
  entries: WorkspaceFindMatch[];
  count: number;
  truncated: boolean;
  /** The 422 refusal message (bad glob / out-of-bounds root / unreachable path). */
  refusalMessage?: string;
  /** A generic failure message. */
  errorMessage?: string;
}

const IDLE: WorkspaceFindState = {
  status: 'idle',
  entries: [],
  count: 0,
  truncated: false,
};

const FLUSH_MS = 60;

export interface UseWorkspaceFindOptions {
  globs: string[];
  root?: string;
  filter?: string;
  policy?: string;
  limit?: number;
  /** Bump to re-issue the current query (e.g. a retry). */
  nonce?: number;
  /** Injected for tests; forwarded to runWorkspaceFind. */
  fetchImpl?: FindFetch;
}

export function useWorkspaceFind({
  globs,
  root,
  filter,
  policy,
  limit,
  nonce,
  fetchImpl,
}: UseWorkspaceFindOptions): WorkspaceFindState {
  const [state, setState] = useState<WorkspaceFindState>(IDLE);
  // The stream effect keys on the serialized glob list so an unchanged array
  // identity across renders does not re-issue the request.
  const globsKey = globs.join('\n');

  useEffect(() => {
    if (globsKey === '') {
      setState(IDLE);
      return;
    }

    const controller = new AbortController();
    const entries: WorkspaceFindMatch[] = [];
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;

    const flush = () => {
      flushTimer = null;
      if (closed) return;
      setState(prev => ({
        ...prev,
        status: 'searching',
        entries: entries.slice(),
        count: entries.length,
      }));
    };

    setState({ status: 'searching', entries: [], count: 0, truncated: false });

    void runWorkspaceFind({
      globs: globsKey.split('\n'),
      root,
      filter,
      policy,
      limit,
      signal: controller.signal,
      fetchImpl,
      onMatch: m => {
        entries.push(m);
        if (flushTimer === null) flushTimer = setTimeout(flush, FLUSH_MS);
      },
      onDone: done => {
        closed = true;
        if (flushTimer !== null) clearTimeout(flushTimer);
        setState({
          status: 'done',
          entries: entries.slice(),
          count: entries.length,
          truncated: done.truncated,
        });
      },
      onRefusal: refusal => {
        closed = true;
        if (flushTimer !== null) clearTimeout(flushTimer);
        setState({ ...IDLE, status: 'refusal', refusalMessage: refusal.message });
      },
      onError: message => {
        closed = true;
        if (flushTimer !== null) clearTimeout(flushTimer);
        setState({ ...IDLE, status: 'error', errorMessage: message });
      },
    });

    return () => {
      closed = true;
      if (flushTimer !== null) clearTimeout(flushTimer);
      controller.abort();
    };
  }, [globsKey, root, filter, policy, limit, nonce, fetchImpl]);

  return state;
}

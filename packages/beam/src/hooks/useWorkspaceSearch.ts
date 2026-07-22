import { useEffect, useState } from 'react';
import {
  groupMatchesByFile,
  runWorkspaceSearch,
  type SearchFetch,
  type SearchFileGroup,
} from '../lib/workspaceSearch';
import type { WorkspaceSearchMatch } from '../lib/types';

/**
 * The Search tab's state machine over `GET /api/workspace/search` (Arc 2). It
 * owns the 300ms debounce, per-keystroke cancellation (each new query aborts the
 * previous stream), match accumulation grouped by file, and the terminal states
 * the tab renders honestly: `refusal` (422 out-of-bounds root → the boundary
 * notice), `dependency` (501 ripgrep absent → the teaching state), `done` (with
 * `truncated` → the "refine your search" affordance), and `error`. The component
 * owns the input value; this hook owns the search — so the input stays instant
 * (the instant-feel law) while the stream is the async layer behind it.
 */

export type SearchStatus = 'idle' | 'searching' | 'done' | 'refusal' | 'dependency' | 'error';

export interface WorkspaceSearchState {
  status: SearchStatus;
  groups: SearchFileGroup[];
  matchCount: number;
  truncated: boolean;
  /** The 422 refusal message, handed to WorkspaceBoundaryNotice (which decides the boundary vs plain copy). */
  refusalMessage?: string;
  /** A generic (or 501 dependency) failure message. */
  errorMessage?: string;
}

const IDLE: WorkspaceSearchState = {
  status: 'idle',
  groups: [],
  matchCount: 0,
  truncated: false,
};

const DEBOUNCE_MS = 300;
const FLUSH_MS = 60;

export interface UseWorkspaceSearchOptions {
  query: string;
  root?: string;
  limit?: number;
  /** Bump to re-issue the current query (e.g. a retry after a transient error). */
  nonce?: number;
  /** Injected for tests; forwarded to runWorkspaceSearch. */
  fetchImpl?: SearchFetch;
}

export function useWorkspaceSearch({
  query,
  root,
  limit,
  nonce,
  fetchImpl,
}: UseWorkspaceSearchOptions): WorkspaceSearchState {
  const [debounced, setDebounced] = useState('');
  const [state, setState] = useState<WorkspaceSearchState>(IDLE);

  // Debounce the raw query into the value the stream keys on.
  useEffect(() => {
    const trimmed = query.trim();
    if (trimmed === '') {
      setDebounced('');
      return;
    }
    const id = setTimeout(() => setDebounced(trimmed), DEBOUNCE_MS);
    return () => clearTimeout(id);
  }, [query]);

  useEffect(() => {
    if (debounced === '') {
      setState(IDLE);
      return;
    }

    const controller = new AbortController();
    const matches: WorkspaceSearchMatch[] = [];
    let flushTimer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;

    const flush = () => {
      flushTimer = null;
      if (closed) return;
      setState(prev => ({
        ...prev,
        status: 'searching',
        groups: groupMatchesByFile(matches),
        matchCount: matches.length,
      }));
    };

    setState({ status: 'searching', groups: [], matchCount: 0, truncated: false });

    void runWorkspaceSearch({
      query: debounced,
      root,
      limit,
      signal: controller.signal,
      fetchImpl,
      onMatch: m => {
        matches.push(m);
        if (flushTimer === null) flushTimer = setTimeout(flush, FLUSH_MS);
      },
      onDone: done => {
        closed = true;
        if (flushTimer !== null) clearTimeout(flushTimer);
        setState({
          status: 'done',
          groups: groupMatchesByFile(matches),
          matchCount: matches.length,
          truncated: done.truncated,
        });
      },
      onRefusal: refusal => {
        closed = true;
        if (flushTimer !== null) clearTimeout(flushTimer);
        if (refusal.kind === 'dependency') {
          setState({ ...IDLE, status: 'dependency', errorMessage: refusal.message });
        } else {
          setState({ ...IDLE, status: 'refusal', refusalMessage: refusal.message });
        }
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
  }, [debounced, root, limit, nonce, fetchImpl]);

  return state;
}

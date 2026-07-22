import type { WorkspaceAccess } from '../pages/chat/lib/workspaceTree';
import { parseSSEFrames } from './workspaceSearch';

/**
 * The workspace-find transport: the filename sibling of `workspaceSearch.ts`.
 * `GET /api/workspace/find` recursively walks a vfs-validated workspace root and
 * streams each file whose name matches the glob(s) as named SSE events
 * (`event: match` / `event: done`), refusing BEFORE any byte with a 422 (bad
 * glob / out-of-bounds root / unreachable path). One request returns every match
 * across the tree — the server-side replacement for a per-directory client walk.
 *
 * WHY fetch + ReadableStream (not `EventSource`): identical to search — the 422
 * refusal arrives as a non-2xx response whose JSON body carries the reason, which
 * `EventSource` cannot surface. The SSE frame parsing is shared with search
 * (`parseSSEFrames`) so both stay byte-for-byte consistent and unit-testable
 * against a mocked fetch with no DOM.
 */

/** One `event: match` frame: a matching file entry (mirrors Go `localfileapi.Entry`). */
export interface WorkspaceFindMatch {
  path: string;
  name: string;
  isDirectory: boolean;
  size?: number;
  /** Present only when the request carried `filter=agent`. */
  access?: WorkspaceAccess;
}

/** The single terminal `event: done` frame (mirrors Go `localfileapi.findDone`). */
export interface WorkspaceFindDone {
  done: boolean;
  matches: number;
  truncated: boolean;
}

/** A pre-stream HTTP refusal (422: bad glob / out-of-bounds root / unreachable path). */
export type FindRefusal = { status: number; code?: string; message: string };

export type FindFetch = (
  url: string,
  init: { signal?: AbortSignal; headers: Record<string, string>; credentials: 'same-origin' },
) => Promise<FindResponseLike>;

/** The minimal shape of a fetch `Response` this consumer reads — satisfied by the real `Response`. */
export interface FindResponseLike {
  ok: boolean;
  status: number;
  json(): Promise<unknown>;
  body: {
    getReader(): {
      read(): Promise<{ done: boolean; value?: Uint8Array }>;
    };
  } | null;
}

export interface RunWorkspaceFindOptions {
  /** filepath.Match patterns; a file matches if ANY matches. Empty is a no-op (the caller guards). */
  globs: string[];
  root?: string;
  /** Subtree to search under, relative to the root; defaults to the whole workspace. */
  path?: string;
  /** "agent" annotates each match with the per-path access verdict; omit for raw entries. */
  filter?: string;
  policy?: string;
  limit?: number;
  signal?: AbortSignal;
  /** Injected for tests; defaults to same-origin `fetch`. */
  fetchImpl?: FindFetch;
  onMatch: (match: WorkspaceFindMatch) => void;
  onDone: (done: WorkspaceFindDone) => void;
  onRefusal: (refusal: FindRefusal) => void;
  onError: (message: string) => void;
}

/** Root-relative so it stays same-origin behind the dev proxy, mirroring `buildSearchUrl`. */
export function buildFindUrl(opts: RunWorkspaceFindOptions): string {
  const params = new URLSearchParams({ glob: opts.globs.join(',') });
  if (opts.root) params.set('root', opts.root);
  if (opts.path && opts.path !== '.') params.set('path', opts.path);
  if (opts.filter) params.set('filter', opts.filter);
  if (opts.policy) params.set('policy', opts.policy);
  if (opts.limit !== undefined) params.set('limit', String(opts.limit));
  return `/api/workspace/find?${params.toString()}`;
}

function messageFromErrorBody(body: unknown): { message?: string; code?: string } {
  if (typeof body === 'object' && body !== null && 'error' in body) {
    const err = (body as { error: unknown }).error;
    if (typeof err === 'object' && err !== null) {
      const e = err as { message?: unknown; code?: unknown };
      return {
        message: typeof e.message === 'string' ? e.message : undefined,
        code: typeof e.code === 'string' ? e.code : undefined,
      };
    }
  }
  return {};
}

const defaultFetch: FindFetch = (url, init) =>
  fetch(url, init) as unknown as Promise<FindResponseLike>;

/**
 * Run one find, streaming matches to the callbacks until `done` or an abort. A
 * non-2xx response is classified into a 422 refusal or a generic error before any
 * frame is read; an aborted stream resolves quietly (a newer keystroke superseded
 * this one).
 */
export async function runWorkspaceFind(opts: RunWorkspaceFindOptions): Promise<void> {
  const fetchImpl = opts.fetchImpl ?? defaultFetch;
  const url = buildFindUrl(opts);

  let response: FindResponseLike;
  try {
    response = await fetchImpl(url, {
      signal: opts.signal,
      credentials: 'same-origin',
      headers: { Accept: 'text/event-stream' },
    });
  } catch (err) {
    if (isAbort(err)) return;
    opts.onError(err instanceof Error ? err.message : String(err));
    return;
  }

  if (!response.ok) {
    let parsed: { message?: string; code?: string } = {};
    try {
      parsed = messageFromErrorBody(await response.json());
    } catch {
      /* non-JSON error body — fall back to a status-derived message */
    }
    const message = parsed.message ?? `find failed (${response.status})`;
    if (response.status === 422) {
      opts.onRefusal({ status: 422, code: parsed.code, message });
    } else {
      opts.onError(message);
    }
    return;
  }

  if (!response.body) {
    opts.onDone({ done: true, matches: 0, truncated: false });
    return;
  }

  const reader = response.body.getReader();
  const streamDecoder = new TextDecoder();
  let buffer = '';
  try {
    for (;;) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += streamDecoder.decode(value, { stream: true });
      const { frames, rest } = parseSSEFrames(buffer);
      buffer = rest;
      for (const frame of frames) {
        if (frame.event === 'match') {
          const m = safeParse<WorkspaceFindMatch>(frame.data);
          if (m) opts.onMatch(m);
        } else if (frame.event === 'done') {
          const d = safeParse<WorkspaceFindDone>(frame.data);
          opts.onDone(d ?? { done: true, matches: 0, truncated: false });
          return;
        }
      }
    }
  } catch (err) {
    if (isAbort(err)) return;
    opts.onError(err instanceof Error ? err.message : String(err));
  }
}

function safeParse<T>(data: string): T | null {
  try {
    return JSON.parse(data) as T;
  } catch {
    return null;
  }
}

function isAbort(err: unknown): boolean {
  return err instanceof DOMException ? err.name === 'AbortError' : false;
}

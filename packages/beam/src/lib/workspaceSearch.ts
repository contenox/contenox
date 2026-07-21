import type { WorkspaceSearchDone, WorkspaceSearchMatch } from './types';

/**
 * The workspace-search transport + pure presentation helpers (Arc 2 of
 * ide-workflows.md): `GET /api/workspace/search` streams matches as named SSE
 * events (`event: match` / `event: done`), refusing BEFORE any byte with a 422
 * (bad query / out-of-bounds root) or a 501 `dependency_missing` (ripgrep
 * absent).
 *
 * WHY fetch + ReadableStream and not `EventSource`: the two refusals arrive as
 * ordinary non-2xx HTTP responses whose JSON body carries the reason, and
 * `EventSource` exposes neither the status code nor the body on error — it can
 * only report an opaque `onerror`. Surfacing the ripgrep teaching state and the
 * boundary notice (a hard requirement of the Search tab) is therefore
 * impossible over `EventSource`; a fetch stream reads the status and body, and
 * an `AbortController` gives the same per-keystroke cancellation. The SSE frame
 * parsing and byte-offset slicing are factored into pure helpers so the stream
 * consumption is unit-testable against a mocked fetch, with no DOM.
 */

// ── Byte-offset highlighting ────────────────────────────────────────────────

const encoder = new TextEncoder();
const decoder = new TextDecoder();

/**
 * Split a match preview into `{ before, match, after }` at the server's BYTE
 * offsets (`column`, `length`) — the match's start and byte length within the
 * UTF-8 encoding of `preview`. JS strings are UTF-16, so a naive `slice` would
 * mis-highlight any line with a multi-byte character before the match; this
 * encodes to bytes, slices, and decodes each part. Offsets are clamped so a
 * malformed frame can never throw or index out of range.
 */
export function byteSlice(
  preview: string,
  column: number,
  length: number,
): { before: string; match: string; after: string } {
  const bytes = encoder.encode(preview);
  const start = Math.max(0, Math.min(column, bytes.length));
  const end = Math.max(start, Math.min(start + Math.max(0, length), bytes.length));
  return {
    before: decoder.decode(bytes.subarray(0, start)),
    match: decoder.decode(bytes.subarray(start, end)),
    after: decoder.decode(bytes.subarray(end)),
  };
}

// ── Per-file grouping ───────────────────────────────────────────────────────

export type SearchFileGroup = {
  path: string;
  matches: WorkspaceSearchMatch[];
};

/**
 * Cluster matches by file, preserving first-seen file order and per-file arrival
 * order (rg streams a file's matches contiguously and in file order, so this is
 * stable without a sort). One group per path with its own count badge.
 */
export function groupMatchesByFile(matches: readonly WorkspaceSearchMatch[]): SearchFileGroup[] {
  const groups: SearchFileGroup[] = [];
  const byPath = new Map<string, SearchFileGroup>();
  for (const m of matches) {
    let group = byPath.get(m.path);
    if (!group) {
      group = { path: m.path, matches: [] };
      byPath.set(m.path, group);
      groups.push(group);
    }
    group.matches.push(m);
  }
  return groups;
}

// ── SSE frame parsing ───────────────────────────────────────────────────────

export type SSEFrame = { event: string; data: string };

/**
 * Parse whatever complete SSE frames are in `buffer`, returning them plus the
 * trailing incomplete text (`rest`) to prepend to the next chunk. Frames are
 * separated by a blank line; within a frame, `event:` names the event and one or
 * more `data:` lines are joined with newlines (the SSE spec). A frame with no
 * `event:` defaults to `message`, matching `EventSource`.
 */
export function parseSSEFrames(buffer: string): { frames: SSEFrame[]; rest: string } {
  const normalized = buffer.replace(/\r\n/g, '\n');
  const segments = normalized.split('\n\n');
  const rest = segments.pop() ?? '';
  const frames: SSEFrame[] = [];
  for (const segment of segments) {
    if (segment.trim() === '') continue;
    let event = 'message';
    const dataLines: string[] = [];
    for (const line of segment.split('\n')) {
      if (line.startsWith('event:')) event = line.slice(6).trim();
      else if (line.startsWith('data:')) dataLines.push(line.slice(5).replace(/^ /, ''));
    }
    frames.push({ event, data: dataLines.join('\n') });
  }
  return { frames, rest };
}

// ── The streaming consumer ──────────────────────────────────────────────────

/** A pre-stream HTTP refusal: `dependency` (501, ripgrep absent) or `refusal` (422, bad query / out-of-bounds root). */
export type SearchRefusal = {
  kind: 'dependency' | 'refusal';
  status: number;
  code?: string;
  message: string;
};

export type SearchFetch = (
  url: string,
  init: { signal?: AbortSignal; headers: Record<string, string>; credentials: 'same-origin' },
) => Promise<SearchResponseLike>;

/** The minimal shape of a fetch `Response` this consumer reads — satisfied by the real `Response`. */
export interface SearchResponseLike {
  ok: boolean;
  status: number;
  json(): Promise<unknown>;
  body: {
    getReader(): {
      read(): Promise<{ done: boolean; value?: Uint8Array }>;
    };
  } | null;
}

export interface RunWorkspaceSearchOptions {
  query: string;
  root?: string;
  limit?: number;
  signal?: AbortSignal;
  /** Injected for tests; defaults to same-origin `fetch`. */
  fetchImpl?: SearchFetch;
  onMatch: (match: WorkspaceSearchMatch) => void;
  onDone: (done: WorkspaceSearchDone) => void;
  onRefusal: (refusal: SearchRefusal) => void;
  onError: (message: string) => void;
}

/** Root-relative so it stays same-origin behind the dev proxy, mirroring `api.taskEvents`. */
export function buildSearchUrl(query: string, root?: string, limit?: number): string {
  const params = new URLSearchParams({ q: query });
  if (root) params.set('root', root);
  if (limit !== undefined) params.set('limit', String(limit));
  return `/api/workspace/search?${params.toString()}`;
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

const defaultFetch: SearchFetch = (url, init) =>
  fetch(url, init) as unknown as Promise<SearchResponseLike>;

/**
 * Run one search, streaming matches to the callbacks until `done` or an abort.
 * A non-2xx response is classified into a refusal (501 dependency / 422) or a
 * generic error before any frame is read; an aborted stream resolves quietly
 * (the caller intended it, e.g. a newer keystroke superseded this one).
 */
export async function runWorkspaceSearch(opts: RunWorkspaceSearchOptions): Promise<void> {
  const fetchImpl = opts.fetchImpl ?? defaultFetch;
  const url = buildSearchUrl(opts.query, opts.root, opts.limit);

  let response: SearchResponseLike;
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
    const message = parsed.message ?? `search failed (${response.status})`;
    if (response.status === 501 && parsed.code === 'dependency_missing') {
      opts.onRefusal({ kind: 'dependency', status: 501, code: parsed.code, message });
    } else if (response.status === 422) {
      opts.onRefusal({ kind: 'refusal', status: 422, code: parsed.code, message });
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
          const m = safeParse<WorkspaceSearchMatch>(frame.data);
          if (m) opts.onMatch(m);
        } else if (frame.event === 'done') {
          const d = safeParse<WorkspaceSearchDone>(frame.data);
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

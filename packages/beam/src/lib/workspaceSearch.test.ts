import { describe, expect, it, vi } from 'vitest';
import {
  buildSearchUrl,
  byteSlice,
  groupMatchesByFile,
  parseSSEFrames,
  runWorkspaceSearch,
  type SearchResponseLike,
} from './workspaceSearch';
import type { WorkspaceSearchDone, WorkspaceSearchMatch } from './types';

describe('byteSlice — byte-offset highlighting', () => {
  it('splits an ASCII preview at the match', () => {
    expect(byteSlice('hello world', 6, 5)).toEqual({ before: 'hello ', match: 'world', after: '' });
  });

  it('aligns to BYTE offsets across a multi-byte character before the match', () => {
    // "héllo " is 7 bytes (é = 2), so "world" starts at byte 7.
    const { before, match, after } = byteSlice('héllo world', 7, 5);
    expect(before).toBe('héllo ');
    expect(match).toBe('world');
    expect(after).toBe('');
  });

  it('clamps out-of-range offsets rather than throwing', () => {
    expect(byteSlice('abc', 10, 5)).toEqual({ before: 'abc', match: '', after: '' });
    expect(byteSlice('abc', 1, 100)).toEqual({ before: 'a', match: 'bc', after: '' });
  });
});

describe('parseSSEFrames', () => {
  it('parses complete named frames and returns the trailing incomplete text', () => {
    const { frames, rest } = parseSSEFrames(
      'event: match\ndata: {"a":1}\n\nevent: done\ndata: {"done":true}\n\nevent: ma',
    );
    expect(frames).toEqual([
      { event: 'match', data: '{"a":1}' },
      { event: 'done', data: '{"done":true}' },
    ]);
    expect(rest).toBe('event: ma');
  });

  it('resumes across a chunk boundary when the leftover is prepended', () => {
    const first = parseSSEFrames('event: match\ndata: {"a":1}\n\nevent: don');
    expect(first.frames).toHaveLength(1);
    const second = parseSSEFrames(first.rest + 'e\ndata: {"done":true}\n\n');
    expect(second.frames).toEqual([{ event: 'done', data: '{"done":true}' }]);
    expect(second.rest).toBe('');
  });
});

describe('groupMatchesByFile', () => {
  it('clusters by path preserving first-seen file order and per-file arrival order', () => {
    const matches: WorkspaceSearchMatch[] = [
      { path: 'a.txt', line: 1, column: 0, length: 1, preview: 'x' },
      { path: 'b.txt', line: 5, column: 0, length: 1, preview: 'y' },
      { path: 'a.txt', line: 9, column: 0, length: 1, preview: 'z' },
    ];
    const groups = groupMatchesByFile(matches);
    expect(groups.map(g => g.path)).toEqual(['a.txt', 'b.txt']);
    expect(groups[0].matches.map(m => m.line)).toEqual([1, 9]);
  });
});

describe('buildSearchUrl', () => {
  it('includes root and limit only when provided, and stays root-relative', () => {
    expect(buildSearchUrl('foo bar')).toBe('/api/workspace/search?q=foo+bar');
    expect(buildSearchUrl('foo', '/repo', 500)).toBe(
      '/api/workspace/search?q=foo&root=%2Frepo&limit=500',
    );
  });
});

// ── runWorkspaceSearch against a mocked fetch stream ─────────────────────────

function streamResponse(chunks: string[]): SearchResponseLike {
  const enc = new TextEncoder();
  let i = 0;
  return {
    ok: true,
    status: 200,
    json: async () => ({}),
    body: {
      getReader: () => ({
        read: async () =>
          i < chunks.length
            ? { done: false, value: enc.encode(chunks[i++]) }
            : { done: true, value: undefined },
      }),
    },
  };
}

function errorResponse(status: number, body: unknown): SearchResponseLike {
  return { ok: false, status, json: async () => body, body: null };
}

describe('runWorkspaceSearch', () => {
  it('streams matches then the done frame, reassembling across chunk boundaries', async () => {
    const matches: WorkspaceSearchMatch[] = [];
    let done: WorkspaceSearchDone | undefined;
    await runWorkspaceSearch({
      query: 'hello',
      fetchImpl: async () =>
        streamResponse([
          'event: match\ndata: {"path":"a.txt","line":1,"column":0,"length":5,"preview":"hello"}\n\nev',
          'ent: done\ndata: {"done":true,"matches":1,"truncated":false}\n\n',
        ]),
      onMatch: m => matches.push(m),
      onDone: d => (done = d),
      onRefusal: () => expect.fail('should not refuse'),
      onError: () => expect.fail('should not error'),
    });
    expect(matches).toEqual([
      { path: 'a.txt', line: 1, column: 0, length: 5, preview: 'hello' },
    ]);
    expect(done).toEqual({ done: true, matches: 1, truncated: false });
  });

  it('classifies a 501 dependency_missing as the ripgrep teaching refusal', async () => {
    const refusal = vi.fn();
    await runWorkspaceSearch({
      query: 'x',
      fetchImpl: async () =>
        errorResponse(501, { error: { message: 'needs ripgrep', code: 'dependency_missing' } }),
      onMatch: () => {},
      onDone: () => {},
      onRefusal: refusal,
      onError: () => expect.fail('should be a refusal, not an error'),
    });
    expect(refusal).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'dependency', status: 501 }),
    );
  });

  it('classifies a 422 as a boundary refusal carrying the wire message', async () => {
    const refusal = vi.fn();
    await runWorkspaceSearch({
      query: 'x',
      root: '/nope',
      fetchImpl: async () =>
        errorResponse(422, { error: { message: 'workspace root "/nope" is not permitted' } }),
      onMatch: () => {},
      onDone: () => {},
      onRefusal: refusal,
      onError: () => expect.fail('should be a refusal, not an error'),
    });
    expect(refusal).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'refusal', status: 422, message: expect.stringContaining('not permitted') }),
    );
  });

  it('reports any other non-2xx as a generic error', async () => {
    const onError = vi.fn();
    await runWorkspaceSearch({
      query: 'x',
      fetchImpl: async () => errorResponse(500, { error: { message: 'boom' } }),
      onMatch: () => {},
      onDone: () => {},
      onRefusal: () => expect.fail('500 is not a refusal'),
      onError,
    });
    expect(onError).toHaveBeenCalledWith('boom');
  });

  it('resolves silently on abort, firing no callbacks', async () => {
    const onError = vi.fn();
    const onDone = vi.fn();
    await runWorkspaceSearch({
      query: 'x',
      fetchImpl: async () => {
        throw new DOMException('aborted', 'AbortError');
      },
      onMatch: () => {},
      onDone,
      onRefusal: () => {},
      onError,
    });
    expect(onError).not.toHaveBeenCalled();
    expect(onDone).not.toHaveBeenCalled();
  });
});

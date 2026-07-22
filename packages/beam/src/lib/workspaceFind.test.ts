import { describe, expect, it } from 'vitest';
import {
  buildFindUrl,
  runWorkspaceFind,
  type FindFetch,
  type FindResponseLike,
  type WorkspaceFindDone,
  type WorkspaceFindMatch,
} from './workspaceFind';

/** A fake streaming Response that yields the given SSE text chunks in order. */
function streamResponse(chunks: string[]): FindResponseLike {
  const enc = new TextEncoder();
  let i = 0;
  return {
    ok: true,
    status: 200,
    json: async () => ({}),
    body: {
      getReader() {
        return {
          read: async () =>
            i < chunks.length ? { done: false, value: enc.encode(chunks[i++]) } : { done: true },
        };
      },
    },
  };
}

const noop = () => {};

describe('buildFindUrl', () => {
  it('joins globs and carries root/filter/path/limit', () => {
    const url = buildFindUrl({
      globs: ['*.md', '*.ts'],
      root: '/repo',
      filter: 'agent',
      path: 'docs',
      limit: 500,
      onMatch: noop,
      onDone: noop,
      onRefusal: noop,
      onError: noop,
    });
    expect(url.startsWith('/api/workspace/find?')).toBe(true);
    const q = new URLSearchParams(url.split('?')[1]);
    expect(q.get('glob')).toBe('*.md,*.ts');
    expect(q.get('root')).toBe('/repo');
    expect(q.get('filter')).toBe('agent');
    expect(q.get('path')).toBe('docs');
    expect(q.get('limit')).toBe('500');
  });

  it('omits path when it is the default "."', () => {
    const url = buildFindUrl({ globs: ['*.md'], path: '.', onMatch: noop, onDone: noop, onRefusal: noop, onError: noop });
    expect(new URLSearchParams(url.split('?')[1]).has('path')).toBe(false);
  });
});

describe('runWorkspaceFind', () => {
  it('streams match frames then the terminal done', async () => {
    const fetchImpl: FindFetch = async () =>
      streamResponse([
        'event: match\ndata: {"path":"README.md","name":"README.md","isDirectory":false}\n\n',
        'event: match\ndata: {"path":"docs/intro.md","name":"intro.md","isDirectory":false}\n\n',
        'event: done\ndata: {"done":true,"matches":2,"truncated":false}\n\n',
      ]);
    const matches: WorkspaceFindMatch[] = [];
    let done: WorkspaceFindDone | undefined;
    await runWorkspaceFind({
      globs: ['*.md'],
      fetchImpl,
      onMatch: m => matches.push(m),
      onDone: d => (done = d),
      onRefusal: noop,
      onError: noop,
    });
    expect(matches.map(m => m.path)).toEqual(['README.md', 'docs/intro.md']);
    expect(done?.truncated).toBe(false);
    expect(done?.matches).toBe(2);
  });

  it('splits a frame straddling two chunks', async () => {
    const fetchImpl: FindFetch = async () =>
      streamResponse([
        'event: match\ndata: {"path":"a.md","nam',
        'e":"a.md","isDirectory":false}\n\nevent: done\ndata: {"done":true,"matches":1,"truncated":false}\n\n',
      ]);
    const matches: WorkspaceFindMatch[] = [];
    await runWorkspaceFind({ globs: ['*.md'], fetchImpl, onMatch: m => matches.push(m), onDone: noop, onRefusal: noop, onError: noop });
    expect(matches.map(m => m.path)).toEqual(['a.md']);
  });

  it('classifies a 422 as a refusal before reading any frame', async () => {
    const fetchImpl: FindFetch = async () => ({
      ok: false,
      status: 422,
      json: async () => ({ error: { message: 'glob is required' } }),
      body: null,
    });
    let refusalMessage = '';
    let refusalStatus = 0;
    await runWorkspaceFind({
      globs: [],
      fetchImpl,
      onMatch: noop,
      onDone: noop,
      onRefusal: r => {
        refusalMessage = r.message;
        refusalStatus = r.status;
      },
      onError: noop,
    });
    expect(refusalStatus).toBe(422);
    expect(refusalMessage).toContain('glob');
  });

  it('reports a non-422 failure as a generic error', async () => {
    const fetchImpl: FindFetch = async () => ({
      ok: false,
      status: 500,
      json: async () => ({ error: { message: 'boom' } }),
      body: null,
    });
    let error = '';
    await runWorkspaceFind({ globs: ['*.md'], fetchImpl, onMatch: noop, onDone: noop, onRefusal: noop, onError: m => (error = m) });
    expect(error).toBe('boom');
  });
});

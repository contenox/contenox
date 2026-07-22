import { describe, expect, it } from 'vitest';
import type { WorkspaceFindMatch } from '../../../lib/workspaceFind';
import {
  availableFilterTypes,
  buildTreeFromMatches,
  filterTypeById,
  WORKSPACE_FILTER_TYPES,
  type FindQuery,
} from './workspaceFilter';

/** Compiles a type's value, asserting it isn't the "inactive" null. */
function query(id: string, value: string): FindQuery {
  const q = filterTypeById(id)!.toQuery(value);
  if (!q) throw new Error(`expected a query for ${id}=${value}`);
  return q;
}

const match = (path: string, access?: WorkspaceFindMatch['access']): WorkspaceFindMatch => ({
  path,
  name: path.split('/').pop()!,
  isDirectory: false,
  ...(access ? { access } : {}),
});

describe('ext filter type', () => {
  it('compiles each extension into a *.<ext> glob', () => {
    expect(query('ext', 'md').globs).toEqual(['*.md']);
    expect(query('ext', 'md, ts').globs).toEqual(['*.md', '*.ts']);
  });

  it('accepts `.md` and `*.md` forms', () => {
    expect(query('ext', '.md').globs).toEqual(['*.md']);
    expect(query('ext', '*.md').globs).toEqual(['*.md']);
  });

  it('is inactive (null) for an empty value', () => {
    expect(filterTypeById('ext')!.toQuery('   ')).toBeNull();
  });
});

describe('glob filter type', () => {
  it('passes patterns through, comma/space separated', () => {
    expect(query('glob', '*.md').globs).toEqual(['*.md']);
    expect(query('glob', '*.md, test_*').globs).toEqual(['*.md', 'test_*']);
  });
});

describe('name filter type', () => {
  it('compiles to a basename substring glob', () => {
    expect(query('name', 'foo').globs).toEqual(['*foo*']);
  });
});

describe('access filter type', () => {
  it('is only offered under the agent view', () => {
    expect(availableFilterTypes({ agentView: false }).some(t => t.id === 'access')).toBe(false);
    expect(availableFilterTypes({ agentView: true }).some(t => t.id === 'access')).toBe(true);
  });

  it('walks everything (*) and refines by the worst read/write verdict', () => {
    const q = query('access', 'approve');
    expect(q.globs).toEqual(['*']);
    expect(q.refine!(match('secret.env', { reachable: true, read: 'allow', write: 'approve' }))).toBe(true);
    expect(q.refine!(match('ok.txt', { reachable: true, read: 'allow', write: 'allow' }))).toBe(false);
    expect(q.refine!(match('plain.txt'))).toBe(false); // no verdict → excluded
  });
});

describe('buildTreeFromMatches', () => {
  const labels = { unreachable: 'Outside', read: 'Read', write: 'Write' };

  it('assembles flat file matches into a tree, synthesizing ancestor dirs, dirs-first', () => {
    const nodes = buildTreeFromMatches([
      match('README.md'),
      match('docs/intro.md'),
      match('docs/beam/guide.md'),
    ]);
    // docs (dir) sorts before README.md (file).
    expect(nodes.map(n => n.id)).toEqual(['docs', 'README.md']);
    const docs = nodes.find(n => n.id === 'docs')!;
    expect(docs.isDirectory).toBe(true);
    expect(docs.children!.map(n => n.id)).toEqual(['docs/beam', 'docs/intro.md']); // dir before file
    const beam = docs.children!.find(n => n.id === 'docs/beam')!;
    expect(beam.children!.map(n => n.id)).toEqual(['docs/beam/guide.md']);
  });

  it('threads the agent-view status + tooltip onto file leaves from their verdict', () => {
    const nodes = buildTreeFromMatches(
      [match('secret.env', { reachable: true, read: 'allow', write: 'deny', writeReason: 'ro' })],
      labels,
    );
    const file = nodes.find(n => n.id === 'secret.env')!;
    expect(file.status).toBe('deny');
    expect(file.title).toBe('Write: deny (ro)');
  });

  it('leaves status/title absent for a raw match (no verdict)', () => {
    const file = buildTreeFromMatches([match('a.ts')])[0];
    expect(file.status).toBeUndefined();
    expect(file.title).toBeUndefined();
  });

  it('returns [] for no matches', () => {
    expect(buildTreeFromMatches([])).toEqual([]);
  });
});

describe('registry invariants', () => {
  it('has unique, stable ids', () => {
    const ids = WORKSPACE_FILTER_TYPES.map(t => t.id);
    expect(new Set(ids).size).toBe(ids.length);
  });
});

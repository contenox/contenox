import { describe, expect, it } from 'vitest';
import type { PathVerdict } from '../../../lib/workspaceAccess';
import type { WorkspaceFindMatch } from '../../../lib/workspaceFind';
import {
  availableFilterTypes,
  axisMatches,
  buildTreeFromMatches,
  filterTypeById,
  WORKSPACE_FILTER_TYPES,
  type FindQuery,
} from './workspaceFilter';
import type { NodeDecoration, NodeDecorator } from './workspaceTree';

/** Compiles a type's value, asserting it isn't the "inactive" null. */
function query(id: string, value: string): FindQuery {
  const q = filterTypeById(id)!.toQuery(value);
  if (!q) throw new Error(`expected a query for ${id}=${value}`);
  return q;
}

const match = (path: string): WorkspaceFindMatch => ({
  path,
  name: path.split('/').pop()!,
  isDirectory: false,
});

const verdict = (
  read: PathVerdict['read'],
  write: PathVerdict['write'],
  reachable = true,
): PathVerdict => ({ path: 'x', reachable, read, write });

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

describe('axisMatches', () => {
  it('matches a reachable path on the chosen dimension only', () => {
    const v = verdict({ action: 'allow' }, { action: 'deny' });
    expect(axisMatches(v, 'read', 'allow')).toBe(true);
    expect(axisMatches(v, 'read', 'deny')).toBe(false);
    expect(axisMatches(v, 'write', 'deny')).toBe(true);
    expect(axisMatches(v, 'write', 'allow')).toBe(false);
  });

  it('matches unreachable independent of dimension, and never a reachable path', () => {
    const unreachable = verdict(undefined, undefined, false);
    expect(axisMatches(unreachable, 'read', 'unreachable')).toBe(true);
    expect(axisMatches(unreachable, 'write', 'unreachable')).toBe(true);
    expect(axisMatches(verdict({ action: 'allow' }, { action: 'allow' }), 'read', 'unreachable')).toBe(false);
  });

  it('excludes a path with no verdict yet (not evaluated)', () => {
    expect(axisMatches(undefined, 'read', 'allow')).toBe(false);
  });
});

describe('access axis filter types', () => {
  it('offers both read and write axes only under the agent view', () => {
    const off = availableFilterTypes({ agentView: false }).map(t => t.id);
    const on = availableFilterTypes({ agentView: true }).map(t => t.id);
    expect(off).not.toContain('access_read');
    expect(off).not.toContain('access_write');
    expect(on).toContain('access_read');
    expect(on).toContain('access_write');
  });

  it('read axis walks everything (*) and refines on the READ verdict', () => {
    const q = query('access_read', 'deny');
    expect(q.globs).toEqual(['*']);
    expect(q.refine!(match('secret'), verdict({ action: 'deny' }, { action: 'deny' }))).toBe(true);
    expect(q.refine!(match('ok'), verdict({ action: 'allow' }, { action: 'deny' }))).toBe(false); // write deny, read allow → excluded
    expect(q.refine!(match('none'), undefined)).toBe(false);
  });

  it('write axis refines on the WRITE verdict, independent of read', () => {
    const q = query('access_write', 'approve');
    expect(q.refine!(match('env'), verdict({ action: 'allow' }, { action: 'approve' }))).toBe(true);
    expect(q.refine!(match('ro'), verdict({ action: 'allow' }, { action: 'allow' }))).toBe(false);
  });

  it('is inactive (null) for an empty value', () => {
    expect(filterTypeById('access_read')!.toQuery('  ')).toBeNull();
    expect(filterTypeById('access_write')!.toQuery('')).toBeNull();
  });
});

describe('buildTreeFromMatches', () => {
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

  it('threads the agent-view decorator onto file leaves (not the synthesized dirs)', () => {
    const decorate: NodeDecorator = path => {
      const table: Record<string, NodeDecoration> = {
        'secret.env': { dimmed: false, indicators: [{ key: 'write', icon: null, status: 'deny', title: 'Write: blocked' }] },
      };
      return table[path];
    };
    const nodes = buildTreeFromMatches([match('secret.env')], decorate);
    const file = nodes.find(n => n.id === 'secret.env')!;
    expect(file.indicators?.[0].status).toBe('deny');
    expect(file.indicators?.[0].title).toBe('Write: blocked');
  });

  it('leaves indicators absent for a raw match (no decorator)', () => {
    const file = buildTreeFromMatches([match('a.ts')])[0];
    expect(file.indicators).toBeUndefined();
    expect(file.dimmed).toBeUndefined();
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

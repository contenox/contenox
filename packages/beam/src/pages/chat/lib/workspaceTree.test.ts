import { describe, expect, it } from 'vitest';
import {
  accessToStatus,
  accessTooltip,
  flattenFiles,
  ROOT_DIR,
  toFileTreeNodes,
  type AccessLabels,
  type DirCache,
  type WorkspaceAccess,
} from './workspaceTree';

const LABELS: AccessLabels = { unreachable: 'Outside', read: 'Read', write: 'Write' };

const cache: DirCache = {
  [ROOT_DIR]: [
    { path: 'src', name: 'src', isDirectory: true },
    { path: 'README.md', name: 'README.md', isDirectory: false },
  ],
  src: [{ path: 'src/app.ts', name: 'app.ts', isDirectory: false }],
};

describe('toFileTreeNodes', () => {
  it('builds root nodes with loaded children and undefined for unloaded dirs', () => {
    const partial: DirCache = { [ROOT_DIR]: cache[ROOT_DIR] };
    const nodes = toFileTreeNodes(partial);
    const dir = nodes.find(n => n.id === 'src')!;
    expect(dir.isDirectory).toBe(true);
    expect(dir.children).toBeUndefined(); // not loaded yet
  });

  it('populates children once the directory is loaded', () => {
    const nodes = toFileTreeNodes(cache);
    const dir = nodes.find(n => n.id === 'src')!;
    expect(dir.children).toEqual([
      { id: 'src/app.ts', name: 'app.ts', path: 'src/app.ts', isDirectory: false, children: undefined },
    ]);
  });

  it('returns [] for an unloaded directory path', () => {
    expect(toFileTreeNodes({}, 'nope')).toEqual([]);
  });
});

describe('accessToStatus', () => {
  it('maps unreachable first, before any read/write', () => {
    expect(accessToStatus({ reachable: false })).toBe('unreachable');
    expect(accessToStatus({ reachable: false, read: 'allow', write: 'allow' })).toBe('unreachable');
  });

  it('takes the worst of read/write (deny > approve > allow)', () => {
    expect(accessToStatus({ reachable: true, read: 'allow', write: 'deny' })).toBe('deny');
    expect(accessToStatus({ reachable: true, read: 'approve', write: 'allow' })).toBe('approve');
    expect(accessToStatus({ reachable: true, read: 'allow', write: 'allow' })).toBe('allow');
    expect(accessToStatus({ reachable: true })).toBe('allow');
  });
});

describe('accessTooltip', () => {
  it('returns the boundary label when unreachable', () => {
    expect(accessTooltip({ reachable: false }, LABELS)).toBe('Outside');
  });

  it('summarizes non-allow read/write verdicts with reasons', () => {
    const access: WorkspaceAccess = {
      reachable: true,
      read: 'allow',
      write: 'approve',
      writeReason: 'glob:secrets/*',
    };
    expect(accessTooltip(access, LABELS)).toBe('Write: approve (glob:secrets/*)');
  });

  it('is undefined when everything is a boring allow', () => {
    expect(accessTooltip({ reachable: true, read: 'allow', write: 'allow' }, LABELS)).toBeUndefined();
  });
});

describe('toFileTreeNodes access threading', () => {
  const denyAccess: WorkspaceAccess = { reachable: true, read: 'allow', write: 'deny', writeReason: 'ro' };
  const accessCache: DirCache = {
    [ROOT_DIR]: [
      { path: 'src', name: 'src', isDirectory: true, access: { reachable: true, read: 'allow', write: 'allow' } },
      { path: 'secret', name: 'secret', isDirectory: false, access: denyAccess },
      { path: '../escape', name: 'escape', isDirectory: false, access: { reachable: false } },
    ],
  };

  it('threads status onto nodes from their access verdict', () => {
    const nodes = toFileTreeNodes(accessCache);
    expect(nodes.find(n => n.id === 'src')!.status).toBe('allow');
    expect(nodes.find(n => n.id === 'secret')!.status).toBe('deny');
    expect(nodes.find(n => n.id === '../escape')!.status).toBe('unreachable');
  });

  it('adds a tooltip only when labels are provided', () => {
    const withLabels = toFileTreeNodes(accessCache, undefined, LABELS);
    expect(withLabels.find(n => n.id === 'secret')!.title).toBe('Write: deny (ro)');
    expect(withLabels.find(n => n.id === '../escape')!.title).toBe('Outside');
    const withoutLabels = toFileTreeNodes(accessCache);
    expect(withoutLabels.find(n => n.id === 'secret')!.title).toBeUndefined();
  });

  it('leaves status/title absent for the raw (no-access) view', () => {
    const raw: DirCache = { [ROOT_DIR]: [{ path: 'a.ts', name: 'a.ts', isDirectory: false }] };
    const node = toFileTreeNodes(raw)[0];
    expect(node.status).toBeUndefined();
    expect(node.title).toBeUndefined();
  });
});

describe('flattenFiles', () => {
  it('flattens loaded files across directories, de-duplicated and sorted', () => {
    expect(flattenFiles(cache)).toEqual([
      { path: 'README.md', name: 'README.md' },
      { path: 'src/app.ts', name: 'app.ts' },
    ]);
  });

  it('excludes directories', () => {
    expect(flattenFiles(cache).some(f => f.path === 'src')).toBe(false);
  });
});

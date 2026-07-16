import { describe, expect, it } from 'vitest';
import { flattenFiles, ROOT_DIR, toFileTreeNodes, type DirCache } from './workspaceTree';

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

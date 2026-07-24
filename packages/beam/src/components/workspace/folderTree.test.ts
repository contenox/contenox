import { describe, expect, it } from 'vitest';
import type { DirCache } from '../../pages/chat/lib/workspaceTree';
import { folderBasename, joinRootRelative, toFolderNodes } from './folderTree';

describe('joinRootRelative', () => {
  it('joins a root and a root-relative directory', () => {
    expect(joinRootRelative('/home/naro', 'src/github.com')).toBe('/home/naro/src/github.com');
  });

  it('resolves a blank, "." or "/" relative path to the root itself', () => {
    expect(joinRootRelative('/home/naro', '')).toBe('/home/naro');
    expect(joinRootRelative('/home/naro', '.')).toBe('/home/naro');
    expect(joinRootRelative('/home/naro', '/')).toBe('/home/naro');
  });

  it('never doubles or drops the separator', () => {
    expect(joinRootRelative('/home/naro/', '/src/')).toBe('/home/naro/src');
    expect(joinRootRelative('/home/naro', './src')).toBe('/home/naro/src');
  });
});

describe('folderBasename', () => {
  it('returns the last segment of a path', () => {
    expect(folderBasename('/home/naro/src/runtime')).toBe('runtime');
    expect(folderBasename('runtime')).toBe('runtime');
  });

  it('returns "" for the empty path or a bare root', () => {
    expect(folderBasename('')).toBe('');
    expect(folderBasename('/')).toBe('');
  });
});

describe('toFolderNodes', () => {
  const cache: DirCache = {
    '': [
      { path: 'src', name: 'src', isDirectory: true },
      { path: 'README.md', name: 'README.md', isDirectory: false },
    ],
    src: [
      { path: 'src/app', name: 'app', isDirectory: true },
      { path: 'src/index.ts', name: 'index.ts', isDirectory: false },
    ],
  };

  it('keeps directories and drops files', () => {
    const nodes = toFolderNodes(cache);
    expect(nodes.map(n => n.id)).toEqual(['src']);
    expect(nodes[0].isDirectory).toBe(true);
  });

  it('populates children only for loaded directories, leaving unloaded ones lazy', () => {
    const nodes = toFolderNodes(cache);
    // `src` is loaded → its children are built (directories only).
    expect(nodes[0].children?.map(n => n.id)).toEqual(['src/app']);
    // `src/app` is not in the cache → its children stay undefined (load on expand).
    expect(nodes[0].children?.[0].children).toBeUndefined();
  });

  it('returns [] for a directory absent from the cache', () => {
    expect(toFolderNodes(cache, 'nope')).toEqual([]);
  });
});

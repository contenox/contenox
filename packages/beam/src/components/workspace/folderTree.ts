/**
 * Pure helpers for the "add a project" folder picker (see AddProjectDialog).
 * They turn the `/files` browse cache into a directory-only `FileTree` and
 * resolve a picked, root-relative directory back to the absolute path the
 * POST /workspace/roots grant needs. DOM-free and fetch-free — `useWorkspaceFiles`
 * owns the browsing; these are what the unit tests exercise, mirroring the split
 * in the session tree's `workspaceTree.ts`.
 */
import type { FileTreeNode } from '@contenox/ui';
import { ROOT_DIR, type DirCache } from '../../pages/chat/lib/workspaceTree';

/**
 * The absolute path of a folder picked inside an allowlisted browse root: the
 * root joined with the entry's root-relative path (what the `/files` listing and
 * the FileTree node id carry). A blank/`.`/`/` relative path resolves to the root
 * itself. Trailing and leading slashes are normalized so the join never doubles
 * or drops a separator.
 */
export function joinRootRelative(root: string, rel: string): string {
  const base = root.replace(/\/+$/, '');
  const cleaned = rel.replace(/^\.?\/+/, '').replace(/\/+$/, '');
  return cleaned === '' || cleaned === '.' ? base : `${base}/${cleaned}`;
}

/**
 * The last path segment of an absolute or relative path — the default project
 * name for a picked folder. `''` for the empty path or a bare `"/"`.
 */
export function folderBasename(path: string): string {
  const segments = path.split('/').filter(s => s.length > 0);
  return segments.length > 0 ? segments[segments.length - 1] : '';
}

/**
 * Directory-only `FileTree` nodes from the `/files` browse cache. A project root
 * is always a directory, so files are dropped — the picker shows folders alone,
 * keeping the tree scannable. Mirrors `toFileTreeNodes`' lazy shape: a
 * subdirectory's `children` is populated only once that directory has been loaded
 * and left `undefined` until then, so an unexpanded folder can lazy-load on click.
 */
export function toFolderNodes(cache: DirCache, dirPath: string = ROOT_DIR): FileTreeNode[] {
  const entries = cache[dirPath];
  if (!entries) return [];
  return entries
    .filter(e => e.isDirectory)
    .map(e => ({
      id: e.path,
      name: e.name,
      path: e.path,
      isDirectory: true,
      children: cache[e.path] ? toFolderNodes(cache, e.path) : undefined,
    }));
}

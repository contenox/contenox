/**
 * Pure helpers turning the per-directory `/files` cache into the shapes the
 * workspace panel and the `@`-mention menu consume: a `FileTree` node tree
 * (built lazily — a directory's children appear only once that directory has
 * been loaded) and a flat file list for mention autocomplete. No React, no
 * fetching — `useWorkspaceFiles` owns those; this is what the tests exercise.
 */
import type { FileTreeNode } from '@contenox/ui';
import type { WorkspaceFileRef } from './mentions';

export interface WorkspaceEntry {
  /** Path relative to the workspace root. */
  path: string;
  name: string;
  isDirectory: boolean;
}

/** Per-directory listing cache. Key: a directory's root-relative path; the root is `''`. */
export type DirCache = Record<string, WorkspaceEntry[] | undefined>;

/** The cache key for the root directory. */
export const ROOT_DIR = '';

/**
 * Builds `FileTree` nodes for the directory at `dirPath` from the cache. A
 * subdirectory's `children` is populated when that directory is loaded and left
 * `undefined` when it is not (so the tree can lazy-load on expand). Files carry
 * no children.
 */
export function toFileTreeNodes(cache: DirCache, dirPath: string = ROOT_DIR): FileTreeNode[] {
  const entries = cache[dirPath];
  if (!entries) return [];
  return entries.map(e => ({
    id: e.path,
    name: e.name,
    path: e.path,
    isDirectory: e.isDirectory,
    children: e.isDirectory ? (cache[e.path] ? toFileTreeNodes(cache, e.path) : undefined) : undefined,
  }));
}

/**
 * Flattens every loaded file (not directories) across the cache into a
 * de-duplicated mention list, sorted by path — the candidate set the `@`-menu
 * autocompletes over. Only loaded directories contribute, so mentioning a deep
 * file requires the tree to have listed its directory first.
 */
export function flattenFiles(cache: DirCache): WorkspaceFileRef[] {
  const seen = new Set<string>();
  const out: WorkspaceFileRef[] = [];
  for (const entries of Object.values(cache)) {
    if (!entries) continue;
    for (const e of entries) {
      if (e.isDirectory || seen.has(e.path)) continue;
      seen.add(e.path);
      out.push({ path: e.path, name: e.name });
    }
  }
  out.sort((a, b) => a.path.localeCompare(b.path));
  return out;
}

/**
 * Pure helpers turning the per-directory `/files` cache into the shapes the
 * workspace panel and the `@`-mention menu consume: a `FileTree` node tree
 * (built lazily — a directory's children appear only once that directory has
 * been loaded) and a flat file list for mention autocomplete. No React, no
 * fetching — `useWorkspaceFiles` owns those; this is what the tests exercise.
 */
import type { FileTreeNode, FileTreeNodeStatus } from '@contenox/ui';
import type { WorkspaceFileRef } from './mentions';

/**
 * The agent-view verdict the `/files?filter=agent` endpoint annotates each entry
 * with (mirrors `agentview.Verdict`): whether the path is reachable at all
 * (inside the workspace boundary) and, if so, the HITL action a read/write would
 * take under the active policy (`allow` | `approve` | `deny`), with an optional
 * human reason. Absent entirely in the raw (non-agent) view.
 */
export interface WorkspaceAccess {
  reachable: boolean;
  read?: string;
  write?: string;
  readReason?: string;
  writeReason?: string;
}

/** i18n'd labels used to build a row tooltip from a {@link WorkspaceAccess}. */
export interface AccessLabels {
  unreachable: string;
  read: string;
  write: string;
}

/** Worst-of(read, write) severity, or `unreachable` when outside the workspace boundary. */
export function accessToStatus(access: WorkspaceAccess): FileTreeNodeStatus {
  if (!access.reachable) return 'unreachable';
  if (access.read === 'deny' || access.write === 'deny') return 'deny';
  if (access.read === 'approve' || access.write === 'approve') return 'approve';
  return 'allow';
}

/** Builds a row tooltip from a verdict's reasons, or the boundary marker when unreachable. */
export function accessTooltip(access: WorkspaceAccess, labels: AccessLabels): string | undefined {
  if (!access.reachable) return labels.unreachable;
  const parts: string[] = [];
  if (access.read && access.read !== 'allow') {
    parts.push(`${labels.read}: ${access.read}${access.readReason ? ` (${access.readReason})` : ''}`);
  }
  if (access.write && access.write !== 'allow') {
    parts.push(`${labels.write}: ${access.write}${access.writeReason ? ` (${access.writeReason})` : ''}`);
  }
  return parts.length > 0 ? parts.join(' · ') : undefined;
}

export interface WorkspaceEntry {
  /** Path relative to the workspace root. */
  path: string;
  name: string;
  isDirectory: boolean;
  /** Agent-view verdict for this path; present only under the `agent` filter. */
  access?: WorkspaceAccess;
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
export function toFileTreeNodes(
  cache: DirCache,
  dirPath: string = ROOT_DIR,
  labels?: AccessLabels,
): FileTreeNode[] {
  const entries = cache[dirPath];
  if (!entries) return [];
  return entries.map(e => {
    const status = e.access ? accessToStatus(e.access) : undefined;
    const title = e.access && labels ? accessTooltip(e.access, labels) : undefined;
    return {
      id: e.path,
      name: e.name,
      path: e.path,
      isDirectory: e.isDirectory,
      children: e.isDirectory ? (cache[e.path] ? toFileTreeNodes(cache, e.path, labels) : undefined) : undefined,
      ...(status ? { status } : {}),
      ...(title ? { title } : {}),
    };
  });
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

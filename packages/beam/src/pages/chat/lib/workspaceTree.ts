/**
 * Pure helpers turning the per-directory `/files` cache into the shapes the
 * workspace panel and the `@`-mention menu consume: a `FileTree` node tree
 * (built lazily — a directory's children appear only once that directory has
 * been loaded) and a flat file list for mention autocomplete. No React, no
 * fetching — `useWorkspaceFiles` owns those; this is what the tests exercise.
 *
 * The agent-view overlay is now a SEPARATE data path: the listing here is raw
 * (no verdict), and `POST /workspace/access` (see `lib/workspaceAccess`) supplies
 * a `path → verdict` map the panel merges in. This module owns the pure, icon-free
 * projection of a verdict onto the two independent axes the tree renders
 * ({@link verdictToAccess}); the panel attaches the eye/pencil icons and threads
 * the result onto nodes via a {@link NodeDecorator}.
 */
import type { FileTreeIndicatorStatus, FileTreeNode } from '@contenox/ui';
import type { DimensionVerdict, PathVerdict } from '../../../lib/workspaceAccess';
import type { WorkspaceFileRef } from './mentions';

/**
 * Localized phrases used to build the two-axis access tooltips from the
 * STRUCTURED verdict codes (not English server strings): the dimension names, the
 * per-action phrase, the per-reason phrase, the boundary marker, and the format
 * that assembles them (e.g. "Read: needs approval — default policy").
 */
export interface AccessLabels {
  /** "Read" — the read dimension name. */
  read: string;
  /** "Write" — the write dimension name. */
  write: string;
  /** "Outside the workspace boundary" — the unreachable marker. */
  unreachable: string;
  /** "allowed" — action === 'allow'. */
  actionAllow: string;
  /** "needs approval" — action === 'approve'. */
  actionApprove: string;
  /** "blocked" — action === 'deny'. */
  actionDeny: string;
  /** "policy rule" — reason === 'matched_rule'. */
  reasonRule: string;
  /** "default policy" — reason === 'default_action' (or absent). */
  reasonDefault: string;
  /** Assembles one dimension's tooltip, e.g. (dim, action, reason) => `${dim}: ${action} — ${reason}`. */
  format: (dim: string, action: string, reason: string) => string;
}

/** One rendered access axis: a severity (which tints the icon) + its localized tooltip. */
export interface AxisIndicator {
  status: FileTreeIndicatorStatus;
  title: string;
}

/**
 * The two-axis, icon-free view of a path verdict: whether the row is dimmed
 * (unreachable) and the per-dimension read/write markers. The panel maps this
 * onto `FileTree` indicators by attaching the eye (read) and pencil (write) icons.
 */
export interface NodeAccess {
  dimmed: boolean;
  read?: AxisIndicator;
  write?: AxisIndicator;
}

function actionLabel(action: DimensionVerdict['action'], labels: AccessLabels): string {
  switch (action) {
    case 'allow':
      return labels.actionAllow;
    case 'approve':
      return labels.actionApprove;
    case 'deny':
      return labels.actionDeny;
  }
}

function reasonLabel(dim: DimensionVerdict, labels: AccessLabels): string {
  return dim.reason === 'matched_rule' ? labels.reasonRule : labels.reasonDefault;
}

function axisIndicator(dimName: string, dim: DimensionVerdict, labels: AccessLabels): AxisIndicator {
  return {
    status: dim.action,
    title: labels.format(dimName, actionLabel(dim.action, labels), reasonLabel(dim, labels)),
  };
}

/**
 * Maps a structured {@link PathVerdict} onto the two independent axes the tree
 * renders. An unreachable path dims the row and shows a muted marker on BOTH axes
 * (with the boundary tooltip); a reachable path maps each dimension's action to
 * its own severity + a tooltip assembled from the structured action/reason codes.
 * Read and write are kept independent — a normal file is `allow` read / `approve`
 * write, so green (read) and yellow (write) appear side by side.
 */
export function verdictToAccess(verdict: PathVerdict, labels: AccessLabels): NodeAccess {
  if (!verdict.reachable) {
    const marker: AxisIndicator = { status: 'unreachable', title: labels.unreachable };
    return { dimmed: true, read: marker, write: marker };
  }
  return {
    dimmed: false,
    ...(verdict.read ? { read: axisIndicator(labels.read, verdict.read, labels) } : {}),
    ...(verdict.write ? { write: axisIndicator(labels.write, verdict.write, labels) } : {}),
  };
}

/**
 * Node fields a caller attaches from a verdict: the trailing indicators (icons
 * wired by the caller), whether the row is dimmed, and an optional row tooltip.
 */
export type NodeDecoration = Pick<FileTreeNode, 'indicators' | 'dimmed' | 'title'>;

/** Supplies the per-path {@link NodeDecoration} (verdict → indicators); returns undefined for no overlay. */
export type NodeDecorator = (path: string) => NodeDecoration | undefined;

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
 * no children. When a {@link NodeDecorator} is given, each node is enriched with
 * its access overlay (indicators / dimmed / tooltip) merged by path.
 */
export function toFileTreeNodes(
  cache: DirCache,
  dirPath: string = ROOT_DIR,
  decorate?: NodeDecorator,
): FileTreeNode[] {
  const entries = cache[dirPath];
  if (!entries) return [];
  return entries.map(e => ({
    id: e.path,
    name: e.name,
    path: e.path,
    isDirectory: e.isDirectory,
    children: e.isDirectory ? (cache[e.path] ? toFileTreeNodes(cache, e.path, decorate) : undefined) : undefined,
    ...(decorate?.(e.path) ?? {}),
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

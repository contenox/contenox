/**
 * The workspace panel's filter facility: a small, extensible registry of filter
 * *types* (extension, glob, name, agent-view access verdict) plus the pure helper
 * that turns the server's flat match stream into a `FileTree`. Deliberately not
 * hardwired to one kind of filter — adding a new type is one entry in
 * {@link WORKSPACE_FILTER_TYPES}.
 *
 * Matching itself runs SERVER-SIDE: each type compiles its value into a
 * {@link FindQuery} — the `glob` patterns sent to `GET /api/workspace/find` (which
 * walks the whole tree in one request) plus an optional client-side `refine` for
 * constraints a filename glob can't express (e.g. the agent-view verdict). No
 * React, no fetching — this is what the tests exercise, mirroring `workspaceTree.ts`.
 */
import type { FileTreeNode } from '@contenox/ui';
import type { WorkspaceFindMatch } from '../../../lib/workspaceFind';
import { accessToStatus, accessTooltip, type AccessLabels } from './workspaceTree';

/** The value affordance a filter type wants: a free-text box, or a fixed option set. */
export type FilterInput =
  | { kind: 'text'; placeholderKey: string }
  | { kind: 'options'; options: string[] };

/**
 * What a filter value compiles to: the server-side glob patterns to walk for, plus
 * an optional client-side predicate applied to each streamed match (for a
 * constraint no filename glob expresses, like an access verdict). An empty `globs`
 * means "match every file" (the server receives `*`), used when the real filter is
 * entirely a `refine`.
 */
export interface FindQuery {
  globs: string[];
  refine?: (match: WorkspaceFindMatch) => boolean;
}

export interface WorkspaceFilterType {
  /** Stable id (persisted as the selected type). */
  id: string;
  /** i18n key for the type's display label (namespace `workspace`). */
  labelKey: string;
  /** How the panel should collect this type's value. */
  input: FilterInput;
  /**
   * Whether this type is offered in the current view. Absent = always offered. The
   * `access` type only makes sense under the agent-view overlay (matches carry a
   * verdict only when the find request is `filter=agent`).
   */
  appliesTo?: (ctx: { agentView: boolean }) => boolean;
  /**
   * Compiles the raw input value into a {@link FindQuery}, or `null` when the value
   * imposes no constraint (empty / whitespace) — a `null` query means "filter
   * inactive", so the ordinary lazy tree shows.
   */
  toQuery: (value: string) => FindQuery | null;
}

/** Splits a comma/space list into trimmed, non-empty tokens. */
function tokens(value: string): string[] {
  return value
    .split(/[,\s]+/)
    .map(s => s.trim())
    .filter(Boolean);
}

/**
 * The built-in filter types. Ordered as offered in the type picker; the first
 * applicable one is the default. Extend this array to add a filter kind — the
 * panel and the pure tree-builder pick it up with no further wiring.
 */
export const WORKSPACE_FILTER_TYPES: WorkspaceFilterType[] = [
  {
    id: 'ext',
    labelKey: 'workspace.filter_type_ext',
    input: { kind: 'text', placeholderKey: 'workspace.filter_placeholder_ext' },
    toQuery: value => {
      // Accept `md`, `.md`, `*.md`, and comma/space lists like `md, ts` → one
      // `*.<ext>` glob per extension.
      const exts = tokens(value)
        .map(s => s.replace(/^[*.]+/, '').toLowerCase())
        .filter(Boolean);
      if (exts.length === 0) return null;
      return { globs: exts.map(ext => `*.${ext}`) };
    },
  },
  {
    id: 'glob',
    labelKey: 'workspace.filter_type_glob',
    input: { kind: 'text', placeholderKey: 'workspace.filter_placeholder_glob' },
    toQuery: value => {
      // Comma/space-separated filepath.Match patterns (server semantics: `*`, `?`,
      // `[…]`, and a `/` switches to full-path matching). No `{a,b}` braces.
      const globs = tokens(value);
      if (globs.length === 0) return null;
      return { globs };
    },
  },
  {
    id: 'name',
    labelKey: 'workspace.filter_type_name',
    input: { kind: 'text', placeholderKey: 'workspace.filter_placeholder_name' },
    toQuery: value => {
      const v = value.trim();
      if (!v) return null;
      // Basename substring, expressed as a `*v*` glob (server matches the basename
      // when the pattern has no `/`).
      return { globs: [`*${v}*`] };
    },
  },
  {
    id: 'access',
    labelKey: 'workspace.filter_type_access',
    input: { kind: 'options', options: ['approve', 'deny', 'unreachable', 'allow'] },
    appliesTo: ({ agentView }) => agentView,
    toQuery: value => {
      const v = value.trim();
      if (!v) return null;
      // No filename glob expresses a verdict, so walk every file (`*`) and keep only
      // those whose worst read/write verdict equals the selection. Requires the
      // find request to carry filter=agent (the panel sends it under agent view).
      return {
        globs: ['*'],
        refine: m => (m.access ? accessToStatus(m.access) === v : false),
      };
    },
  },
];

/** Looks up a filter type by id. */
export function filterTypeById(id: string): WorkspaceFilterType | undefined {
  return WORKSPACE_FILTER_TYPES.find(t => t.id === id);
}

/** The filter types offered in the current view (honours each type's `appliesTo`). */
export function availableFilterTypes(ctx: { agentView: boolean }): WorkspaceFilterType[] {
  return WORKSPACE_FILTER_TYPES.filter(t => !t.appliesTo || t.appliesTo(ctx));
}

interface DirBuild {
  dirs: Map<string, DirBuild>;
  files: FileTreeNode[];
}

function serializeDir(dir: DirBuild, prefix: string): FileTreeNode[] {
  const dirNodes: FileTreeNode[] = [];
  for (const [name, child] of dir.dirs) {
    const path = prefix ? `${prefix}/${name}` : name;
    dirNodes.push({ id: path, name, path, isDirectory: true, children: serializeDir(child, path) });
  }
  dirNodes.sort((a, b) => a.name.localeCompare(b.name));
  const fileNodes = [...dir.files].sort((a, b) => a.name.localeCompare(b.name));
  // Dirs first, then files — matching the /files listing convention.
  return [...dirNodes, ...fileNodes];
}

/**
 * Builds a {@link FileTreeNode} tree from the flat list of matching FILE entries
 * the find stream returns, synthesizing the ancestor directory nodes each path
 * implies. File leaves carry the agent-view status dot + tooltip when the match
 * has an `access` verdict (and `labels` are given). Directories are structural
 * (no verdict — the find stream annotates files only). Pure; never mutates input.
 */
export function buildTreeFromMatches(
  matches: readonly WorkspaceFindMatch[],
  labels?: AccessLabels,
): FileTreeNode[] {
  const root: DirBuild = { dirs: new Map(), files: [] };
  for (const m of matches) {
    const parts = m.path.split('/').filter(Boolean);
    if (parts.length === 0) continue;
    let cur = root;
    for (let i = 0; i < parts.length - 1; i++) {
      let child = cur.dirs.get(parts[i]);
      if (!child) {
        child = { dirs: new Map(), files: [] };
        cur.dirs.set(parts[i], child);
      }
      cur = child;
    }
    const status = m.access ? accessToStatus(m.access) : undefined;
    const title = m.access && labels ? accessTooltip(m.access, labels) : undefined;
    cur.files.push({
      id: m.path,
      name: parts[parts.length - 1],
      path: m.path,
      isDirectory: false,
      ...(status ? { status } : {}),
      ...(title ? { title } : {}),
    });
  }
  return serializeDir(root, '');
}

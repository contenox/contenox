/**
 * The workspace panel's filter facility: a small, extensible registry of filter
 * *types* (extension, glob, name, and the two agent-view access axes) plus the
 * pure helper that turns the server's flat match stream into a `FileTree`.
 * Deliberately not hardwired to one kind of filter — adding a new type is one
 * entry in {@link WORKSPACE_FILTER_TYPES}.
 *
 * Matching itself runs SERVER-SIDE: each type compiles its value into a
 * {@link FindQuery} — the `glob` patterns sent to `GET /api/workspace/find` (which
 * walks the whole tree in one request) plus an optional client-side `refine` for
 * constraints a filename glob can't express. The access axes refine on the
 * per-path verdict from `POST /workspace/access` (find is now a RAW listing;
 * verdicts arrive out-of-band and are threaded into `refine` by the panel). No
 * React, no fetching — this is what the tests exercise, mirroring `workspaceTree.ts`.
 */
import type { FileTreeNode } from '@contenox/ui';
import type { PathVerdict } from '../../../lib/workspaceAccess';
import type { WorkspaceFindMatch } from '../../../lib/workspaceFind';
import type { NodeDecorator } from './workspaceTree';

/** The value affordance a filter type wants: a free-text box, or a fixed option set. */
export type FilterInput =
  | { kind: 'text'; placeholderKey: string }
  | { kind: 'options'; options: string[] };

/**
 * What a filter value compiles to: the server-side glob patterns to walk for, plus
 * an optional client-side predicate applied to each streamed match. The predicate
 * receives the match's per-path access verdict (from the batch `/workspace/access`
 * call, threaded in by the panel) so an axis filter can keep only the files whose
 * read/write verdict matches. An empty `globs` means "match every file" (the
 * server receives `*`), used when the real filter is entirely a `refine`.
 */
export interface FindQuery {
  globs: string[];
  refine?: (match: WorkspaceFindMatch, verdict?: PathVerdict) => boolean;
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
   * access axes only make sense under the agent-view overlay (the panel only
   * fetches verdicts, which the `refine` needs, when agent view is on).
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
 * Whether one access dimension of a path's verdict matches the selected value.
 * `unreachable` matches paths outside the workspace boundary; the three actions
 * (`allow`/`approve`/`deny`) match a reachable path whose read/write action
 * equals the selection. A missing verdict (not yet evaluated) never matches.
 */
export function axisMatches(
  verdict: PathVerdict | undefined,
  dimension: 'read' | 'write',
  value: string,
): boolean {
  if (!verdict) return false;
  if (value === 'unreachable') return !verdict.reachable;
  if (!verdict.reachable) return false;
  return verdict[dimension]?.action === value;
}

/** The fixed option set shared by both access axes. */
const ACCESS_AXIS_OPTIONS = ['allow', 'approve', 'deny', 'unreachable'];

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
    id: 'access_read',
    labelKey: 'workspace.filter_type_access_read',
    input: { kind: 'options', options: ACCESS_AXIS_OPTIONS },
    appliesTo: ({ agentView }) => agentView,
    toQuery: value => {
      const v = value.trim();
      if (!v) return null;
      // No filename glob expresses a verdict, so walk every file (`*`) and keep only
      // those whose READ verdict equals the selection. The panel supplies the
      // per-path verdict to `refine` from the batch /workspace/access call.
      return { globs: ['*'], refine: (_m, verdict) => axisMatches(verdict, 'read', v) };
    },
  },
  {
    id: 'access_write',
    labelKey: 'workspace.filter_type_access_write',
    input: { kind: 'options', options: ACCESS_AXIS_OPTIONS },
    appliesTo: ({ agentView }) => agentView,
    toQuery: value => {
      const v = value.trim();
      if (!v) return null;
      // As `access_read`, but refines on the WRITE verdict.
      return { globs: ['*'], refine: (_m, verdict) => axisMatches(verdict, 'write', v) };
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
 * implies. When a {@link NodeDecorator} is given, file leaves carry the agent-view
 * access overlay (indicators / dimmed / tooltip) merged by path. Directories are
 * structural (the find stream returns files only, so they get no overlay). Pure;
 * never mutates input.
 */
export function buildTreeFromMatches(
  matches: readonly WorkspaceFindMatch[],
  decorate?: NodeDecorator,
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
    cur.files.push({
      id: m.path,
      name: parts[parts.length - 1],
      path: m.path,
      isDirectory: false,
      ...(decorate?.(m.path) ?? {}),
    });
  }
  return serializeDir(root, '');
}

import type { WorkspaceRoot } from './types';

/**
 * Pure helpers for the workspace-root picker (source: `GET /workspace/roots`).
 * Kept DOM-free and side-effect-free so the picker's decisions — which root is
 * active, how a long path is shortened for a chip, and whether a server refusal
 * is the "outside the permitted roots" one — are unit-testable without a
 * component or a live server (matches dispatchForm.ts / unitStatus.ts).
 */

/**
 * The root a picker should show as active when nothing else is selected: the
 * one flagged `default` (what the empty cwd and "/" resolve to server-side),
 * falling back to the first entry, or undefined for an empty allowlist.
 */
export function activeWorkspaceRoot(roots: readonly WorkspaceRoot[]): WorkspaceRoot | undefined {
  return roots.find(r => r.default) ?? roots[0];
}

/**
 * Compact display for an absolute root path in a chip: keeps the last
 * `maxSegments` path segments, prefixing an ellipsis when it dropped any. A
 * short path is returned unchanged. Never trims to empty — a single segment
 * always survives. Purely cosmetic; the full path stays available as a title.
 */
export function shortenRootPath(path: string, maxSegments = 3): string {
  const trimmed = path.replace(/\/+$/, '');
  if (trimmed === '') return path;
  const segments = trimmed.split('/').filter(s => s.length > 0);
  if (segments.length <= maxSegments) return trimmed;
  return '…/' + segments.slice(-maxSegments).join('/');
}

/** The basename of an absolute path — its last non-empty segment — or `''` for
 * `"/"` or an empty path. Internal to the project-name helpers below. */
function basename(path: string): string {
  const segments = path.split('/').filter(s => s.length > 0);
  return segments.length > 0 ? segments[segments.length - 1] : '';
}

/**
 * The friendly name to show for a root: its server-supplied `name` (the
 * EXPLICIT marker name — the server sends no fallback), falling back to the
 * basename of its `path` when the name is absent or empty, and to the raw path
 * itself only for a `"/"` root that has neither. Never empty for a non-empty
 * path, so a chip or label always has something to render.
 */
export function projectName(root: WorkspaceRoot): string {
  const name = root.name?.trim();
  if (name) return name;
  return basename(root.path) || root.path;
}

/** Whether `root` contains `target` on a path-segment boundary: `/a/b` contains
 * `/a/b` and `/a/b/c` but NOT `/a/bc`. Both are expected trailing-slash-free. */
function containsPath(root: string, target: string): boolean {
  return target === root || target.startsWith(root + '/');
}

/**
 * The root that CONTAINS a cwd: the one whose `path` is the LONGEST segment-aware
 * prefix of `cwd`. Segment-aware means `/a/b` contains `/a/b` and `/a/b/c` but
 * NOT `/a/bc`, so sibling folders sharing a name prefix never cross-match; the
 * matching roots form an ancestor chain, so longest path = deepest = most
 * specific. Returns null when no root contains the cwd — an absent cwd, or a
 * legacy session opened outside every current grant.
 */
export function projectForCwd(
  cwd: string | undefined | null,
  roots: readonly WorkspaceRoot[],
): WorkspaceRoot | null {
  if (!cwd) return null;
  const target = cwd.replace(/\/+$/, '');
  let best: WorkspaceRoot | null = null;
  let bestLen = -1;
  for (const root of roots) {
    const rootPath = root.path.replace(/\/+$/, '');
    if (!containsPath(rootPath, target)) continue;
    if (rootPath.length > bestLen) {
      best = root;
      bestLen = rootPath.length;
    }
  }
  return best;
}

/**
 * The project name for the root that contains a cwd (see {@link projectForCwd})
 * — considering ONLY explicitly named roots (real registered projects). A
 * structural root without a marker name (serve's home default, a legacy
 * pre-registry grant) must NOT swallow the label: a session at
 * `/home/me/demo-app` under a bare home root should read "demo-app" (the
 * caller's cwd-basename fallback), not "me". Null when no named root contains
 * the cwd.
 */
export function workspaceNameForCwd(
  cwd: string | undefined | null,
  roots: readonly WorkspaceRoot[],
): string | null {
  const named = roots.filter(r => (r.name?.trim() ?? '') !== '');
  const root = projectForCwd(cwd, named);
  return root ? projectName(root) : null;
}

/**
 * Whether a server error message is the workspace-root refusal — the 422 the
 * per-request `root` check returns for a path outside the allowlist (see
 * localfileapi.wrap: `%w: workspace root %q is not permitted`). Matched on the
 * stable phrasing rather than the wrapped ErrUnprocessableEntity prefix, and
 * case-insensitively, so a legible localized notice can replace the raw wire
 * string.
 */
export function isWorkspaceRootRefusal(message: string | undefined | null): boolean {
  if (!message) return false;
  const m = message.toLowerCase();
  return m.includes('workspace root') && m.includes('not permitted');
}

/**
 * The offending path pulled out of a workspace-root refusal message (the value
 * inside the `%q` double-quotes), or null when none is present. Lets the notice
 * name the exact folder that was rejected.
 */
export function extractRefusedRoot(message: string | undefined | null): string | null {
  if (!message) return null;
  const match = message.match(/workspace root\s+"([^"]*)"/i);
  return match ? match[1] : null;
}

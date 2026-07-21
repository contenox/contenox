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

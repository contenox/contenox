import type { PlanRevisionSummary } from './types';

/**
 * Pure, DOM-free helpers for the plan-revision feed (component roadmap Tier 2
 * item 6 / attention-layer.md): the durable "+2/−1 — why" history a mission
 * accrues, shown newest-first on the mission detail and, compactly, in the
 * inbox's per-mission groups. Kept out of the components so the ordering and the
 * delta formatting are unit-testable without a component (matches
 * inboxPresentation.ts).
 *
 * The wire order is oldest-first (newest is the final element); every surface
 * here wants newest-first, so that reversal is the one shared decision.
 */

/**
 * A mission's revisions newest-first. Returns a fresh array (never mutates the
 * input), and an empty array for an absent/empty ring — so a caller renders
 * NOTHING (no empty shell) exactly when this is empty.
 */
export function revisionsNewestFirst(
  revisions?: readonly PlanRevisionSummary[] | null,
): PlanRevisionSummary[] {
  if (!revisions || revisions.length === 0) return [];
  return revisions.slice().reverse();
}

/** The most recent revision, or undefined when there is none — the inbox's compact one-liner source. */
export function latestRevision(
  revisions?: readonly PlanRevisionSummary[] | null,
): PlanRevisionSummary | undefined {
  if (!revisions || revisions.length === 0) return undefined;
  return revisions[revisions.length - 1];
}

/**
 * The entry delta of a revision as a compact "+2/−1" string. A side with a zero
 * count is omitted (a pure-addition revision is "+2", not "+2/−0"); a revision
 * that changed no entry count — a reorder or a status-only change — yields '±0'
 * rather than an empty or misleading string.
 */
export function formatRevisionDelta(added: number, removed: number): string {
  const parts: string[] = [];
  if (added > 0) parts.push(`+${added}`);
  if (removed > 0) parts.push(`−${removed}`); // U+2212 MINUS SIGN, matching the design "+2/−1"
  if (parts.length === 0) return '±0';
  return parts.join('/');
}

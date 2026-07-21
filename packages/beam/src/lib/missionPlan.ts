import type { Status } from '@contenox/ui';
import type { TranslationKey } from '../i18n';
import type { MissionPlan, MissionPlanEntryPriority, MissionPlanEntryStatus } from './types';

/**
 * A mission's plan, reduced to the one fact every fleet surface shows beside the
 * composed status: how far along it is. `composeUnitStatus` (unitStatus.ts) owns
 * the four STATUS truths (blocked / process / verdict / liveness); plan progress
 * is a distinct fact — not a health signal — so it stays a separate ONE-helper
 * pure function rather than a fifth status atom, and is rendered adjacent to the
 * status atoms by `<PlanProgress>`.
 *
 * Returns null when there is no plan to report — an absent plan, or the zero
 * Plan a never-planned mission carries (empty/absent `entries`). A caller
 * therefore renders NOTHING (no "0/0" shell) exactly when `planProgress` is null.
 */
export type PlanProgress = {
  /** Entries whose status is `completed`. */
  completed: number;
  /** Total entries in the current revision. */
  total: number;
  /** Entries whose status is `in_progress` — the "something is happening now" marker. */
  inProgress: number;
};

/** The completed/total/in-progress counts of a plan, or null when there is no plan. */
export function planProgress(plan?: MissionPlan | null): PlanProgress | null {
  const entries = plan?.entries;
  if (!entries || entries.length === 0) return null;
  let completed = 0;
  let inProgress = 0;
  for (const entry of entries) {
    if (entry.status === 'completed') completed += 1;
    else if (entry.status === 'in_progress') inProgress += 1;
  }
  return { completed, total: entries.length, inProgress };
}

/** Whether a plan has any entries worth rendering (the panel/fragment gate). */
export function hasPlan(plan?: MissionPlan | null): boolean {
  return planProgress(plan) !== null;
}

/**
 * Plan-entry status → StatusIndicator dot, mirroring the chat PlanPanel's read
 * (pending → neutral ○, in_progress → amber ⟳, completed → green ✓) and the
 * same STATE_STATUS idiom every other fleet surface uses.
 */
export const PLAN_ENTRY_STATUS_INDICATOR: Record<MissionPlanEntryStatus, Status> = {
  pending: 'planned',
  in_progress: 'in-progress',
  completed: 'completed',
};

export const PLAN_ENTRY_STATUS_LABEL_KEY: Record<MissionPlanEntryStatus, TranslationKey> = {
  pending: 'plan.status_pending',
  in_progress: 'plan.status_in_progress',
  completed: 'plan.status_completed',
};

export const PLAN_ENTRY_PRIORITY_LABEL_KEY: Record<MissionPlanEntryPriority, TranslationKey> = {
  high: 'plan.priority_high',
  medium: 'plan.priority_medium',
  low: 'plan.priority_low',
};

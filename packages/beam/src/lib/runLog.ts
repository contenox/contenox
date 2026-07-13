import {
  createEmptyTaskEventState,
  reduceTaskEventState,
  type TaskEventViewState,
} from './taskEvents';
import type { TaskEvent } from './types';

/**
 * RunLog retains the reduced task-event view state of every run in a session,
 * keyed by requestId — the console's in-session scrollback memory. Unlike the
 * live-only useTaskEvents state, entries survive run completion; only the
 * retention cap evicts them (oldest first).
 */
export type RunLog = {
  /** requestIds in arrival order (oldest first). */
  order: string[];
  runs: Record<string, TaskEventViewState>;
};

/** Session-scoped cap; beyond it the oldest run's events are evicted. */
export const RUN_RETENTION_LIMIT = 50;

export function emptyRunLog(): RunLog {
  return { order: [], runs: {} };
}

/** Ensures an entry exists for the run (idempotent). */
export function beginRun(log: RunLog, requestId: string): RunLog {
  if (log.runs[requestId]) return log;
  const next: RunLog = {
    order: [...log.order, requestId],
    runs: { ...log.runs, [requestId]: createEmptyTaskEventState() },
  };
  if (next.order.length > RUN_RETENTION_LIMIT) {
    const evicted = next.order.shift()!;
    delete next.runs[evicted];
  }
  return next;
}

export function applyRunEvent(log: RunLog, requestId: string, event: TaskEvent): RunLog {
  const withRun = beginRun(log, requestId);
  return {
    order: withRun.order,
    runs: {
      ...withRun.runs,
      [requestId]: reduceTaskEventState(withRun.runs[requestId], event),
    },
  };
}

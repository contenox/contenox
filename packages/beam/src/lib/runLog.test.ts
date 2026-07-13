import { describe, expect, it } from 'vitest';
import { applyRunEvent, beginRun, emptyRunLog, RUN_RETENTION_LIMIT } from './runLog';
import type { TaskEvent } from './types';

const ev = (kind: TaskEvent['kind'], extra: Partial<TaskEvent> = {}): TaskEvent => ({
  kind,
  timestamp: '2026-07-13T12:00:00Z',
  ...extra,
});

describe('runLog', () => {
  it('retains a completed run instead of resetting it', () => {
    let log = emptyRunLog();
    log = applyRunEvent(log, 'req-1', ev('chain_started', { chain_id: 'c' }));
    log = applyRunEvent(log, 'req-1', ev('step_completed', { task_id: 't1' }));
    log = applyRunEvent(log, 'req-1', ev('chain_completed'));

    expect(log.runs['req-1'].events).toHaveLength(3);
    expect(log.runs['req-1'].status).toBe('Completed');

    // A new run starting leaves the first untouched — the wipe bug is dead.
    log = applyRunEvent(log, 'req-2', ev('chain_started'));
    expect(log.runs['req-1'].events).toHaveLength(3);
    expect(log.order).toEqual(['req-1', 'req-2']);
  });

  it('tracks pending approval per run and clears it on step completion', () => {
    let log = emptyRunLog();
    log = applyRunEvent(
      log,
      'req-1',
      ev('approval_requested', { approval_id: 'a1', tool_name: 'local_shell' }),
    );
    expect(log.runs['req-1'].pendingApproval?.approvalId).toBe('a1');

    log = applyRunEvent(log, 'req-1', ev('step_completed', { task_id: 't1' }));
    expect(log.runs['req-1'].pendingApproval).toBeNull();
    // The approval event itself is retained in scrollback.
    expect(log.runs['req-1'].events.some(e => e.kind === 'approval_requested')).toBe(true);
  });

  it('beginRun is idempotent', () => {
    let log = emptyRunLog();
    log = beginRun(log, 'req-1');
    const again = beginRun(log, 'req-1');
    expect(again).toBe(log);
  });

  it('evicts the oldest run past the retention cap', () => {
    let log = emptyRunLog();
    for (let i = 0; i < RUN_RETENTION_LIMIT + 5; i++) {
      log = beginRun(log, `req-${i}`);
    }
    expect(log.order).toHaveLength(RUN_RETENTION_LIMIT);
    expect(log.runs['req-0']).toBeUndefined();
    expect(log.runs[`req-${RUN_RETENTION_LIMIT + 4}`]).toBeDefined();
  });
});

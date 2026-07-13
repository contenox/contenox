import { describe, expect, it } from 'vitest';
import { DENY_MESSAGE, deriveApprovals } from './approvals';
import type { TaskEvent } from './types';

const ev = (kind: TaskEvent['kind'], extra: Partial<TaskEvent> = {}): TaskEvent => ({
  kind,
  timestamp: '2026-07-13T12:00:00Z',
  ...extra,
});

describe('deriveApprovals', () => {
  it('marks the live pending approval as pending', () => {
    const events = [ev('approval_requested', { approval_id: 'a1', tool_name: 'local_shell' })];
    const entries = deriveApprovals(events, {
      approvalId: 'a1',
      hookName: 'local',
      toolName: 'local_shell',
      args: {},
      diff: '',
    });
    expect(entries).toEqual([{ approvalId: 'a1', toolName: 'local_shell', status: 'pending' }]);
  });

  it('marks an approval followed by an executed tool call as approved', () => {
    const events = [
      ev('approval_requested', { approval_id: 'a1', tool_name: 'local_shell' }),
      ev('tool_call', { tool_name: 'local_shell', content: 'exit 0' }),
      ev('step_completed', { task_id: 't1' }),
    ];
    expect(deriveApprovals(events, null)[0].status).toBe('approved');
  });

  it('marks an approval whose tool result is the deny message as denied', () => {
    const events = [
      ev('approval_requested', { approval_id: 'a1', tool_name: 'local_shell' }),
      ev('tool_call', { tool_name: 'local_shell', content: DENY_MESSAGE }),
    ];
    expect(deriveApprovals(events, null)[0].status).toBe('denied');
  });

  it('treats a step completing without a tool call as resolved', () => {
    const events = [
      ev('approval_requested', { approval_id: 'a1', tool_name: 'local_fs' }),
      ev('step_completed', { task_id: 't1' }),
    ];
    expect(deriveApprovals(events, null)[0].status).toBe('approved');
  });

  it('keeps an unresolved trailing request pending', () => {
    const events = [ev('approval_requested', { approval_id: 'a1', tool_name: 'local_fs' })];
    expect(deriveApprovals(events, null)[0].status).toBe('pending');
  });

  it('marks resolution-by-chain-end as resolved (decision unknowable client-side)', () => {
    const events = [
      ev('approval_requested', { approval_id: 'a1', tool_name: 'local_shell' }),
      ev('chain_completed'),
    ];
    expect(deriveApprovals(events, null)[0].status).toBe('resolved');
  });

  it('matches provider-namespaced tool_call events (local_shell.local_shell)', () => {
    const events = [
      ev('approval_requested', { approval_id: 'a1', tool_name: 'local_shell' }),
      ev('tool_call', { tool_name: 'local_shell.local_shell', content: '{"stdout":"ok"}' }),
    ];
    expect(deriveApprovals(events, null)[0].status).toBe('approved');
  });

  it('handles multiple sequential approvals independently', () => {
    const events = [
      ev('approval_requested', { approval_id: 'a1', tool_name: 'local_shell' }),
      ev('tool_call', { tool_name: 'local_shell', content: 'ok' }),
      ev('approval_requested', { approval_id: 'a2', tool_name: 'local_fs' }),
      ev('tool_call', { tool_name: 'local_fs', content: DENY_MESSAGE }),
    ];
    const entries = deriveApprovals(events, null);
    expect(entries.map(e => e.status)).toEqual(['approved', 'denied']);
  });
});

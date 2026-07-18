import { describe, expect, it } from 'vitest';
import { AGENT_META_KEY } from '../../../lib/acp';
import { resolveActiveAgentName } from './activeAgent';

/**
 * The agent-name shown by the chat header (and, by the same rule, the empty
 * surface's attribution). Pins that a pick staged from the sessions rail is
 * VISIBLE on the empty surface, and that a live session shows its own agent.
 */
describe('resolveActiveAgentName', () => {
  it('shows the STAGED agent on the empty/new-chat surface (the picked agent is visible)', () => {
    expect(
      resolveActiveAgentName({
        sessionMeta: null,
        isEmptySurface: true,
        stagedAgent: 'claude',
        workspaceAgentName: 'contenox',
      }),
    ).toBe('claude');
  });

  it('reflects a re-staged agent on the empty surface (updates when the pick changes)', () => {
    const base = { sessionMeta: null, isEmptySurface: true, workspaceAgentName: 'contenox' };
    expect(resolveActiveAgentName({ ...base, stagedAgent: 'claude' })).toBe('claude');
    // Re-pick: the resolved (rendered) name follows the new staged value.
    expect(resolveActiveAgentName({ ...base, stagedAgent: 'gpt-4o' })).toBe('gpt-4o');
  });

  it('clears to the workspace agent on the empty surface when staging is native (null)', () => {
    expect(
      resolveActiveAgentName({
        sessionMeta: null,
        isEmptySurface: true,
        stagedAgent: null,
        workspaceAgentName: 'contenox',
      }),
    ).toBe('contenox');
  });

  it('ignores the staged agent once a session is active — shows the session\'s own agent', () => {
    expect(
      resolveActiveAgentName({
        sessionMeta: { [AGENT_META_KEY]: 'claude' },
        isEmptySurface: false,
        stagedAgent: 'gpt-4o', // a stale stage must not leak into a live session's label
        workspaceAgentName: 'contenox',
      }),
    ).toBe('claude');
  });

  it('falls back to the workspace agent for a native (no-meta) live session', () => {
    expect(
      resolveActiveAgentName({
        sessionMeta: {},
        isEmptySurface: false,
        stagedAgent: null,
        workspaceAgentName: 'contenox',
      }),
    ).toBe('contenox');
  });

  it('returns null (no label) when nothing is known', () => {
    expect(
      resolveActiveAgentName({
        sessionMeta: null,
        isEmptySurface: true,
        stagedAgent: null,
        workspaceAgentName: undefined,
      }),
    ).toBeNull();
  });
});

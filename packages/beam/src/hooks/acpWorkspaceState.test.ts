import { describe, expect, it } from 'vitest';
import { acpWorkspaceReducer, initialAcpWorkspaceState, type AcpWorkspaceState } from './acpWorkspaceState';

function run(...actions: Parameters<typeof acpWorkspaceReducer>[1][]): AcpWorkspaceState {
  return actions.reduce(acpWorkspaceReducer, initialAcpWorkspaceState);
}

describe('acpWorkspaceReducer: connection status', () => {
  it('walks connecting -> ready -> reconnecting -> disconnected', () => {
    let state = run({ type: 'connecting' });
    expect(state.status).toBe('connecting');

    state = acpWorkspaceReducer(state, { type: 'ready', agentName: 'contenox' });
    expect(state.status).toBe('ready');
    expect(state.agentName).toBe('contenox');
    expect(state.error).toBeNull();

    state = acpWorkspaceReducer(state, { type: 'reconnecting' });
    expect(state.status).toBe('reconnecting');

    state = acpWorkspaceReducer(state, { type: 'disconnected' });
    expect(state.status).toBe('disconnected');
  });

  it('setup_required and error both carry a message and are distinct terminal-ish states', () => {
    const setup = run({ type: 'setup_required', message: 'no default-model configured' });
    expect(setup.status).toBe('setup_required');
    expect(setup.error).toBe('no default-model configured');

    const errored = run({ type: 'error', message: 'boom' });
    expect(errored.status).toBe('error');
    expect(errored.error).toBe('boom');
  });

  it('a fresh connecting/ready clears a prior error', () => {
    const errored = run({ type: 'error', message: 'boom' });
    const reconnected = acpWorkspaceReducer(errored, { type: 'connecting' });
    expect(reconnected.error).toBeNull();
  });
});

describe('acpWorkspaceReducer: session roster freshness sort', () => {
  it('sessions_replaced sorts freshest updatedAt first', () => {
    const state = run({
      type: 'sessions_replaced',
      sessions: [
        { sessionId: 'old', updatedAt: '2026-01-01T00:00:00Z' },
        { sessionId: 'newest', updatedAt: '2026-07-15T00:00:00Z' },
        { sessionId: 'mid', updatedAt: '2026-03-01T00:00:00Z' },
      ],
    });
    expect(state.sessions.map(s => s.sessionId)).toEqual(['newest', 'mid', 'old']);
  });

  it('sessions with no updatedAt sort after every session that has one', () => {
    const state = run({
      type: 'sessions_replaced',
      sessions: [
        { sessionId: 'no-date-1' },
        { sessionId: 'has-date', updatedAt: '2026-01-01T00:00:00Z' },
        { sessionId: 'no-date-2' },
      ],
    });
    expect(state.sessions.map(s => s.sessionId)).toEqual(['has-date', 'no-date-1', 'no-date-2']);
  });

  it('session_upserted inserts a brand-new session and re-sorts', () => {
    const withOne = run({
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'a', updatedAt: '2026-01-01T00:00:00Z' }],
    });
    const state = acpWorkspaceReducer(withOne, {
      type: 'session_upserted',
      session: { sessionId: 'b', updatedAt: '2026-07-01T00:00:00Z' },
    });
    expect(state.sessions.map(s => s.sessionId)).toEqual(['b', 'a']);
  });

  it('session_upserted merges onto an existing entry without clobbering fields the update omits', () => {
    const withOne = run({
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'a', cwd: '/work', title: 'Original title', updatedAt: '2026-01-01T00:00:00Z' }],
    });
    // A live session_info_update carries only sessionId + updatedAt (no title, no cwd) — see acpsvc/prompt.go.
    const state = acpWorkspaceReducer(withOne, {
      type: 'session_upserted',
      session: { sessionId: 'a', updatedAt: '2026-07-01T00:00:00Z' },
    });
    expect(state.sessions[0]).toEqual({ sessionId: 'a', cwd: '/work', title: 'Original title', updatedAt: '2026-07-01T00:00:00Z' });
  });

  it('session_removed drops the session from the roster', () => {
    const withTwo = run({
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'a' }, { sessionId: 'b' }],
    });
    const state = acpWorkspaceReducer(withTwo, { type: 'session_removed', sessionId: 'a' });
    expect(state.sessions.map(s => s.sessionId)).toEqual(['b']);
  });
});

describe('acpWorkspaceReducer: active session', () => {
  it('active_session_changed tracks the currently open session id, including back to null', () => {
    let state = run({ type: 'active_session_changed', sessionId: 'sess-1' });
    expect(state.activeSessionId).toBe('sess-1');
    state = acpWorkspaceReducer(state, { type: 'active_session_changed', sessionId: null });
    expect(state.activeSessionId).toBeNull();
  });
});

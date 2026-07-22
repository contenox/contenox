import { describe, expect, it } from 'vitest';
import {
  acpSessionsReducer,
  acpWorkspaceReducer,
  EMPTY_SESSION_KEY,
  initialAcpSessionsState,
  initialAcpWorkspaceState,
  selectFocusedSession,
  selectOpenSessionIds,
  type AcpSessionsState,
  type AcpWorkspaceState,
} from './acpWorkspaceState';

function run(...actions: Parameters<typeof acpWorkspaceReducer>[1][]): AcpWorkspaceState {
  return actions.reduce(acpWorkspaceReducer, initialAcpWorkspaceState);
}

function runSessions(...actions: Parameters<typeof acpSessionsReducer>[1][]): AcpSessionsState {
  return actions.reduce(acpSessionsReducer, initialAcpSessionsState);
}

describe('acpWorkspaceReducer: connection status', () => {
  it('walks connecting -> ready -> reconnecting -> disconnected', () => {
    let state = run({ type: 'connecting' });
    expect(state.status).toBe('connecting');

    state = acpWorkspaceReducer(state, {
      type: 'ready',
      agentName: 'contenox',
      workspaceConfigOptions: [],
      promptCapabilities: {},
    });
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

  it('ready carries the workspace-level config options and a later ready replaces them (reconnect refresh)', () => {
    const first = [{ id: 'model', name: 'Model', type: 'select', currentValue: 'openai/gpt-5-mini', options: [] }];
    let state = acpWorkspaceReducer(initialAcpWorkspaceState, {
      type: 'ready',
      agentName: 'contenox',
      workspaceConfigOptions: first,
      promptCapabilities: {},
    });
    expect(state.workspaceConfigOptions).toEqual(first);

    // A reconnect re-reads them (runtime model list may have changed).
    const second = [{ id: 'model', name: 'Model', type: 'select', currentValue: 'anthropic/claude', options: [] }];
    state = acpWorkspaceReducer(state, {
      type: 'ready',
      agentName: 'contenox',
      workspaceConfigOptions: second,
      promptCapabilities: {},
    });
    expect(state.workspaceConfigOptions).toEqual(second);
  });

  it('ready carries the agent\'s prompt capabilities (image gating) and a later ready replaces them', () => {
    let state = acpWorkspaceReducer(initialAcpWorkspaceState, {
      type: 'ready',
      agentName: 'contenox',
      workspaceConfigOptions: [],
      promptCapabilities: { image: true },
    });
    expect(state.promptCapabilities).toEqual({ image: true });

    // A reconnect re-reads them — a differently-capable agent replaces the set.
    state = acpWorkspaceReducer(state, {
      type: 'ready',
      agentName: 'other-agent',
      workspaceConfigOptions: [],
      promptCapabilities: {},
    });
    expect(state.promptCapabilities).toEqual({});
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

  it('sessions_replaced merges onto known entries instead of clobbering them (BUG: a reconnect-triggered refresh regressing an already-learned title)', () => {
    // A session already has a live-pushed title (session_upserted, e.g. from
    // session_info_update once the first turn resolves — see the test above).
    const withTitle = acpWorkspaceReducer(initialAcpWorkspaceState, {
      type: 'session_upserted',
      session: { sessionId: 'a', cwd: '/work', title: 'Read README.md, then update its title' },
    });
    expect(withTitle.sessions[0].title).toBe('Read README.md, then update its title');

    // `refreshSessions()` (auto-triggered by every reconnect, see
    // acpWorkspaceController.ts's attemptReconnect()) re-pages session/list and
    // replaces the roster — but the server's OWN listing index can lag behind
    // the title the client already learned live, so its row for 'a' has none.
    // A bare replace previously stomped `withTitle`'s title back to undefined
    // the instant this same-membership snapshot landed, with no further live
    // event to ever restore it — the sidebar and tab strip both read this same
    // roster, so both would go back to (or stay on) the raw-id fallback.
    const refreshed = acpWorkspaceReducer(withTitle, {
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'a' }],
    });
    expect(refreshed.sessions[0]).toMatchObject({ sessionId: 'a', cwd: '/work', title: 'Read README.md, then update its title' });
  });

  it('sessions_replaced still drops a session genuinely absent from the new snapshot (merge is field-level, not membership-level)', () => {
    const withTwo = run({
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'a', title: 'Keep me' }, { sessionId: 'b' }],
    });
    const state = acpWorkspaceReducer(withTwo, { type: 'sessions_replaced', sessions: [{ sessionId: 'b' }] });
    expect(state.sessions.map(s => s.sessionId)).toEqual(['b']);
  });

  it('session_removed drops the session from the roster', () => {
    const withTwo = run({
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'a' }, { sessionId: 'b' }],
    });
    const state = acpWorkspaceReducer(withTwo, { type: 'session_removed', sessionId: 'a' });
    expect(state.sessions.map(s => s.sessionId)).toEqual(['b']);
  });

  it('carries and preserves external-agent _meta attribution across upsert + a lagging refresh', () => {
    // session/new echoes the external-agent binding in _meta (AGENT_META_KEY).
    const created = acpWorkspaceReducer(initialAcpWorkspaceState, {
      type: 'session_upserted',
      session: { sessionId: 'ext', cwd: '/', _meta: { 'contenox.agent': 'stub-bot' } },
    });
    expect(created.sessions[0]._meta).toEqual({ 'contenox.agent': 'stub-bot' });

    // A session/list page for the same session that omits _meta (lagging index)
    // must NOT clear the already-known agent binding — same merge rule as title.
    const refreshed = acpWorkspaceReducer(created, {
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'ext' }],
    });
    expect(refreshed.sessions[0]._meta).toEqual({ 'contenox.agent': 'stub-bot' });

    // A session first SEEN via session/list (reloaded external session) picks up
    // its _meta straight from the listing.
    const listed = acpWorkspaceReducer(initialAcpWorkspaceState, {
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'ext2', _meta: { 'contenox.agent': 'other-bot' } }],
    });
    expect(listed.sessions[0]._meta).toEqual({ 'contenox.agent': 'other-bot' });
  });

  it('preserves the adopt outcome (contenox.adopt) when a session/list page carries only contenox.agent', () => {
    // session/new echoes BOTH keys for an adopted session (adoptMeta.ts).
    const adopted = acpWorkspaceReducer(initialAcpWorkspaceState, {
      type: 'session_upserted',
      session: {
        sessionId: 'ad',
        cwd: '/',
        _meta: {
          'contenox.agent': 'chain-acp',
          'contenox.adopt': { instanceId: 'i', sessionId: 'down', controller: true },
        },
      },
    });

    // A later session/list only reconstructs contenox.agent. A wholesale replace
    // would demote the adopted chat back to ordinary; a per-key merge keeps the
    // adopt outcome (the "Übernommen"/"Beobachten" header + delete-guard read it).
    const refreshed = acpWorkspaceReducer(adopted, {
      type: 'sessions_replaced',
      sessions: [{ sessionId: 'ad', _meta: { 'contenox.agent': 'chain-acp' } }],
    });
    expect(refreshed.sessions[0]._meta).toEqual({
      'contenox.agent': 'chain-acp',
      'contenox.adopt': { instanceId: 'i', sessionId: 'down', controller: true },
    });
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

describe('acpSessionsReducer: multiplexed per-session slices', () => {
  it('routes each session_dispatch into its OWN slice, keyed by session id', () => {
    const state = runSessions(
      { type: 'session_dispatch', key: 'sess-a', action: { type: 'session_reset', sessionId: 'sess-a' } },
      { type: 'session_dispatch', key: 'sess-b', action: { type: 'session_reset', sessionId: 'sess-b' } },
      { type: 'session_dispatch', key: 'sess-a', action: { type: 'user_message_chunk', id: 'ua', text: 'to A' } },
      { type: 'session_dispatch', key: 'sess-b', action: { type: 'user_message_chunk', id: 'ub', text: 'to B' } },
    );

    expect(state.slices['sess-a'].messages['ua']).toMatchObject({ text: 'to A' });
    expect(state.slices['sess-b'].messages['ub']).toMatchObject({ text: 'to B' });
    // Isolation: neither slice sees the other's message.
    expect(state.slices['sess-a'].messages['ub']).toBeUndefined();
    expect(state.slices['sess-b'].messages['ua']).toBeUndefined();
  });

  it('auto-creates a slice from the initial session state on first dispatch to an unknown key', () => {
    const state = runSessions({ type: 'session_dispatch', key: 'sess-a', action: { type: 'plan', entries: [] } });
    expect(state.slices['sess-a']).toBeDefined();
    expect(state.slices['sess-a'].sessionId).toBeNull(); // no session_reset yet
  });

  it('session_closed drops exactly one slice and leaves the others intact', () => {
    let state = runSessions(
      { type: 'session_dispatch', key: 'sess-a', action: { type: 'session_reset', sessionId: 'sess-a' } },
      { type: 'session_dispatch', key: 'sess-b', action: { type: 'session_reset', sessionId: 'sess-b' } },
    );
    state = acpSessionsReducer(state, { type: 'session_closed', key: 'sess-a' });
    expect(state.slices['sess-a']).toBeUndefined();
    expect(state.slices['sess-b']).toBeDefined();
  });

  it('session_closed for an unknown key is a no-op (returns the same reference)', () => {
    const state = runSessions({ type: 'session_dispatch', key: 'sess-a', action: { type: 'session_reset', sessionId: 'sess-a' } });
    expect(acpSessionsReducer(state, { type: 'session_closed', key: 'nope' })).toBe(state);
  });

  it('session_focused re-points which slice selectFocusedSession returns', () => {
    let state = runSessions(
      { type: 'session_dispatch', key: 'sess-a', action: { type: 'session_reset', sessionId: 'sess-a' } },
      { type: 'session_dispatch', key: 'sess-b', action: { type: 'session_reset', sessionId: 'sess-b' } },
      { type: 'session_focused', key: 'sess-a' },
    );
    expect(selectFocusedSession(state).sessionId).toBe('sess-a');
    state = acpSessionsReducer(state, { type: 'session_focused', key: 'sess-b' });
    expect(selectFocusedSession(state).sessionId).toBe('sess-b');
  });

  it('selectFocusedSession returns the shared initial state (session-less) for the empty-chat key with no slice', () => {
    const focused = selectFocusedSession(initialAcpSessionsState);
    expect(initialAcpSessionsState.focusedKey).toBe(EMPTY_SESSION_KEY);
    expect(focused.sessionId).toBeNull();
    // A failed lazy newSession() surfaces its error on the empty-chat slice,
    // which the empty-chat view (focused on EMPTY_SESSION_KEY) then renders.
    const withError = acpSessionsReducer(initialAcpSessionsState, {
      type: 'session_dispatch',
      key: EMPTY_SESSION_KEY,
      action: { type: 'prompt_error', message: 'model is broken' },
    });
    expect(selectFocusedSession(withError).error).toBe('model is broken');
  });

  it('selectOpenSessionIds lists open sessions and excludes the reserved empty-chat key', () => {
    const state = runSessions(
      { type: 'session_dispatch', key: 'sess-a', action: { type: 'session_reset', sessionId: 'sess-a' } },
      { type: 'session_dispatch', key: EMPTY_SESSION_KEY, action: { type: 'prompt_error', message: 'x' } },
      { type: 'session_dispatch', key: 'sess-b', action: { type: 'session_reset', sessionId: 'sess-b' } },
    );
    expect(selectOpenSessionIds(state).sort()).toEqual(['sess-a', 'sess-b']);
  });
});

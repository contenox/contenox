import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Transport } from '../lib/acp';
import { acpSessionReducer, initialAcpSessionState, type AcpSessionState } from './acpSessionState';
import { createAcpWorkspaceController, type AcpWorkspaceController } from './acpWorkspaceController';
import { acpWorkspaceReducer, initialAcpWorkspaceState, type AcpWorkspaceState } from './acpWorkspaceState';

/**
 * `@testing-library/react` is not a dependency of `packages/beam` — this
 * drives `acpWorkspaceController` directly against a `Transport` double, the
 * same approach `acpSessionController`'s tests used for the single-session
 * controller it replaces.
 */

class MockTransport implements Transport {
  readonly sentRaw: string[] = [];
  private readonly messageCbs: Array<(text: string) => void> = [];
  private readonly closeCbs: Array<(err?: Error) => void> = [];
  closeCalls = 0;
  private closedFired = false;

  send(text: string): void {
    this.sentRaw.push(text);
  }
  onMessage(cb: (text: string) => void): void {
    this.messageCbs.push(cb);
  }
  onClose(cb: (err?: Error) => void): void {
    this.closeCbs.push(cb);
  }
  close(): void {
    this.closeCalls++;
  }
  feed(msg: unknown): void {
    const text = JSON.stringify(msg);
    for (const cb of this.messageCbs) cb(text);
  }
  /** Simulates the remote/underlying socket actually closing (async in real WebSocket, explicit here). */
  fireClose(err?: Error): void {
    if (this.closedFired) return;
    this.closedFired = true;
    for (const cb of this.closeCbs) cb(err);
  }
  get sent(): Array<Record<string, unknown>> {
    return this.sentRaw.map(t => JSON.parse(t) as Record<string, unknown>);
  }
  lastSent(): Record<string, unknown> {
    const raw = this.sentRaw.at(-1);
    if (!raw) throw new Error('MockTransport: nothing sent yet');
    return JSON.parse(raw) as Record<string, unknown>;
  }
}

async function flushMicrotasks(): Promise<void> {
  await new Promise(resolve => setTimeout(resolve, 0));
}

function makeStore<S, A>(reducer: (s: S, a: A) => S, initial: S) {
  let state = initial;
  return {
    dispatch: (action: A) => {
      state = reducer(state, action);
    },
    get state() {
      return state;
    },
  };
}

interface Harness {
  controller: AcpWorkspaceController;
  wsStore: ReturnType<typeof makeStore<AcpWorkspaceState, Parameters<typeof acpWorkspaceReducer>[1]>>;
  sessStore: ReturnType<typeof makeStore<AcpSessionState, Parameters<typeof acpSessionReducer>[1]>>;
  transports: MockTransport[];
}

function setup(): Harness {
  const transports: MockTransport[] = [];
  const wsStore = makeStore(acpWorkspaceReducer, initialAcpWorkspaceState);
  const sessStore = makeStore(acpSessionReducer, initialAcpSessionState);
  const controller = createAcpWorkspaceController(
    {
      createTransport: () => {
        const t = new MockTransport();
        transports.push(t);
        return t;
      },
      cwd: '/work',
    },
    wsStore.dispatch,
    sessStore.dispatch,
  );
  return { controller, wsStore, sessStore, transports };
}

/** Drives `connect()` to completion against `transports[0]`: initialize + one empty session/list page. */
async function connectReady(h: Harness): Promise<void> {
  const p = h.controller.connect();
  await flushMicrotasks();
  const initReq = h.transports[0].lastSent();
  h.transports[0].feed({ jsonrpc: '2.0', id: initReq.id, result: { protocolVersion: 1, agentInfo: { name: 'contenox' } } });
  await flushMicrotasks();
  const listReq = h.transports[0].lastSent();
  h.transports[0].feed({ jsonrpc: '2.0', id: listReq.id, result: { sessions: [] } });
  await p;
}

/** Drives `newSession()` to completion against `transports[0]`, returning the minted id. */
async function createSession(h: Harness, sessionId: string): Promise<string> {
  const p = h.controller.newSession();
  await flushMicrotasks();
  const req = h.transports[0].lastSent();
  expect(req.method).toBe('session/new');
  h.transports[0].feed({ jsonrpc: '2.0', id: req.id, result: { sessionId } });
  await p;
  return sessionId;
}

afterEach(() => {
  vi.useRealTimers();
});

describe('acpWorkspaceController: connect()', () => {
  it('initializes then lists sessions, transitioning connecting -> ready', async () => {
    const h = setup();
    expect(h.wsStore.state.status).toBe('connecting');
    await connectReady(h);
    expect(h.wsStore.state.status).toBe('ready');
    expect(h.wsStore.state.agentName).toBe('contenox');
    expect(h.wsStore.state.sessions).toEqual([]);
  });

  it('is idempotent: concurrent callers share one initialize call', async () => {
    const h = setup();
    const p1 = h.controller.connect();
    const p2 = h.controller.connect();
    await flushMicrotasks();
    expect(h.transports).toHaveLength(1);
    expect(h.transports[0].sent.filter(f => f.method === 'initialize')).toHaveLength(1);

    const initReq = h.transports[0].lastSent();
    h.transports[0].feed({ jsonrpc: '2.0', id: initReq.id, result: { protocolVersion: 1 } });
    await flushMicrotasks();
    const listReq = h.transports[0].lastSent();
    h.transports[0].feed({ jsonrpc: '2.0', id: listReq.id, result: { sessions: [] } });
    await Promise.all([p1, p2]);
    expect(h.wsStore.state.status).toBe('ready');
  });

  it('maps a -32000 auth_required error to setup_required (terminal — never mapped to the generic error status)', async () => {
    const h = setup();
    const p = h.controller.connect();
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, error: { code: -32000, message: 'contenox is not configured yet' } });
    await p;
    expect(h.wsStore.state.status).toBe('setup_required');
    expect(h.wsStore.state.error).toContain('not configured');
  });

  it('maps any other error to the generic error status', async () => {
    const h = setup();
    const p = h.controller.connect();
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, error: { code: -32603, message: 'boom' } });
    await p;
    expect(h.wsStore.state.status).toBe('error');
    expect(h.wsStore.state.error).toBe('boom');
  });

  it('paginates session/list to completion', async () => {
    const h = setup();
    const p = h.controller.connect();
    await flushMicrotasks();
    h.transports[0].feed({ jsonrpc: '2.0', id: h.transports[0].lastSent().id, result: { protocolVersion: 1 } });
    await flushMicrotasks();

    const page1 = h.transports[0].lastSent();
    expect(page1.params).toEqual({});
    h.transports[0].feed({ jsonrpc: '2.0', id: page1.id, result: { sessions: [{ sessionId: 'a' }], nextCursor: 'cursor-1' } });
    await flushMicrotasks();

    const page2 = h.transports[0].lastSent();
    expect(page2.params).toEqual({ cursor: 'cursor-1' });
    h.transports[0].feed({ jsonrpc: '2.0', id: page2.id, result: { sessions: [{ sessionId: 'b' }] } });
    await p;

    expect(h.wsStore.state.sessions.map(s => s.sessionId).sort()).toEqual(['a', 'b']);
  });
});

describe('acpWorkspaceController: newSession() is lazy (D5)', () => {
  it('connect() never creates a session automatically', async () => {
    const h = setup();
    await connectReady(h);
    expect(h.wsStore.state.activeSessionId).toBeNull();
    expect(h.transports[0].sent.some(f => f.method === 'session/new')).toBe(false);
  });

  it('newSession() creates, subscribes, and activates the new session', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');

    expect(h.wsStore.state.activeSessionId).toBe('sess-a');
    expect(h.wsStore.state.sessions.map(s => s.sessionId)).toContain('sess-a');
    expect(h.sessStore.state.sessionId).toBe('sess-a');

    // The standing subscription (not a per-prompt handler) routes live updates.
    h.transports[0].feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: { sessionId: 'sess-a', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'hi' }, messageId: 'm1' } },
    });
    await flushMicrotasks();
    expect(h.sessStore.state.messages['m1']).toMatchObject({ text: 'hi' });
  });

  it('applies configOptions carried inline on the session/new response (no separate notification arrives for a fresh session)', async () => {
    const h = setup();
    await connectReady(h);

    const p = h.controller.newSession();
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    const configOptions = [{ id: 'think', name: 'Think', type: 'boolean', currentValue: 'false', options: [] }];
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, result: { sessionId: 'sess-a', configOptions } });
    await p;

    expect(h.sessStore.state.configOptions).toEqual(configOptions);
  });
});

describe('acpWorkspaceController: openSession() switching', () => {
  it('subscribes BEFORE session/load resolves, so replay notifications land in the reducer before the response settles', async () => {
    const h = setup();
    await connectReady(h);

    const openPromise = h.controller.openSession('sess-b');
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    expect(req.method).toBe('session/load');
    expect((req.params as Record<string, unknown>).sessionId).toBe('sess-b');
    expect((req.params as Record<string, unknown>).cwd).toBe('/work');

    // Replay reaches the wire before the session/load response — matches
    // acpsvc/session.go's replayMessages, which runs synchronously inside
    // the handler before the RPC response is written.
    h.transports[0].feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-b',
        update: { sessionUpdate: 'user_message_chunk', content: { type: 'text', text: 'hi' }, messageId: 'replay-0' },
      },
    });
    await flushMicrotasks();
    expect(h.sessStore.state.messages['replay-0']).toMatchObject({ role: 'user', text: 'hi' });

    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, result: {} });
    await openPromise;
    expect(h.sessStore.state.sessionId).toBe('sess-b');
    expect(h.wsStore.state.activeSessionId).toBe('sess-b');
  });

  it('closes the previously open session and rejects its pending permission with outcome cancelled', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');

    h.transports[0].feed({
      jsonrpc: '2.0',
      id: 'perm-1',
      method: 'session/request_permission',
      params: { sessionId: 'sess-a', toolCall: { toolCallId: 'tc-1' }, options: [{ optionId: 'x', name: 'X', kind: 'allow_once' }] },
    });
    await flushMicrotasks();
    expect(h.sessStore.state.pendingPermission).not.toBeNull();

    const openPromise = h.controller.openSession('sess-b');
    await flushMicrotasks();
    // Not lastSent(): the permission rejection's "cancelled" response is only
    // sent once its promise's .catch runs as a microtask, which can land
    // after the (synchronously-sent) session/load request — search by method
    // rather than assume send order between the two.
    const loadReq = h.transports[0].sent.find(f => f.method === 'session/load')!;
    expect(loadReq).toBeDefined();
    h.transports[0].feed({ jsonrpc: '2.0', id: loadReq.id, result: {} });
    await openPromise;

    const permResponse = h.transports[0].sent.find(f => f.id === 'perm-1');
    expect(permResponse).toMatchObject({ result: { outcome: { outcome: 'cancelled' } } });

    const closeReq = h.transports[0].sent.find(f => f.method === 'session/close');
    expect(closeReq).toBeDefined();
    expect((closeReq!.params as Record<string, unknown>).sessionId).toBe('sess-a');
  });

  it('is a no-op when opening the session that is already active', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');
    const sentBefore = h.transports[0].sentRaw.length;

    await h.controller.openSession('sess-a');
    expect(h.transports[0].sentRaw.length).toBe(sentBefore);
  });

  it('applies configOptions carried inline on the session/load response', async () => {
    const h = setup();
    await connectReady(h);

    const openPromise = h.controller.openSession('sess-b');
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    const configOptions = [{ id: 'model', name: 'Model', type: 'string', currentValue: 'openvino/qwen2.5-coder-0.5b-ov', options: [] }];
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, result: { configOptions } });
    await openPromise;

    expect(h.sessStore.state.configOptions).toEqual(configOptions);
  });
});

describe('acpWorkspaceController: openSession() explicit outcome signal (replaces the deleted notFound heuristic)', () => {
  it('sets sessionLoadState to not_found on an invalidParams failure (acpsvc/session.go maps unknown session to -32602)', async () => {
    const h = setup();
    await connectReady(h);

    expect(h.wsStore.state.sessionLoadState).toBe('ready');
    const openPromise = h.controller.openSession('missing');
    await flushMicrotasks();
    expect(h.wsStore.state.sessionLoadState).toBe('loading');

    const req = h.transports[0].lastSent();
    expect(req.method).toBe('session/load');
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, error: { code: -32602, message: 'load session "missing": not found' } });
    await openPromise;

    expect(h.wsStore.state.sessionLoadState).toBe('not_found');
    expect(h.wsStore.state.sessionLoadError).toBeNull();
    // No session is left "open" pointing at an id that doesn't exist.
    expect(h.wsStore.state.activeSessionId).toBeNull();
    expect(h.sessStore.state.sessionId).toBeNull();
    // The connection-level status is untouched — this is not a connection error.
    expect(h.wsStore.state.status).toBe('ready');
  });

  it('sets sessionLoadState to error (with message) on a non-not-found failure', async () => {
    const h = setup();
    await connectReady(h);

    const openPromise = h.controller.openSession('sess-x');
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, error: { code: -32603, message: 'internal boom' } });
    await openPromise;

    expect(h.wsStore.state.sessionLoadState).toBe('error');
    expect(h.wsStore.state.sessionLoadError).toBe('internal boom');
    expect(h.wsStore.state.status).toBe('ready');
  });

  it('a subsequent successful openSession() resets a stale not_found/error back to ready', async () => {
    const h = setup();
    await connectReady(h);

    const failedOpen = h.controller.openSession('missing');
    await flushMicrotasks();
    h.transports[0].feed({ jsonrpc: '2.0', id: h.transports[0].lastSent().id, error: { code: -32602, message: 'nope' } });
    await failedOpen;
    expect(h.wsStore.state.sessionLoadState).toBe('not_found');

    const openPromise = h.controller.openSession('sess-real');
    await flushMicrotasks();
    h.transports[0].feed({ jsonrpc: '2.0', id: h.transports[0].lastSent().id, result: {} });
    await openPromise;

    expect(h.wsStore.state.sessionLoadState).toBe('ready');
    expect(h.wsStore.state.sessionLoadError).toBeNull();
    expect(h.wsStore.state.activeSessionId).toBe('sess-real');
  });

  it('newSession() also resets a stale not_found/error back to ready', async () => {
    const h = setup();
    await connectReady(h);

    const failedOpen = h.controller.openSession('missing');
    await flushMicrotasks();
    h.transports[0].feed({ jsonrpc: '2.0', id: h.transports[0].lastSent().id, error: { code: -32602, message: 'nope' } });
    await failedOpen;
    expect(h.wsStore.state.sessionLoadState).toBe('not_found');

    await createSession(h, 'sess-fresh');
    expect(h.wsStore.state.sessionLoadState).toBe('ready');
  });
});

describe('acpWorkspaceController: deleteSession()', () => {
  it('removes the session from the roster and, if it was active, clears the active session first', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');
    expect(h.wsStore.state.activeSessionId).toBe('sess-a');

    const deletePromise = h.controller.deleteSession('sess-a');
    await flushMicrotasks();

    // session/close is fire-and-forget cleanup (see deleteSession's doc
    // comment) — deletePromise does not wait on it.
    const closeReq = h.transports[0].sent.find(f => f.method === 'session/close');
    expect(closeReq).toBeDefined();

    const delReq = h.transports[0].sent.filter(f => f.method === 'session/delete').at(-1)!;
    h.transports[0].feed({ jsonrpc: '2.0', id: delReq.id, result: {} });
    await deletePromise;

    expect(h.wsStore.state.sessions.find(s => s.sessionId === 'sess-a')).toBeUndefined();
    expect(h.wsStore.state.activeSessionId).toBeNull();
    expect(h.sessStore.state.sessionId).toBeNull();
  });
});

describe('acpWorkspaceController: sendPrompt()', () => {
  it('no-ops with no active session', async () => {
    const h = setup();
    await connectReady(h);
    const sentBefore = h.transports[0].sentRaw.length;
    h.controller.sendPrompt('hello');
    expect(h.transports[0].sentRaw.length).toBe(sentBefore);
  });

  it('sends session/prompt with no per-turn handlers — the standing subscription routes all streamed content', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');

    h.controller.sendPrompt('hello there');
    expect(h.sessStore.state.isPrompting).toBe(true);
    expect(Object.values(h.sessStore.state.messages).some(m => m.role === 'user' && m.text === 'hello there')).toBe(true);

    const req = h.transports[0].lastSent();
    expect(req.method).toBe('session/prompt');
    expect(req.params).toMatchObject({ sessionId: 'sess-a' });

    h.transports[0].feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: { sessionId: 'sess-a', update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'hi!' }, messageId: 'm1' } },
    });
    await flushMicrotasks();
    expect(h.sessStore.state.messages['m1']).toMatchObject({ text: 'hi!' });

    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await flushMicrotasks();
    expect(h.sessStore.state.isPrompting).toBe(false);
    expect(h.sessStore.state.stopReason).toBe('end_turn');
  });
});

describe('acpWorkspaceController: refreshSessions()', () => {
  it('re-pages session/list to completion and replaces the roster', async () => {
    const h = setup();
    await connectReady(h);

    const p = h.controller.refreshSessions();
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    expect(req.method).toBe('session/list');
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, result: { sessions: [{ sessionId: 'a' }, { sessionId: 'b' }] } });
    await p;

    expect(h.wsStore.state.sessions.map(s => s.sessionId).sort()).toEqual(['a', 'b']);
  });
});

describe('acpWorkspaceController: setConfigOption()', () => {
  it('sends session/set_config_option for the open session and applies the returned configOptions', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');

    const p = h.controller.setConfigOption('think', true);
    await flushMicrotasks();
    const req = h.transports[0].lastSent();
    expect(req.method).toBe('session/set_config_option');
    expect(req.params).toMatchObject({ sessionId: 'sess-a', configId: 'think', value: true });

    const configOptions = [{ id: 'think', name: 'Think', type: 'boolean', currentValue: 'true', options: [] }];
    h.transports[0].feed({ jsonrpc: '2.0', id: req.id, result: { configOptions } });
    await p;

    expect(h.sessStore.state.configOptions).toEqual(configOptions);
  });
});

describe('acpWorkspaceController: reconnect supervisor (D2)', () => {
  it('does not attempt reconnection before the 1s backoff elapses, then does', async () => {
    const h = setup();
    await connectReady(h);
    vi.useFakeTimers();

    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);
    expect(h.wsStore.state.status).toBe('reconnecting');

    await vi.advanceTimersByTimeAsync(999);
    expect(h.transports).toHaveLength(1);

    await vi.advanceTimersByTimeAsync(1);
    expect(h.transports).toHaveLength(2);
    expect(h.transports[1].sent.some(f => f.method === 'initialize')).toBe(true);
  });

  it('gives up after 8 failed attempts with exponential backoff capped at 30s, then goes disconnected', async () => {
    const h = setup();
    await connectReady(h);
    vi.useFakeTimers();

    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);

    const expectedDelays = [1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000];
    for (let i = 0; i < expectedDelays.length; i++) {
      await vi.advanceTimersByTimeAsync(expectedDelays[i]);
      expect(h.transports).toHaveLength(i + 2);
      h.transports[i + 1].fireClose();
      await vi.advanceTimersByTimeAsync(0);
    }

    expect(h.wsStore.state.status).toBe('disconnected');
  });

  it('resumes the active session (transcript kept client-side) when session/resume succeeds', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');
    h.sessStore.dispatch({ type: 'message_chunk', id: 'a1', text: 'before the drop' });
    vi.useFakeTimers();

    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);
    expect(h.sessStore.state.connectionBanner).toBe('disconnected');

    await vi.advanceTimersByTimeAsync(1000);
    expect(h.transports).toHaveLength(2);
    const initReq = h.transports[1].lastSent();
    h.transports[1].feed({ jsonrpc: '2.0', id: initReq.id, result: { protocolVersion: 1, agentInfo: { name: 'contenox' } } });
    await vi.advanceTimersByTimeAsync(0);

    const resumeReq = h.transports[1].lastSent();
    expect(resumeReq.method).toBe('session/resume');
    expect((resumeReq.params as Record<string, unknown>).sessionId).toBe('sess-a');
    const configOptions = [{ id: 'think', name: 'Think', type: 'boolean', currentValue: 'true', options: [] }];
    h.transports[1].feed({ jsonrpc: '2.0', id: resumeReq.id, result: { configOptions } });
    await vi.advanceTimersByTimeAsync(0);

    // Resume keeps the client-side transcript — no session_reset happened.
    expect(h.sessStore.state.messages['a1']).toMatchObject({ text: 'before the drop' });
    expect(h.sessStore.state.connectionBanner).toBe('resumed');
    expect(h.wsStore.state.status).toBe('ready');
    // session/resume's response carries configOptions inline too (same as
    // session/new / session/load) — applied here rather than waiting on a
    // session/update notification that may never come.
    expect(h.sessStore.state.configOptions).toEqual(configOptions);

    const listReq = h.transports[1].lastSent();
    expect(listReq.method).toBe('session/list');
    h.transports[1].feed({ jsonrpc: '2.0', id: listReq.id, result: { sessions: [{ sessionId: 'sess-a' }] } });
    await vi.advanceTimersByTimeAsync(0);
  });

  it('falls back to a full session/load replay when session/resume fails', async () => {
    const h = setup();
    await connectReady(h);
    await createSession(h, 'sess-a');
    h.sessStore.dispatch({ type: 'message_chunk', id: 'a1', text: 'before the drop' });
    vi.useFakeTimers();

    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(1000);

    const initReq = h.transports[1].lastSent();
    h.transports[1].feed({ jsonrpc: '2.0', id: initReq.id, result: { protocolVersion: 1, agentInfo: { name: 'contenox' } } });
    await vi.advanceTimersByTimeAsync(0);

    const resumeReq = h.transports[1].lastSent();
    expect(resumeReq.method).toBe('session/resume');
    h.transports[1].feed({ jsonrpc: '2.0', id: resumeReq.id, error: { code: -32602, message: 'unknown session' } });
    await vi.advanceTimersByTimeAsync(0);

    // Fallback to session/load — the client-side transcript is cleared for a
    // clean server-side replay.
    expect(h.sessStore.state.messages['a1']).toBeUndefined();
    const loadReq = h.transports[1].lastSent();
    expect(loadReq.method).toBe('session/load');
    expect((loadReq.params as Record<string, unknown>).sessionId).toBe('sess-a');

    h.transports[1].feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-a',
        update: { sessionUpdate: 'user_message_chunk', content: { type: 'text', text: 'replayed' }, messageId: 'replay-0' },
      },
    });
    await vi.advanceTimersByTimeAsync(0);
    expect(h.sessStore.state.messages['replay-0']).toMatchObject({ text: 'replayed' });

    h.transports[1].feed({ jsonrpc: '2.0', id: loadReq.id, result: {} });
    await vi.advanceTimersByTimeAsync(0);
    expect(h.sessStore.state.connectionBanner).toBe('resumed');

    const listReq = h.transports[1].lastSent();
    h.transports[1].feed({ jsonrpc: '2.0', id: listReq.id, result: { sessions: [{ sessionId: 'sess-a' }] } });
    await vi.advanceTimersByTimeAsync(0);
  });

  it('maps -32000 auth_required during reconnect to setup_required and stops retrying', async () => {
    const h = setup();
    await connectReady(h);
    vi.useFakeTimers();

    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);
    await vi.advanceTimersByTimeAsync(1000);

    const initReq = h.transports[1].lastSent();
    h.transports[1].feed({ jsonrpc: '2.0', id: initReq.id, error: { code: -32000, message: 'contenox is not configured yet' } });
    await vi.advanceTimersByTimeAsync(0);

    expect(h.wsStore.state.status).toBe('setup_required');

    // No further attempts scheduled, even given a long time.
    await vi.advanceTimersByTimeAsync(120_000);
    expect(h.transports).toHaveLength(2);
  });

  it('does not reconnect after dispose()', async () => {
    const h = setup();
    await connectReady(h);
    vi.useFakeTimers();

    h.controller.dispose();
    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(120_000);

    expect(h.transports).toHaveLength(1);
    expect(h.wsStore.state.status).toBe('ready'); // untouched by the post-dispose close
  });
});

describe('acpWorkspaceController: reconnect() (manual retry, D2 follow-up)', () => {
  it('retries immediately (no backoff wait) from disconnected, and succeeds', async () => {
    const h = setup();
    await connectReady(h);
    vi.useFakeTimers();

    // Exhaust the automatic supervisor, mirroring the "gives up after 8
    // attempts" test above.
    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);
    const delays = [1000, 2000, 4000, 8000, 16000, 30000, 30000, 30000];
    for (let i = 0; i < delays.length; i++) {
      await vi.advanceTimersByTimeAsync(delays[i]);
      h.transports[i + 1].fireClose();
      await vi.advanceTimersByTimeAsync(0);
    }
    expect(h.wsStore.state.status).toBe('disconnected');
    const transportCountBeforeManualRetry = h.transports.length;

    const reconnectPromise = h.controller.reconnect();
    await vi.advanceTimersByTimeAsync(0);
    // A brand-new transport was opened immediately — no 1s+ wait required.
    expect(h.transports).toHaveLength(transportCountBeforeManualRetry + 1);
    expect(h.wsStore.state.status).toBe('reconnecting');

    const latest = h.transports.at(-1)!;
    const initReq = latest.lastSent();
    expect(initReq.method).toBe('initialize');
    latest.feed({ jsonrpc: '2.0', id: initReq.id, result: { protocolVersion: 1, agentInfo: { name: 'contenox' } } });
    await vi.advanceTimersByTimeAsync(0);
    const listReq = latest.lastSent();
    expect(listReq.method).toBe('session/list');
    latest.feed({ jsonrpc: '2.0', id: listReq.id, result: { sessions: [] } });
    await reconnectPromise;

    expect(h.wsStore.state.status).toBe('ready');
  });

  it('cancels a pending automatic backoff timer and retries right away', async () => {
    const h = setup();
    await connectReady(h);
    vi.useFakeTimers();

    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);
    expect(h.wsStore.state.status).toBe('reconnecting');
    // Still mid-backoff: the automatic attempt is not due for another ~1s.

    const reconnectPromise = h.controller.reconnect();
    await vi.advanceTimersByTimeAsync(0);
    expect(h.transports).toHaveLength(2); // fired immediately, not after the scheduled 1s

    const latest = h.transports[1];
    latest.feed({ jsonrpc: '2.0', id: latest.lastSent().id, result: { protocolVersion: 1 } });
    await vi.advanceTimersByTimeAsync(0);
    latest.feed({ jsonrpc: '2.0', id: latest.lastSent().id, result: { sessions: [] } });
    await reconnectPromise;
    expect(h.wsStore.state.status).toBe('ready');

    // The original (now-superseded) backoff timer must not fire a second,
    // redundant reconnect attempt later.
    await vi.advanceTimersByTimeAsync(30_000);
    expect(h.transports).toHaveLength(2);
  });

  it('re-enters the exponential backoff supervisor if the manual attempt itself fails', async () => {
    const h = setup();
    await connectReady(h);
    vi.useFakeTimers();

    h.transports[0].fireClose();
    await vi.advanceTimersByTimeAsync(0);

    void h.controller.reconnect();
    await vi.advanceTimersByTimeAsync(0);
    expect(h.transports).toHaveLength(2);
    h.transports[1].fireClose();
    await vi.advanceTimersByTimeAsync(0);

    // Failing right away schedules attempt index 1 next (2s), not 0 again —
    // "resets the attempt counter" means the manual call itself is attempt 0,
    // not that every subsequent automatic failure restarts from scratch.
    await vi.advanceTimersByTimeAsync(1999);
    expect(h.transports).toHaveLength(2);
    await vi.advanceTimersByTimeAsync(1);
    expect(h.transports).toHaveLength(3);
  });

  it('rejects when called after dispose()', async () => {
    const h = setup();
    await connectReady(h);
    h.controller.dispose();
    await expect(h.controller.reconnect()).rejects.toThrow('disposed');
  });
});

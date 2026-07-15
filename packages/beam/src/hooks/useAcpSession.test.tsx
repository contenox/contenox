import { describe, expect, it } from 'vitest';
import { AcpClient } from '../lib/acp';
import type { Transport } from '../lib/acp';
import { createAcpSessionController } from './acpSessionController';
import { acpSessionReducer, initialAcpSessionState, type AcpSessionState } from './acpSessionState';

/**
 * `@testing-library/react` is not a dependency of `packages/beam` (checked
 * `package.json` — no `@testing-library/*` entry, and none is hoisted for
 * this package). Per the task's sanctioned fallback, this drives the pure
 * state-transition logic instead: `useAcpSession.ts` itself is a thin
 * `useReducer` + `useEffect` shell (see the file) with zero branching of its
 * own, so exercising `acpSessionController` (the orchestration) wired to
 * `acpSessionReducer` (the state) through a `Transport` double covers the
 * same behavior the hook exposes, without mounting a component.
 */

class MockTransport implements Transport {
  readonly sentRaw: string[] = [];
  private readonly messageCbs: Array<(text: string) => void> = [];
  private readonly closeCbs: Array<(err?: Error) => void> = [];
  closeCalls = 0;

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

  fireClose(err?: Error): void {
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

/** Mimics `useReducer`: a mutable box updated by dispatching through the real reducer. */
function makeStore() {
  let state: AcpSessionState = initialAcpSessionState;
  return {
    dispatch: (action: Parameters<typeof acpSessionReducer>[1]) => {
      state = acpSessionReducer(state, action);
    },
    get state() {
      return state;
    },
  };
}

function setUpConnectedController() {
  const transport = new MockTransport();
  const client = new AcpClient(transport);
  const store = makeStore();
  const controller = createAcpSessionController(client, store.dispatch);
  return { transport, client, store, controller };
}

async function connect(transport: MockTransport, controller: ReturnType<typeof createAcpSessionController>) {
  const connectPromise = controller.connect('/');
  await flushMicrotasks();
  transport.feed({ jsonrpc: '2.0', id: 1, result: { protocolVersion: 1, agentInfo: { name: 'demo-agent' } } });
  await flushMicrotasks();
  transport.feed({ jsonrpc: '2.0', id: 2, result: { sessionId: 'sess-1' } });
  await connectPromise;
}

describe('acpSessionController + acpSessionReducer: connect', () => {
  it('transitions connecting -> ready with the session id and agent name', async () => {
    const { transport, store, controller } = setUpConnectedController();
    expect(store.state.status).toBe('connecting');

    await connect(transport, controller);

    expect(store.state.status).toBe('ready');
    expect(store.state.sessionId).toBe('sess-1');
    expect(store.state.agentName).toBe('demo-agent');
    expect(store.state.error).toBeNull();
  });

  it('transitions to error on an initialize failure', async () => {
    const { transport, store, controller } = setUpConnectedController();
    const connectPromise = controller.connect('/');
    transport.feed({ jsonrpc: '2.0', id: 1, error: { code: -32603, message: 'boom' } });
    await connectPromise;

    expect(store.state.status).toBe('error');
    expect(store.state.error).toBe('boom');
  });
});

describe('acpSessionController + acpSessionReducer: a prompt turn', () => {
  it('populates messages (grouped by messageId), plan, usage, and the permission gate; respondPermission answers and clears it', async () => {
    const { transport, store, controller } = setUpConnectedController();
    await connect(transport, controller);

    controller.sendPrompt('hello there');

    // Optimistic user message + prompt-in-flight, before any server reply.
    expect(store.state.messages).toEqual([{ id: expect.any(String), role: 'user', text: 'hello there' }]);
    expect(store.state.isPrompting).toBe(true);

    const promptReq = transport.lastSent();
    expect(promptReq.method).toBe('session/prompt');
    expect((promptReq.params as Record<string, unknown>).sessionId).toBe('sess-1');

    // Two chunks sharing one server-assigned messageId accumulate into one message.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'Hi ' }, messageId: 'm1' },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'there!' }, messageId: 'm1' },
      },
    });
    await flushMicrotasks();

    const assistantMessages = store.state.messages.filter(m => m.role === 'assistant');
    expect(assistantMessages).toHaveLength(1);
    expect(assistantMessages[0].text).toBe('Hi there!');
    expect(assistantMessages[0].streaming).toBe(true);

    // Plan + usage updates.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'plan', entries: [{ content: 'do the thing', priority: 'high', status: 'in_progress' }] },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: { sessionId: 'sess-1', update: { sessionUpdate: 'usage_update', used: 42, size: 1000 } },
    });
    await flushMicrotasks();

    expect(store.state.plan).toEqual([{ content: 'do the thing', priority: 'high', status: 'in_progress' }]);
    expect(store.state.usage).toEqual({ used: 42, size: 1000 });

    // A tool call, created then advanced in place (not a second card).
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'tool_call', toolCallId: 'tc-1', title: 'Run ls', kind: 'execute', status: 'pending' },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'tool_call_update', toolCallId: 'tc-1', status: 'completed' },
      },
    });
    await flushMicrotasks();

    expect(store.state.toolCallOrder).toEqual(['tc-1']);
    expect(store.state.toolCalls['tc-1']).toMatchObject({ title: 'Run ls', kind: 'execute', status: 'completed' });

    // The permission gate: the agent (server) sends a request, not a notification.
    transport.feed({
      jsonrpc: '2.0',
      id: 'perm-1',
      method: 'session/request_permission',
      params: {
        sessionId: 'sess-1',
        toolCall: { toolCallId: 'tc-2', title: 'rm -rf /tmp/x', kind: 'delete' },
        options: [
          { optionId: 'allow', name: 'Allow once', kind: 'allow_once' },
          { optionId: 'reject', name: 'Reject', kind: 'reject_once' },
        ],
      },
    });
    await flushMicrotasks();

    expect(store.state.pendingPermission).not.toBeNull();
    expect(store.state.pendingPermission?.toolCall.title).toBe('rm -rf /tmp/x');
    expect(store.state.pendingPermission?.options).toHaveLength(2);

    controller.respondPermission('allow');
    await flushMicrotasks();

    expect(store.state.pendingPermission).toBeNull();
    const permissionResponse = transport.sent.find(f => f.id === 'perm-1');
    expect(permissionResponse).toMatchObject({
      jsonrpc: '2.0',
      id: 'perm-1',
      result: { outcome: { outcome: 'selected', optionId: 'allow' } },
    });

    // The turn ends: reply to the outstanding session/prompt call.
    transport.feed({ jsonrpc: '2.0', id: promptReq.id, result: { stopReason: 'end_turn' } });
    await flushMicrotasks();

    expect(store.state.isPrompting).toBe(false);
    expect(store.state.messages.find(m => m.role === 'assistant')?.streaming).toBe(false);
  });

  it('ignores a second sendPrompt while one is already in flight', async () => {
    const { transport, store, controller } = setUpConnectedController();
    await connect(transport, controller);

    controller.sendPrompt('first');
    const sentAfterFirst = transport.sentRaw.length;
    controller.sendPrompt('second');

    expect(transport.sentRaw.length).toBe(sentAfterFirst);
    expect(store.state.messages.filter(m => m.role === 'user')).toHaveLength(1);
  });

  it('rejects a pending permission request on dispose instead of hanging', async () => {
    const { transport, store, controller } = setUpConnectedController();
    await connect(transport, controller);

    controller.sendPrompt('hello');
    transport.feed({
      jsonrpc: '2.0',
      id: 'perm-2',
      method: 'session/request_permission',
      params: { sessionId: 'sess-1', toolCall: { toolCallId: 'tc-3' }, options: [{ optionId: 'a', name: 'A', kind: 'allow_once' }] },
    });
    await flushMicrotasks();
    expect(store.state.pendingPermission).not.toBeNull();

    controller.dispose();
    await flushMicrotasks();

    const permissionResponse = transport.sent.find(f => f.id === 'perm-2');
    expect(permissionResponse).toMatchObject({ result: { outcome: { outcome: 'cancelled' } } });
  });
});

import { describe, expect, it, vi } from 'vitest';
import { AcpClient, type PromptHandlers, type SessionEventHandlers, type ToolCallEvent } from './client';
import { createAcpClient, type AcpCapabilityProvider } from './clientFactory';
import type { Transport } from './transport';

/**
 * A `Transport` double that lets a test feed inbound frames (as if the server
 * sent them) and inspect outbound frames the client sent — no network, no
 * timers beyond microtask flushing.
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

  /** Simulate the server sending one JSON-RPC frame. */
  feed(msg: unknown): void {
    const text = JSON.stringify(msg);
    for (const cb of this.messageCbs) cb(text);
  }

  /** Simulate the transport closing (remotely or locally). */
  fireClose(err?: Error): void {
    for (const cb of this.closeCbs) cb(err);
  }

  get sent(): unknown[] {
    return this.sentRaw.map((t) => JSON.parse(t));
  }

  lastSent(): Record<string, unknown> {
    const raw = this.sentRaw.at(-1);
    if (!raw) throw new Error('MockTransport: nothing sent yet');
    return JSON.parse(raw) as Record<string, unknown>;
  }
}

/** Flush pending microtasks (promise chains inside the client's async handlers). */
async function flushMicrotasks(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe('AcpClient: session/new _meta (external-agent binding)', () => {
  it('forwards a passed meta as the session/new `_meta` param (external agent)', () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    void client.newSession('/work', [], { 'contenox.agent': 'stub-bot' });

    const req = transport.lastSent();
    expect(req.method).toBe('session/new');
    expect(req.params).toEqual({
      cwd: '/work',
      mcpServers: [],
      _meta: { 'contenox.agent': 'stub-bot' },
    });
  });

  it('omits `_meta` entirely when none is passed (native session unchanged)', () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    void client.newSession('/work');

    const req = transport.lastSent();
    expect(req.params).toEqual({ cwd: '/work', mcpServers: [] });
  });
});

describe('AcpClient: request/response id correlation', () => {
  it('correlates the initialize request with its response', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const resultPromise = client.initialize();

    expect(transport.sentRaw).toHaveLength(1);
    const req = transport.lastSent();
    expect(req.method).toBe('initialize');
    expect(req.jsonrpc).toBe('2.0');
    expect(req.id).toBe(1);
    expect((req.params as Record<string, unknown>).protocolVersion).toBe(1);

    transport.feed({
      jsonrpc: '2.0',
      id: req.id,
      result: { protocolVersion: 1, agentInfo: { name: 'demo-agent' } },
    });

    await expect(resultPromise).resolves.toEqual({
      protocolVersion: 1,
      agentInfo: { name: 'demo-agent' },
    });
  });

  it('routes concurrent requests to the correct caller regardless of response order', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const newSessionPromise = client.newSession('/work/a');
    const listSessionsPromise = client.listSessions();

    expect(transport.sentRaw).toHaveLength(2);
    const [newSessionReq, listSessionsReq] = transport.sent as Array<Record<string, unknown>>;
    expect(newSessionReq.method).toBe('session/new');
    expect(newSessionReq.id).toBe(1);
    expect(listSessionsReq.method).toBe('session/list');
    expect(listSessionsReq.id).toBe(2);

    // Respond out of order: the second request's response arrives first.
    transport.feed({ jsonrpc: '2.0', id: 2, result: { sessions: [] } });
    transport.feed({
      jsonrpc: '2.0',
      id: 1,
      result: { sessionId: 'sess-a' },
    });

    await expect(listSessionsPromise).resolves.toEqual({ sessions: [] });
    await expect(newSessionPromise).resolves.toEqual({ sessionId: 'sess-a' });
  });

  it('rejects the caller with an AcpError on a JSON-RPC error response', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const promise = client.newSession('/work');
    const req = transport.lastSent();
    transport.feed({
      jsonrpc: '2.0',
      id: req.id,
      error: { code: -32602, message: 'invalid params' },
    });

    await expect(promise).rejects.toMatchObject({ name: 'AcpError', code: -32602, message: 'invalid params' });
  });
});

describe('AcpClient: prompt turn update routing', () => {
  it('fires handlers in arrival order and resolves stopReason on the final response', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const events: string[] = [];
    const handlers: PromptHandlers = {
      onMessageChunk: (text) => events.push(`message:${text}`),
      onThoughtChunk: (text) => events.push(`thought:${text}`),
      onPlan: (entries) => events.push(`plan:${entries.map((e) => e.content).join(',')}`),
      onUsage: (usage) => events.push(`usage:${usage.used}/${usage.size}`),
      onToolCall: (event) => events.push(`tool:${event.updateKind}:${event.toolCallId}:${event.status}`),
      onAvailableCommands: (commands) => events.push(`commands:${commands.map((c) => c.name).join(',')}`),
    };

    const promptPromise = client.prompt(
      'sess-1',
      [{ type: 'text', text: 'reply with one word: ready' }],
      handlers,
    );

    const req = transport.lastSent();
    expect(req.method).toBe('session/prompt');
    expect((req.params as Record<string, unknown>).sessionId).toBe('sess-1');

    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'agent_thought_chunk', content: { type: 'text', text: 'thinking...' } },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'ready' } },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: {
          sessionUpdate: 'tool_call',
          toolCallId: 'tc-1',
          status: 'in_progress',
        },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: {
          sessionUpdate: 'plan',
          entries: [{ content: 'do the thing', priority: 'medium', status: 'pending' }],
        },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-1',
        update: { sessionUpdate: 'usage_update', used: 42, size: 1000 },
      },
    });
    // Updates for a different / inactive session must not leak into this turn's handlers.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-other',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'ignore me' } },
      },
    });

    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });

    const result = await promptPromise;
    expect(result.stopReason).toBe('end_turn');
    expect(events).toEqual([
      'thought:thinking...',
      'message:ready',
      'tool:tool_call:tc-1:in_progress',
      'plan:do the thing',
      'usage:42/1000',
    ]);
  });

  it('advances a tool card in place via tool_call_update using the same handler', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const toolEvents: ToolCallEvent[] = [];
    const promptPromise = client.prompt('sess-2', [{ type: 'text', text: 'run it' }], {
      onToolCall: (event) => toolEvents.push(event),
    });
    const req = transport.lastSent();

    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-2',
        update: { sessionUpdate: 'tool_call', toolCallId: 'tc-9', status: 'pending', title: 'ls' },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-2',
        update: { sessionUpdate: 'tool_call_update', toolCallId: 'tc-9', status: 'completed' },
      },
    });
    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await promptPromise;

    expect(toolEvents).toHaveLength(2);
    expect(toolEvents[0]).toMatchObject({ updateKind: 'tool_call', toolCallId: 'tc-9', status: 'pending' });
    expect(toolEvents[1]).toMatchObject({ updateKind: 'tool_call_update', toolCallId: 'tc-9', status: 'completed' });
  });

  it('stops routing updates for a session once its prompt turn has resolved', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const onMessageChunk = vi.fn();

    const promptPromise = client.prompt('sess-3', [{ type: 'text', text: 'hi' }], { onMessageChunk });
    const req = transport.lastSent();
    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await promptPromise;

    // A stray update after the turn resolved (no active handler registered) is dropped, not thrown.
    expect(() =>
      transport.feed({
        jsonrpc: '2.0',
        method: 'session/update',
        params: {
          sessionId: 'sess-3',
          update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'late' } },
        },
      }),
    ).not.toThrow();
    expect(onMessageChunk).not.toHaveBeenCalled();
  });
});

describe('AcpClient: session/request_permission', () => {
  it('resolves onPermissionRequest to an optionId and sends the matching response frame', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const onPermissionRequest = vi.fn().mockResolvedValue('opt-allow-once');
    const promptPromise = client.prompt('sess-4', [{ type: 'text', text: 'rm -rf /tmp/x' }], {
      onPermissionRequest,
    });
    const promptReq = transport.lastSent();

    transport.feed({
      jsonrpc: '2.0',
      id: 99,
      method: 'session/request_permission',
      params: {
        sessionId: 'sess-4',
        toolCall: { toolCallId: 'tc-1', title: 'rm -rf /tmp/x', kind: 'execute', status: 'pending' },
        options: [
          { optionId: 'opt-allow-once', name: 'Allow once', kind: 'allow_once' },
          { optionId: 'opt-reject-once', name: 'Reject', kind: 'reject_once' },
        ],
      },
    });

    await flushMicrotasks();

    expect(onPermissionRequest).toHaveBeenCalledTimes(1);
    expect(onPermissionRequest.mock.calls[0][0]).toMatchObject({
      sessionId: 'sess-4',
      toolCall: { toolCallId: 'tc-1' },
    });

    const permissionResponseFrame = transport.sent.find(
      (m) => (m as Record<string, unknown>).id === 99,
    ) as Record<string, unknown>;
    expect(permissionResponseFrame).toEqual({
      jsonrpc: '2.0',
      id: 99,
      result: { outcome: { outcome: 'selected', optionId: 'opt-allow-once' } },
    });

    transport.feed({ jsonrpc: '2.0', id: promptReq.id, result: { stopReason: 'end_turn' } });
    await expect(promptPromise).resolves.toEqual({ stopReason: 'end_turn' });
  });

  it('responds with outcome cancelled if the permission handler rejects', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const onPermissionRequest = vi.fn().mockRejectedValue(new Error('dialog dismissed'));
    const promptPromise = client.prompt('sess-5', [{ type: 'text', text: 'do it' }], {
      onPermissionRequest,
    });
    const promptReq = transport.lastSent();

    transport.feed({
      jsonrpc: '2.0',
      id: 7,
      method: 'session/request_permission',
      params: {
        sessionId: 'sess-5',
        toolCall: { toolCallId: 'tc-2' },
        options: [{ optionId: 'opt-a', name: 'Allow', kind: 'allow_once' }],
      },
    });

    await flushMicrotasks();

    const responseFrame = transport.sent.find((m) => (m as Record<string, unknown>).id === 7) as Record<
      string,
      unknown
    >;
    expect(responseFrame).toEqual({
      jsonrpc: '2.0',
      id: 7,
      result: { outcome: { outcome: 'cancelled' } },
    });

    transport.feed({ jsonrpc: '2.0', id: promptReq.id, result: { stopReason: 'cancelled' } });
    await promptPromise;
  });

  it('answers with an error instead of hanging when no permission handler is registered', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const promptPromise = client.prompt('sess-6', [{ type: 'text', text: 'do it' }], {});
    const promptReq = transport.lastSent();

    transport.feed({
      jsonrpc: '2.0',
      id: 5,
      method: 'session/request_permission',
      params: {
        sessionId: 'sess-6',
        toolCall: { toolCallId: 'tc-3' },
        options: [{ optionId: 'opt-a', name: 'Allow', kind: 'allow_once' }],
      },
    });

    await flushMicrotasks();

    const responseFrame = transport.sent.find((m) => (m as Record<string, unknown>).id === 5) as Record<
      string,
      unknown
    >;
    expect(responseFrame.error).toMatchObject({ code: -32603 });

    transport.feed({ jsonrpc: '2.0', id: promptReq.id, result: { stopReason: 'end_turn' } });
    await promptPromise;
  });
});

describe('AcpClient: fs/* and terminal/* requests', () => {
  it('answers unsupported fs/terminal requests with a method-not-found error rather than hanging', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    void client; // constructed for its onMessage/onClose side effects only

    transport.feed({
      jsonrpc: '2.0',
      id: 1,
      method: 'fs/read_text_file',
      params: { sessionId: 'sess-1', path: '/tmp/x' },
    });
    transport.feed({
      jsonrpc: '2.0',
      id: 2,
      method: 'terminal/create',
      params: { sessionId: 'sess-1', command: 'ls' },
    });

    await flushMicrotasks();

    const responses = transport.sent as Array<Record<string, unknown>>;
    expect(responses).toHaveLength(2);
    for (const resp of responses) {
      expect(resp.result).toBeUndefined();
      expect((resp.error as Record<string, unknown>).code).toBe(-32601);
    }
  });

  it('answers a wholly unknown server->client method gracefully', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    void client; // keep the client referenced/alive for lint clarity

    transport.feed({ jsonrpc: '2.0', id: 1, method: 'something/unexpected', params: {} });
    await flushMicrotasks();

    const resp = transport.lastSent();
    expect((resp.error as Record<string, unknown>).code).toBe(-32601);
  });
});

describe('AcpClient: transport close', () => {
  it('rejects all pending calls when the transport closes', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);

    const promise = client.initialize();
    transport.fireClose(new Error('socket dropped'));

    await expect(promise).rejects.toThrow('socket dropped');
  });

  it('close() delegates to the transport', () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    client.close();
    expect(transport.closeCalls).toBe(1);
  });
});

describe('AcpClient: subscribe() out-of-turn routing', () => {
  it('delivers session/update notifications to a subscription with no prompt ever started for that session', () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const events: string[] = [];

    client.subscribe('sess-sub-none', {
      onMessageChunk: (text) => events.push(`message:${text}`),
    });

    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-sub-none',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'hello' } },
      },
    });

    expect(events).toEqual(['message:hello']);
  });

  it('delivers to a subscription registered before a prompt for that session ever starts', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const events: string[] = [];

    client.subscribe('sess-sub-before', {
      onMessageChunk: (text) => events.push(`message:${text}`),
    });

    // Arrives before any prompt() call for this session exists.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-sub-before',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'pre-turn' } },
      },
    });

    const promptPromise = client.prompt('sess-sub-before', [{ type: 'text', text: 'hi' }]);
    const req = transport.lastSent();
    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await promptPromise;

    expect(events).toEqual(['message:pre-turn']);
  });

  it('routes to the subscription instead of the per-prompt handlers while a prompt is in flight', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const subEvents: string[] = [];
    const promptEvents: string[] = [];

    client.subscribe('sess-sub-during', {
      onMessageChunk: (text) => subEvents.push(text),
    });

    const promptPromise = client.prompt('sess-sub-during', [{ type: 'text', text: 'hi' }], {
      onMessageChunk: (text) => promptEvents.push(text),
    });
    const req = transport.lastSent();

    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-sub-during',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'during turn' } },
      },
    });

    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await promptPromise;

    expect(subEvents).toEqual(['during turn']);
    expect(promptEvents).toEqual([]);
  });

  it('never delivers the same event to both an active subscription and per-prompt handlers', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const subCall = vi.fn();
    const promptCall = vi.fn();

    client.subscribe('sess-sub-nodouble', { onMessageChunk: subCall });
    const promptPromise = client.prompt('sess-sub-nodouble', [{ type: 'text', text: 'hi' }], {
      onMessageChunk: promptCall,
    });
    const req = transport.lastSent();

    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-sub-nodouble',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'only once' } },
      },
    });

    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await promptPromise;

    expect(subCall).toHaveBeenCalledTimes(1);
    // (text, messageId, image) — a plain text chunk carries no image payload.
    expect(subCall).toHaveBeenCalledWith('only once', undefined, undefined);
    expect(promptCall).not.toHaveBeenCalled();
  });

  it('delivers a post-turn session_info_update to a subscription after the prompt has resolved (matches acpsvc/prompt.go)', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const infos: Array<{ title?: string; updatedAt?: string }> = [];

    client.subscribe('sess-sub-after', { onSessionInfo: (info) => infos.push(info) });

    const promptPromise = client.prompt('sess-sub-after', [{ type: 'text', text: 'hi' }]);
    const req = transport.lastSent();
    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await promptPromise;

    // acpsvc/prompt.go sends session_info_update via libacp.AfterResponse — i.e.
    // only once the session/prompt result is already on the wire.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-sub-after',
        update: { sessionUpdate: 'session_info_update', updatedAt: '2026-07-15T00:00:00Z' },
      },
    });

    expect(infos).toEqual([{ updatedAt: '2026-07-15T00:00:00Z' }]);
  });

  it('stops delivering once unsubscribed, falling back to per-prompt handlers when a prompt is still active', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const subEvents: string[] = [];
    const promptEvents: string[] = [];

    const unsubscribe = client.subscribe('sess-unsub', {
      onMessageChunk: (text) => subEvents.push(text),
    });

    const promptPromise = client.prompt('sess-unsub', [{ type: 'text', text: 'hi' }], {
      onMessageChunk: (text) => promptEvents.push(text),
    });
    const req = transport.lastSent();

    // While both are registered, the subscription wins.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-unsub',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'via-sub' } },
      },
    });

    unsubscribe();

    // With the subscription gone but the prompt still in flight, the per-prompt
    // handlers take over — the event is not silently dropped.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-unsub',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'text', text: 'via-prompt' } },
      },
    });

    // Calling the unsubscribe function again is a no-op, not an error.
    expect(() => unsubscribe()).not.toThrow();

    transport.feed({ jsonrpc: '2.0', id: req.id, result: { stopReason: 'end_turn' } });
    await promptPromise;

    expect(subEvents).toEqual(['via-sub']);
    expect(promptEvents).toEqual(['via-prompt']);
  });

  it('routes session/request_permission to an active subscription, taking priority over per-prompt handlers', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const subPermission = vi.fn().mockResolvedValue('opt-from-sub');
    const promptPermission = vi.fn().mockResolvedValue('opt-from-prompt');

    client.subscribe('sess-perm-sub', { onPermissionRequest: subPermission });
    const promptPromise = client.prompt('sess-perm-sub', [{ type: 'text', text: 'do it' }], {
      onPermissionRequest: promptPermission,
    });
    const promptReq = transport.lastSent();

    transport.feed({
      jsonrpc: '2.0',
      id: 42,
      method: 'session/request_permission',
      params: {
        sessionId: 'sess-perm-sub',
        toolCall: { toolCallId: 'tc-perm' },
        options: [{ optionId: 'opt-from-sub', name: 'Allow', kind: 'allow_once' }],
      },
    });

    await flushMicrotasks();

    expect(subPermission).toHaveBeenCalledTimes(1);
    expect(promptPermission).not.toHaveBeenCalled();

    const responseFrame = transport.sent.find((m) => (m as Record<string, unknown>).id === 42) as Record<
      string,
      unknown
    >;
    expect(responseFrame).toEqual({
      jsonrpc: '2.0',
      id: 42,
      result: { outcome: { outcome: 'selected', optionId: 'opt-from-sub' } },
    });

    transport.feed({ jsonrpc: '2.0', id: promptReq.id, result: { stopReason: 'end_turn' } });
    await promptPromise;
  });
});

describe('AcpClient: session/load replay routing (matches acpsvc/session.go)', () => {
  it('delivers a full replay sequence to a subscription, in wire-arrival order', async () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const events: string[] = [];
    let capturedCommands: Array<{ name: string }> | undefined;

    const handlers: SessionEventHandlers = {
      onUserMessageChunk: (text, messageId) => events.push(`user:${messageId}:${text}`),
      onMessageChunk: (text, messageId) => events.push(`message:${messageId}:${text}`),
      onThoughtChunk: (text, messageId) => events.push(`thought:${messageId}:${text}`),
      onToolCall: (event) => events.push(`tool:${event.updateKind}:${event.toolCallId}:${event.status}`),
      onUsage: (usage) => events.push(`usage:${usage.used}/${usage.size}`),
      onAvailableCommands: (commands) => {
        capturedCommands = commands;
        events.push(`commands:${commands.map((c) => c.name).join(',')}`);
      },
    };
    // The caller already knows the sessionId for session/load (unlike session/new,
    // where it's minted server-side), so it can subscribe before issuing the call —
    // this is exactly what lets a subscription observe acpsvc/session.go's
    // replayMessages() notifications, which reach the wire BEFORE the session/load
    // response itself (replayMessages runs synchronously inside the handler, before
    // it returns; only available_commands_update is deferred via libacp.AfterResponse
    // until after the response).
    client.subscribe('sess-replay', handlers);

    const loadPromise = client.loadSession('sess-replay', '/work/replay');
    const req = transport.lastSent();
    expect(req.method).toBe('session/load');

    // One messageId per historical message (acpsvc/session.go: replayMessages).
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-replay',
        update: {
          sessionUpdate: 'user_message_chunk',
          content: { type: 'text', text: 'hi there' },
          messageId: 'replay-0',
        },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-replay',
        update: {
          sessionUpdate: 'agent_thought_chunk',
          content: { type: 'text', text: 'thinking about it' },
          messageId: 'replay-1',
        },
      },
    });
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-replay',
        update: {
          sessionUpdate: 'agent_message_chunk',
          content: { type: 'text', text: 'here is my answer' },
          messageId: 'replay-1',
        },
      },
    });
    // toolCallUpdateFromCall: no messageId, status forced to "completed" for replay.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-replay',
        update: {
          sessionUpdate: 'tool_call',
          toolCallId: 'tc-1',
          title: 'search: foo',
          kind: 'search',
          status: 'completed',
          rawInput: { query: 'foo' },
        },
      },
    });
    // toolCallUpdateFromResult: the tool's result message.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-replay',
        update: {
          sessionUpdate: 'tool_call_update',
          toolCallId: 'tc-1',
          status: 'completed',
          rawOutput: 'result content',
        },
      },
    });
    // sendInitialUsageUpdate, called at the end of replayMessages; used/size always
    // present on the wire for usage_update.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-replay',
        update: { sessionUpdate: 'usage_update', used: 0, size: 8000 },
      },
    });

    // The session/load JSON-RPC response reaches the wire only after the replay
    // notifications above (acpsvc/session.go emits them synchronously before
    // returning the response).
    transport.feed({
      jsonrpc: '2.0',
      id: req.id,
      result: {
        configOptions: [{ id: 'model', name: 'Model', type: 'string', currentValue: 'demo-model', options: [] }],
      },
    });
    const loadResult = await loadPromise;

    // available_commands_update is scheduled via libacp.AfterResponse — it must
    // only reach the client once the session/load result is already resolvable.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-replay',
        update: {
          sessionUpdate: 'available_commands_update',
          availableCommands: [
            { name: 'help', description: 'List the available commands.' },
            { name: 'clear', description: "Clear this session's conversation history." },
          ],
        },
      },
    });

    expect(events).toEqual([
      'user:replay-0:hi there',
      'thought:replay-1:thinking about it',
      'message:replay-1:here is my answer',
      'tool:tool_call:tc-1:completed',
      'tool:tool_call_update:tc-1:completed',
      'usage:0/8000',
      'commands:help,clear',
    ]);
    expect(loadResult.configOptions).toEqual([
      { id: 'model', name: 'Model', type: 'string', currentValue: 'demo-model', options: [] },
    ]);
    expect(capturedCommands).toHaveLength(2);
  });
});

describe('AcpClient: capability provider (clientFactory.ts)', () => {
  it("merges the capability provider's capabilities() into initialize()'s clientCapabilities", () => {
    const transport = new MockTransport();
    const provider: AcpCapabilityProvider = {
      capabilities: () => ({ terminal: true, fs: { readTextFile: true } }),
      handleRequest: vi.fn(),
    };
    const client = createAcpClient(transport, { capabilities: provider });

    void client.initialize({ session: { configOptions: { boolean: {} } } });

    const req = transport.lastSent();
    expect((req.params as Record<string, unknown>).clientCapabilities).toEqual({
      terminal: true,
      fs: { readTextFile: true },
      session: { configOptions: { boolean: {} } },
    });
  });

  it('lets explicit initialize() capabilities override the provider per top-level key', () => {
    const transport = new MockTransport();
    const provider: AcpCapabilityProvider = {
      capabilities: () => ({ terminal: true }),
      handleRequest: vi.fn(),
    };
    const client = createAcpClient(transport, { capabilities: provider });

    void client.initialize({ terminal: false });

    const req = transport.lastSent();
    expect((req.params as Record<string, unknown>).clientCapabilities).toEqual({ terminal: false });
  });

  it('sends clientCapabilities unchanged when no provider is registered', () => {
    const transport = new MockTransport();
    const client = createAcpClient(transport);

    void client.initialize({ terminal: true });

    const req = transport.lastSent();
    expect((req.params as Record<string, unknown>).clientCapabilities).toEqual({ terminal: true });
  });

  it('round-trips terminal/create through a capability provider', async () => {
    const transport = new MockTransport();
    const handleRequest = vi.fn(async (method: string, params: unknown) => {
      expect(method).toBe('terminal/create');
      expect(params).toMatchObject({ command: 'ls' });
      return { terminalId: 'term-1' };
    });
    const provider: AcpCapabilityProvider = {
      capabilities: () => ({ terminal: true }),
      handleRequest,
    };
    const client = createAcpClient(transport, { capabilities: provider });
    void client; // constructed for its onMessage side effects only

    transport.feed({
      jsonrpc: '2.0',
      id: 1,
      method: 'terminal/create',
      params: { sessionId: 'sess-1', command: 'ls' },
    });

    await flushMicrotasks();

    expect(handleRequest).toHaveBeenCalledTimes(1);
    expect(transport.lastSent()).toEqual({ jsonrpc: '2.0', id: 1, result: { terminalId: 'term-1' } });
  });

  it('still refuses terminal/create exactly as today when no provider is registered', async () => {
    const transport = new MockTransport();
    const client = createAcpClient(transport);
    void client;

    transport.feed({
      jsonrpc: '2.0',
      id: 1,
      method: 'terminal/create',
      params: { sessionId: 'sess-1', command: 'ls' },
    });

    await flushMicrotasks();

    const resp = transport.lastSent();
    expect(resp.result).toBeUndefined();
    expect((resp.error as Record<string, unknown>).code).toBe(-32601);
    expect((resp.error as Record<string, unknown>).message).toBe('not supported by this client: terminal/create');
  });

  it('falls back to the standard refusal when the provider declines the method', async () => {
    const transport = new MockTransport();
    const provider: AcpCapabilityProvider = {
      capabilities: () => ({}),
      handleRequest: vi.fn().mockRejectedValue(new Error('not implemented')),
    };
    const client = createAcpClient(transport, { capabilities: provider });
    void client;

    transport.feed({
      jsonrpc: '2.0',
      id: 1,
      method: 'terminal/create',
      params: { sessionId: 'sess-1', command: 'ls' },
    });

    await flushMicrotasks();

    const resp = transport.lastSent();
    expect((resp.error as Record<string, unknown>).code).toBe(-32601);
  });
});

describe('AcpClient: image content blocks in message chunks', () => {
  it('delivers a user_message_chunk image block as the handler image payload instead of flattening it to ""', () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const events: Array<{ text: string; messageId?: string; image?: { data: string; mimeType: string } }> = [];
    client.subscribe('sess-img', {
      onUserMessageChunk: (text, messageId, image) => events.push({ text, messageId, image }),
    });

    // Wire-fact shape: libacp's NewImageContent — type image, base64 data (no
    // data: prefix), mimeType. Seen in session/load replay of image prompts.
    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-img',
        update: {
          sessionUpdate: 'user_message_chunk',
          content: { type: 'image', data: 'aGVsbG8=', mimeType: 'image/png' },
          messageId: 'u1',
        },
      },
    });

    expect(events).toEqual([{ text: '', messageId: 'u1', image: { data: 'aGVsbG8=', mimeType: 'image/png' } }]);
  });

  it('delivers an agent_message_chunk image block the same way', () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const events: Array<{ text: string; image?: { data: string; mimeType: string } }> = [];
    client.subscribe('sess-img', {
      onMessageChunk: (text, _messageId, image) => events.push({ text, image }),
    });

    transport.feed({
      jsonrpc: '2.0',
      method: 'session/update',
      params: {
        sessionId: 'sess-img',
        update: { sessionUpdate: 'agent_message_chunk', content: { type: 'image', data: 'aW1n', mimeType: 'image/jpeg' } },
      },
    });

    expect(events).toEqual([{ text: '', image: { data: 'aW1n', mimeType: 'image/jpeg' } }]);
  });

  it('passes NO image payload for text chunks and for malformed image blocks (missing data/mimeType)', () => {
    const transport = new MockTransport();
    const client = new AcpClient(transport);
    const images: Array<unknown> = [];
    client.subscribe('sess-img', {
      onUserMessageChunk: (_text, _messageId, image) => images.push(image),
    });

    const feed = (content: Record<string, unknown>) =>
      transport.feed({
        jsonrpc: '2.0',
        method: 'session/update',
        params: { sessionId: 'sess-img', update: { sessionUpdate: 'user_message_chunk', content } },
      });

    feed({ type: 'text', text: 'plain' });
    feed({ type: 'image', mimeType: 'image/png' }); // no data
    feed({ type: 'image', data: 'aGVsbG8=' }); // no mimeType

    expect(images).toEqual([undefined, undefined, undefined]);
  });
});

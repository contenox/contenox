import { describe, expect, it, vi } from 'vitest';
import { AcpClient, type PromptHandlers, type ToolCallEvent } from './client';
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

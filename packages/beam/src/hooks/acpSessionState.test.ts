import { describe, expect, it } from 'vitest';
import type { ToolCallEvent } from '../lib/acp';
import { acpSessionReducer, initialAcpSessionState, type AcpSessionState } from './acpSessionState';

/** Applies a sequence of actions to `initialAcpSessionState` and returns the final state. */
function run(...actions: Parameters<typeof acpSessionReducer>[1][]): AcpSessionState {
  return actions.reduce(acpSessionReducer, initialAcpSessionState);
}

describe('acpSessionReducer: unified timeline (D4)', () => {
  it('orders interleaved live messages and tool calls by first arrival, not by kind', () => {
    const state = run(
      { type: 'session_reset', sessionId: 'sess-1' },
      { type: 'prompt_start' },
      { type: 'user_message_chunk', id: 'u1', text: 'run ls' },
      { type: 'tool_call', event: { updateKind: 'tool_call', toolCallId: 'tc-1', title: 'ls', status: 'pending' } },
      { type: 'message_chunk', id: 'a1', text: 'Running ' },
      { type: 'tool_call', event: { updateKind: 'tool_call_update', toolCallId: 'tc-1', status: 'completed' } },
      { type: 'message_chunk', id: 'a1', text: 'ls now.' },
    );

    expect(state.items).toEqual([
      { kind: 'message', id: 'u1' },
      { kind: 'tool_call', id: 'tc-1' },
      { kind: 'message', id: 'a1' },
    ]);
    expect(state.messages['u1']).toMatchObject({ role: 'user', text: 'run ls' });
    expect(state.messages['a1']).toMatchObject({ role: 'assistant', text: 'Running ls now.', streaming: true });
    expect(state.toolCalls['tc-1']).toMatchObject({ title: 'ls', status: 'completed' });
    // tool_call_update merges into the existing card, not a second timeline item.
    expect(state.items.filter(it => it.id === 'tc-1')).toHaveLength(1);
  });

  it('attaches thought chunks to their message by messageId as collapsible data, not a separate timeline item', () => {
    const state = run(
      { type: 'session_reset', sessionId: 'sess-1' },
      { type: 'prompt_start' },
      { type: 'thought_chunk', id: 'a1', text: 'Let me check ' },
      { type: 'thought_chunk', id: 'a1', text: 'the docs.' },
      { type: 'message_chunk', id: 'a1', text: 'Sure, here you go.' },
    );

    // Thinking arrived first — the message item is created on the thought
    // chunk, not duplicated when the text chunk for the same id follows.
    expect(state.items).toEqual([{ kind: 'message', id: 'a1' }]);
    expect(state.messages['a1']).toMatchObject({
      role: 'assistant',
      text: 'Sure, here you go.',
      thinking: 'Let me check the docs.',
      streaming: true,
      thinkingStreaming: true,
    });
  });

  it('prompt_end/prompt_error clear streaming flags on every message, including thinking', () => {
    const streaming = run(
      { type: 'session_reset', sessionId: 'sess-1' },
      { type: 'prompt_start' },
      { type: 'thought_chunk', id: 'a1', text: 'hmm' },
      { type: 'message_chunk', id: 'a1', text: 'ok' },
    );
    expect(streaming.messages['a1']).toMatchObject({ streaming: true, thinkingStreaming: true });

    const ended = acpSessionReducer(streaming, { type: 'prompt_end', stopReason: 'end_turn' });
    expect(ended.messages['a1']).toMatchObject({ streaming: false, thinkingStreaming: false, text: 'ok', thinking: 'hmm' });
    expect(ended.isPrompting).toBe(false);
    expect(ended.stopReason).toBe('end_turn');

    const erroredState = acpSessionReducer(streaming, { type: 'prompt_error', message: 'boom' });
    expect(erroredState.messages['a1']).toMatchObject({ streaming: false, thinkingStreaming: false });
    expect(erroredState.error).toBe('boom');
  });
});

describe('acpSessionReducer: stable message identity within one turn', () => {
  // Root cause: `acpWorkspaceController.ts`'s `buildSessionHandlers` groups
  // `message_chunk`/`thought_chunk`s by `id ?? currentTurnId` — if the
  // agent's chunks switch between carrying a real `messageId` and falling
  // back to the turn-id alias (or a thought chunk disagrees with the text
  // chunk's id) mid-turn, keying the reducer purely off `action.id` would
  // split one message into two timeline items, orphaning open/closed state.
  // Per the documented one-assistant-message-per-turn contract, all of these
  // must resolve onto a single item.

  it('no-id-then-id: a turn-id-fallback chunk followed by a real messageId chunk yields ONE item', () => {
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'prompt_start' },
      // No server messageId yet — the controller falls back to its turn id.
      { type: 'message_chunk', id: 'assistant-1', text: 'Hello' },
      // The agent starts tagging chunks with a real messageId mid-turn.
      { type: 'message_chunk', id: 'msg-42', text: ' world' },
    );

    expect(state.items).toEqual([{ kind: 'message', id: 'assistant-1' }]);
    expect(Object.keys(state.messages)).toEqual(['assistant-1']);
    expect(state.messages['assistant-1']).toMatchObject({ role: 'assistant', text: 'Hello world', streaming: true });
  });

  it('id-then-no-id: a real messageId chunk followed by a turn-id-fallback chunk yields ONE item', () => {
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'prompt_start' },
      { type: 'message_chunk', id: 'msg-42', text: 'Hello' },
      // The agent stops sending a messageId partway through the same turn.
      { type: 'message_chunk', id: 'assistant-1', text: ' world' },
    );

    expect(state.items).toEqual([{ kind: 'message', id: 'msg-42' }]);
    expect(Object.keys(state.messages)).toEqual(['msg-42']);
    expect(state.messages['msg-42']).toMatchObject({ role: 'assistant', text: 'Hello world', streaming: true });
  });

  it('thought-chunk-then-text-chunk with disagreeing ids yields ONE item with both thinking and text', () => {
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'prompt_start' },
      { type: 'thought_chunk', id: 'assistant-1', text: 'Let me think…' },
      { type: 'message_chunk', id: 'msg-42', text: 'Here you go.' },
    );

    expect(state.items).toEqual([{ kind: 'message', id: 'assistant-1' }]);
    expect(Object.keys(state.messages)).toEqual(['assistant-1']);
    expect(state.messages['assistant-1']).toMatchObject({
      role: 'assistant',
      text: 'Here you go.',
      thinking: 'Let me think…',
      streaming: true,
      thinkingStreaming: true,
    });
  });

  it('a NEW turn does not inherit the previous turn canonical id (no cross-turn bleed)', () => {
    const first = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'prompt_start' },
      { type: 'message_chunk', id: 'assistant-1', text: 'first turn' },
      { type: 'prompt_end', stopReason: 'end_turn' },
    );
    const second = acpSessionReducer(
      acpSessionReducer(first, { type: 'prompt_start' }),
      { type: 'message_chunk', id: 'assistant-2', text: 'second turn' },
    );

    expect(second.items).toEqual([
      { kind: 'message', id: 'assistant-1' },
      { kind: 'message', id: 'assistant-2' },
    ]);
    expect(second.messages['assistant-1']).toMatchObject({ text: 'first turn' });
    expect(second.messages['assistant-2']).toMatchObject({ text: 'second turn' });
  });

  it('out-of-turn (no active prompt) chunks are never merged, even with differing ids', () => {
    // No prompt_start dispatched — mirrors session/load replay, where each
    // historical message legitimately keeps its own id (see the replay
    // ordering suite below).
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'message_chunk', id: 'replay-a', text: 'one' },
      { type: 'message_chunk', id: 'replay-b', text: 'two' },
    );
    expect(state.items).toEqual([
      { kind: 'message', id: 'replay-a' },
      { kind: 'message', id: 'replay-b' },
    ]);
  });
});

describe('acpSessionReducer: session/load replay ordering', () => {
  it('reproduces a full replay sequence (user -> thought -> assistant -> tool -> tool result) in wire-arrival order', () => {
    // Mirrors acpsvc/session.go's replayMessages: one messageId per historical
    // message, thinking+text of one assistant turn share an id.
    const state = run(
      { type: 'session_reset', sessionId: 'sess-replay' },
      { type: 'user_message_chunk', id: 'replay-0', text: 'hi there' },
      { type: 'thought_chunk', id: 'replay-1', text: 'thinking about it' },
      { type: 'message_chunk', id: 'replay-1', text: 'Hello!' },
      {
        type: 'tool_call',
        event: { updateKind: 'tool_call', toolCallId: 'tc-replay', title: 'search', status: 'completed' },
      },
    );

    expect(state.items).toEqual([
      { kind: 'message', id: 'replay-0' },
      { kind: 'message', id: 'replay-1' },
      { kind: 'tool_call', id: 'tc-replay' },
    ]);
    expect(state.messages['replay-0']).toMatchObject({ role: 'user', text: 'hi there' });
    expect(state.messages['replay-1']).toMatchObject({
      role: 'assistant',
      text: 'Hello!',
      thinking: 'thinking about it',
    });
    // BUG 4c: replay lands with no prompt turn in flight — nothing will ever
    // call endStreaming() for these chunks, so they must render as already
    // complete, not stuck mid-typing-indicator.
    expect(state.messages['replay-0'].streaming).toBeFalsy();
    expect(state.messages['replay-1'].streaming).toBeFalsy();
    expect(state.messages['replay-1'].thinkingStreaming).toBeFalsy();
  });

  it('session_reset clears the previous session entirely before a new replay lands', () => {
    const before = run(
      { type: 'session_reset', sessionId: 'sess-a' },
      { type: 'message_chunk', id: 'a1', text: 'old session content' },
      { type: 'plan', entries: [{ content: 'do it', priority: 'high', status: 'in_progress' }] },
    );
    expect(before.items).toHaveLength(1);

    const after = acpSessionReducer(before, { type: 'session_reset', sessionId: 'sess-b' });
    expect(after).toEqual({ ...initialAcpSessionState, sessionId: 'sess-b' });
  });

  it('session_reset clears error, plan, usage, and configOptions specifically — a fresh session shows no leftover banner/plan/meter/config header', () => {
    const dirty = run(
      { type: 'session_reset', sessionId: 'sess-a' },
      { type: 'plan', entries: [{ content: 'route it', priority: 'high', status: 'completed' }] },
      { type: 'usage', usage: { used: 135, size: 26603 } },
      {
        type: 'config_options',
        configOptions: [{ id: 'model', name: 'Model', type: 'string', currentValue: 'x', options: [] }],
      },
      { type: 'prompt_start' },
      { type: 'prompt_error', message: 'chain execution failed' },
    );
    expect(dirty.error).toBe('chain execution failed');
    expect(dirty.plan).toHaveLength(1);
    expect(dirty.usage).not.toBeNull();
    expect(dirty.configOptions).toHaveLength(1);

    const fresh = acpSessionReducer(dirty, { type: 'session_reset', sessionId: null });
    expect(fresh.error).toBeNull();
    expect(fresh.plan).toEqual([]);
    expect(fresh.usage).toBeNull();
    expect(fresh.configOptions).toEqual([]);
    expect(fresh.items).toEqual([]);
  });
});

describe('acpSessionReducer: tool calls, plan, usage, config, commands', () => {
  it('merges tool_call_update onto an existing card in place', () => {
    const created: ToolCallEvent = { updateKind: 'tool_call', toolCallId: 'tc-1', title: 'Run ls', kind: 'execute', status: 'pending' };
    const updated: ToolCallEvent = { updateKind: 'tool_call_update', toolCallId: 'tc-1', status: 'completed', rawOutput: 'ok' };
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'tool_call', event: created },
      { type: 'tool_call', event: updated },
    );
    expect(state.toolCalls['tc-1']).toMatchObject({ title: 'Run ls', kind: 'execute', status: 'completed', rawOutput: 'ok' });
  });

  it('replaces plan/usage/configOptions/availableCommands wholesale on each update', () => {
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'plan', entries: [{ content: 'a', priority: 'low', status: 'pending' }] },
      { type: 'usage', usage: { used: 10, size: 100 } },
      { type: 'available_commands', commands: [{ name: 'help', description: 'help' }] },
      {
        type: 'config_options',
        configOptions: [{ id: 'think', name: 'Think', type: 'boolean', currentValue: 'true', options: [] }],
      },
    );
    expect(state.plan).toEqual([{ content: 'a', priority: 'low', status: 'pending' }]);
    expect(state.usage).toEqual({ used: 10, size: 100 });
    expect(state.availableCommands).toEqual([{ name: 'help', description: 'help' }]);
    expect(state.configOptions).toEqual([{ id: 'think', name: 'Think', type: 'boolean', currentValue: 'true', options: [] }]);
  });
});

describe('acpSessionReducer: permission gate', () => {
  it('sets and clears pendingPermission', () => {
    const withRequest = run(
      { type: 'session_reset', sessionId: 's1' },
      {
        type: 'permission_request',
        request: { sessionId: 's1', toolCall: { toolCallId: 'tc-1' }, options: [{ optionId: 'a', name: 'A', kind: 'allow_once' }] },
      },
    );
    expect(withRequest.pendingPermission).not.toBeNull();

    const resolved = acpSessionReducer(withRequest, { type: 'permission_resolved' });
    expect(resolved.pendingPermission).toBeNull();
  });
});

describe('acpSessionReducer: connection banner', () => {
  it('connection_lost sets the disconnected banner and interrupts an in-flight turn', () => {
    const prompting = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'prompt_start' },
      {
        type: 'permission_request',
        request: { sessionId: 's1', toolCall: { toolCallId: 'tc-1' }, options: [] },
      },
    );
    expect(prompting.isPrompting).toBe(true);
    expect(prompting.pendingPermission).not.toBeNull();

    const dropped = acpSessionReducer(prompting, { type: 'connection_lost' });
    expect(dropped.connectionBanner).toBe('disconnected');
    expect(dropped.isPrompting).toBe(false);
    expect(dropped.pendingPermission).toBeNull();
  });

  it('connection_lost clears per-message streaming flags too (BUG 4b), not just isPrompting', () => {
    const prompting = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'prompt_start' },
      { type: 'thought_chunk', id: 'a1', text: 'hmm' },
      { type: 'message_chunk', id: 'a1', text: 'partial' },
    );
    expect(prompting.messages['a1']).toMatchObject({ streaming: true, thinkingStreaming: true });

    const dropped = acpSessionReducer(prompting, { type: 'connection_lost' });
    // No prompt_end/prompt_error is ever coming for a dropped connection —
    // without this, the message would show a stuck "..." typing indicator
    // forever (see acpWorkspaceController.ts's handleTransportClose).
    expect(dropped.messages['a1']).toMatchObject({
      streaming: false,
      thinkingStreaming: false,
      text: 'partial',
      thinking: 'hmm',
    });
  });

  it('connection_resumed sets the resumed banner without touching timeline state', () => {
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'message_chunk', id: 'a1', text: 'hello' },
      { type: 'connection_lost' },
      { type: 'connection_resumed' },
    );
    expect(state.connectionBanner).toBe('resumed');
    expect(state.messages['a1']).toMatchObject({ text: 'hello' });
  });
});

describe('acpSessionReducer: streaming only marked during an active prompt turn (BUG 4c)', () => {
  it('does not mark a chunk as streaming when no prompt is in flight', () => {
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'message_chunk', id: 'orphan', text: 'nobody asked' },
    );
    expect(state.messages['orphan']).toMatchObject({ text: 'nobody asked', streaming: false });
  });

  it('a late chunk arriving after prompt_end does not resurrect the typing indicator', () => {
    const state = run(
      { type: 'session_reset', sessionId: 's1' },
      { type: 'prompt_start' },
      { type: 'message_chunk', id: 'a1', text: 'hi' },
      { type: 'prompt_end', stopReason: 'end_turn' },
      // Out-of-turn: the standing subscription still routes it (see
      // acpWorkspaceController.ts's buildSessionHandlers), but isPrompting
      // is already false again by now.
      { type: 'message_chunk', id: 'a1', text: ' again' },
    );
    expect(state.messages['a1']).toMatchObject({ text: 'hi again', streaming: false });
  });

  it('replayed history (session/load, no active turn) still renders as completed text, not stuck streaming', () => {
    const state = run(
      { type: 'session_reset', sessionId: 'sess-replay' },
      { type: 'user_message_chunk', id: 'replay-0', text: 'hi there' },
      { type: 'thought_chunk', id: 'replay-1', text: 'thinking' },
      { type: 'message_chunk', id: 'replay-1', text: 'Hello!' },
    );
    expect(state.messages['replay-0']).toMatchObject({ text: 'hi there', streaming: false });
    expect(state.messages['replay-1']).toMatchObject({ text: 'Hello!', thinking: 'thinking', streaming: false, thinkingStreaming: false });
  });
});

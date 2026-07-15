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
      { type: 'thought_chunk', id: 'a1', text: 'hmm' },
      { type: 'message_chunk', id: 'a1', text: 'ok' },
      { type: 'prompt_start' },
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

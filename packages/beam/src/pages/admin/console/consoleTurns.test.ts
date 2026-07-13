import { describe, expect, it } from 'vitest';
import type { ChatMessage } from '../../../lib/types';
import { buildConsoleTurns } from './consoleTurns';

const msg = (partial: Partial<ChatMessage> & Pick<ChatMessage, 'role' | 'content'>): ChatMessage => ({
  id: partial.id ?? `${partial.role}-${partial.content}`,
  sentAt: '2026-07-13T12:00:00Z',
  isUser: partial.role === 'user',
  isLatest: false,
  ...partial,
});

describe('buildConsoleTurns', () => {
  it('groups a plain exchange into one turn with a result', () => {
    const turns = buildConsoleTurns([
      msg({ role: 'user', content: 'what is 2+2?' }),
      msg({ role: 'assistant', content: '4' }),
    ]);
    expect(turns).toHaveLength(1);
    expect(turns[0].command?.content).toBe('what is 2+2?');
    expect(turns[0].result?.content).toBe('4');
    expect(turns[0].work).toHaveLength(0);
  });

  it('pairs tool calls and results into work, keeping the final answer as result', () => {
    const turns = buildConsoleTurns([
      msg({ role: 'user', content: 'list files' }),
      msg({
        role: 'assistant',
        content: '',
        callTools: [{ id: 'c1', function: { name: 'local_fs', arguments: '{}' } }],
      }),
      msg({ role: 'tool', content: 'a.txt b.txt', toolCallId: 'c1' }),
      msg({ role: 'assistant', content: 'Two files: a.txt and b.txt' }),
    ]);
    expect(turns).toHaveLength(1);
    expect(turns[0].work).toHaveLength(2);
    expect(turns[0].result?.content).toBe('Two files: a.txt and b.txt');
  });

  it('handles multiple turns and legacy sessions without provenance', () => {
    const turns = buildConsoleTurns([
      msg({ role: 'user', content: 'first' }),
      msg({ role: 'assistant', content: 'one' }),
      msg({ role: 'user', content: 'second' }),
      msg({ role: 'assistant', content: 'two' }),
    ]);
    expect(turns).toHaveLength(2);
    expect(turns[0].requestId).toBeUndefined();
    expect(turns[1].result?.content).toBe('two');
  });

  it('adopts provenance from stamped messages and marks the active turn live', () => {
    const turns = buildConsoleTurns(
      [
        msg({ role: 'user', content: 'go', requestId: 'req-1', chainRef: 'default-chain.json' }),
        msg({ role: 'assistant', content: 'done', requestId: 'req-1' }),
      ],
      null,
      'req-1',
    );
    expect(turns[0].requestId).toBe('req-1');
    expect(turns[0].chainRef).toBe('default-chain.json');
    expect(turns[0].live).toBe(true);
  });

  it('appends an optimistic turn until the history echoes it', () => {
    const optimistic = {
      requestId: 'req-2',
      content: 'new task',
      attachments: [],
      sentAt: '2026-07-13T12:01:00Z',
    };
    const before = buildConsoleTurns([msg({ role: 'user', content: 'old' })], optimistic, 'req-2');
    expect(before).toHaveLength(2);
    expect(before[1].live).toBe(true);
    expect(before[1].command?.content).toBe('new task');

    // Echoed by provenance…
    const byProvenance = buildConsoleTurns(
      [msg({ role: 'user', content: 'new task', requestId: 'req-2' })],
      optimistic,
      'req-2',
    );
    expect(byProvenance).toHaveLength(1);

    // …or by content (context block prepended server-side).
    const byContent = buildConsoleTurns(
      [msg({ role: 'user', content: 'Additional context:\n[x] y\n\nnew task' })],
      optimistic,
      'req-2',
    );
    expect(byContent).toHaveLength(1);
  });

  it('treats failure annotations as the result when the run failed', () => {
    const turns = buildConsoleTurns([
      msg({ role: 'user', content: 'break' }),
      msg({ role: 'assistant', content: '[step "chat" (chat_completion) failed: boom]' }),
    ]);
    expect(turns[0].result?.content).toContain('failed: boom');
  });

  it('collects preamble messages before any user turn', () => {
    const turns = buildConsoleTurns([
      msg({ role: 'system', content: 'agents.md' }),
      msg({ role: 'user', content: 'hi' }),
      msg({ role: 'assistant', content: 'hello' }),
    ]);
    expect(turns).toHaveLength(2);
    expect(turns[0].command).toBeUndefined();
    expect(turns[0].work).toHaveLength(1);
    expect(turns[1].result?.content).toBe('hello');
  });
});

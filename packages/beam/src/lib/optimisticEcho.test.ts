import { describe, expect, it } from 'vitest';
import { isOptimisticEcho, matchesOptimisticEcho } from './optimisticEcho';

describe('isOptimisticEcho', () => {
  it('matches byte-identical content (plain send)', () => {
    expect(isOptimisticEcho('hello', 'hello')).toBe(true);
  });

  it('matches when the server prepended a context block', () => {
    const persisted = 'Additional context:\n[file_excerpt] func main() {}\n\nhello';
    expect(isOptimisticEcho(persisted, 'hello')).toBe(true);
  });

  it('rejects unrelated content', () => {
    expect(isOptimisticEcho('goodbye', 'hello')).toBe(false);
  });

  it('rejects when optimistic content is only a partial suffix overlap', () => {
    expect(isOptimisticEcho('say hello', 'hello')).toBe(true); // suffix match is intentional
    expect(isOptimisticEcho('hello there', 'hello')).toBe(false);
  });

  it('rejects empty optimistic content against non-empty persisted', () => {
    expect(isOptimisticEcho('anything', '')).toBe(false);
    expect(isOptimisticEcho('', '')).toBe(true);
  });
});

describe('matchesOptimisticEcho', () => {
  const optimistic = { requestId: 'req-1', content: '/chain run', sentAt: '2026-07-13T12:00:00Z' };

  it('matches a stamped persisted message by requestId regardless of content', () => {
    const persisted = {
      content: 'rewritten by server',
      sentAt: '2026-07-13T09:00:00Z',
      requestId: 'req-1',
    };
    expect(matchesOptimisticEcho(persisted, optimistic)).toBe(true);
  });

  it('rejects a stamped message with another requestId even on identical content', () => {
    const persisted = { content: '/chain run', sentAt: '2026-07-13T12:00:01Z', requestId: 'req-0' };
    expect(matchesOptimisticEcho(persisted, optimistic)).toBe(false);
  });

  it('falls back to windowed content matching for unstamped messages', () => {
    expect(
      matchesOptimisticEcho({ content: '/chain run', sentAt: '2026-07-13T12:01:00Z' }, optimistic),
    ).toBe(true);
    expect(
      matchesOptimisticEcho({ content: 'other text', sentAt: '2026-07-13T12:01:00Z' }, optimistic),
    ).toBe(false);
  });

  it('rejects unstamped content matches outside the window (repeated command)', () => {
    expect(
      matchesOptimisticEcho({ content: '/chain run', sentAt: '2026-07-13T11:00:00Z' }, optimistic),
    ).toBe(false);
  });
});

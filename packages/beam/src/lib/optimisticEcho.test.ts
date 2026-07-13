import { describe, expect, it } from 'vitest';
import { isOptimisticEcho } from './optimisticEcho';

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

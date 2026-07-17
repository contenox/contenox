import { describe, expect, it, vi } from 'vitest';
import { toggleStore } from './usePersistentToggle';

/**
 * Covers the pure store behind `usePersistentToggle` (the React binding needs a
 * DOM renderer, which this package's test env doesn't provide — see
 * AcpWorkspaceProvider.test.ts). The store is what keeps every open chat tab's
 * panel toggle in lockstep, so its subscribe/notify/dedup contract is the part
 * worth locking down. Each test uses a unique key since stores are cached for
 * the process lifetime.
 */
describe('toggleStore', () => {
  it('defaults to false and caches one store per key', () => {
    const a = toggleStore('t:cache');
    expect(a.getSnapshot()).toBe(false);
    expect(toggleStore('t:cache')).toBe(a);
  });

  it('notifies every subscriber on change so all consumers stay in sync', () => {
    const store = toggleStore('t:notify');
    const one = vi.fn();
    const two = vi.fn();
    store.subscribe(one);
    store.subscribe(two);

    store.set(true);
    expect(store.getSnapshot()).toBe(true);
    expect(one).toHaveBeenCalledTimes(1);
    expect(two).toHaveBeenCalledTimes(1);
  });

  it('is a no-op (no notify) when the value is unchanged', () => {
    const store = toggleStore('t:noop');
    const listener = vi.fn();
    store.subscribe(listener);
    store.set(false); // already false
    expect(listener).not.toHaveBeenCalled();
  });

  it('toggle flips the value', () => {
    const store = toggleStore('t:toggle');
    store.toggle();
    expect(store.getSnapshot()).toBe(true);
    store.toggle();
    expect(store.getSnapshot()).toBe(false);
  });

  it('stops notifying after unsubscribe', () => {
    const store = toggleStore('t:unsub');
    const listener = vi.fn();
    const unsubscribe = store.subscribe(listener);
    unsubscribe();
    store.set(true);
    expect(listener).not.toHaveBeenCalled();
  });
});

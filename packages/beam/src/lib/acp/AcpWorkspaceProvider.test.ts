import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createDeferredDisposer } from './AcpWorkspaceProvider';

/**
 * Pure unit coverage for the StrictMode-safety mechanism behind
 * `AcpWorkspaceProvider`'s controller lifecycle (BUG 3). `@testing-library/react`
 * isn't a dependency of this package (see acpWorkspaceController.test.ts's doc
 * comment), so this exercises `createDeferredDisposer` directly rather than
 * mounting the provider under `<React.StrictMode>` — it's the one piece of
 * StrictMode-specific logic the provider adds on top of the (already
 * extensively tested, framework-free) controller itself.
 */

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
});

describe('createDeferredDisposer (AcpWorkspaceProvider StrictMode safety, BUG 3)', () => {
  it('disposes after the deferred delay when armed and never cancelled — the genuine-unmount case', async () => {
    const dispose = vi.fn();
    const disposer = createDeferredDisposer(dispose);

    disposer.armForCleanup();
    expect(dispose).not.toHaveBeenCalled();

    await vi.runAllTimersAsync();
    expect(dispose).toHaveBeenCalledTimes(1);
  });

  it('never disposes if cancelled before the timer fires — the StrictMode double-invoke (mount, cleanup, mount) case', async () => {
    const dispose = vi.fn();
    const disposer = createDeferredDisposer(dispose);

    // Mirrors AcpWorkspaceProvider.tsx's effect: mount -> cleanup (arms) ->
    // mount again (cancels), all before the timer would ever fire, since
    // StrictMode's simulated remount happens synchronously in the same tick.
    disposer.armForCleanup();
    disposer.cancelPendingDispose();

    await vi.runAllTimersAsync();
    expect(dispose).not.toHaveBeenCalled();
  });

  it('cancelPendingDispose() is a harmless no-op when nothing is armed', () => {
    const dispose = vi.fn();
    const disposer = createDeferredDisposer(dispose);

    expect(() => disposer.cancelPendingDispose()).not.toThrow();
    expect(dispose).not.toHaveBeenCalled();
  });

  it('a second armForCleanup() after a cancelled one still disposes on a genuine final unmount', async () => {
    const dispose = vi.fn();
    const disposer = createDeferredDisposer(dispose);

    // StrictMode's phantom remount...
    disposer.armForCleanup();
    disposer.cancelPendingDispose();
    // ...followed later by the real, final unmount.
    disposer.armForCleanup();

    await vi.runAllTimersAsync();
    expect(dispose).toHaveBeenCalledTimes(1);
  });
});

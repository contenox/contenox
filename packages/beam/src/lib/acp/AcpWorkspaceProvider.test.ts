import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  acpSessionsReducer,
  initialAcpSessionsState,
  selectFocusedSession,
  selectOpenSessionIds,
} from '../../hooks/acpWorkspaceState';
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

describe('AcpWorkspaceProvider context-value composition (multiplexing wiring)', () => {
  // The provider derives its context value from the multiplexed sessions
  // reducer: `session` = selectFocusedSession(sessions) (the single-view
  // accessor consumers like AcpChatPage read), while `sessions` exposes every
  // open slice for Slice 2. @testing-library/react isn't a dependency (see the
  // doc comment above), so this exercises that derivation directly rather than
  // mounting the provider — it's the one piece of state wiring the provider
  // adds on top of the (separately-tested) reducers.
  it('exposes the focused slice as `session` while keeping every open slice reachable', () => {
    let sessions = initialAcpSessionsState;
    const dispatch = (action: Parameters<typeof acpSessionsReducer>[1]) => {
      sessions = acpSessionsReducer(sessions, action);
    };

    // Two sessions open concurrently, sess-b focused.
    dispatch({ type: 'session_dispatch', key: 'sess-a', action: { type: 'session_reset', sessionId: 'sess-a' } });
    dispatch({ type: 'session_dispatch', key: 'sess-b', action: { type: 'session_reset', sessionId: 'sess-b' } });
    dispatch({ type: 'session_dispatch', key: 'sess-a', action: { type: 'message_chunk', id: 'ma', text: 'A background' } });
    dispatch({ type: 'session_focused', key: 'sess-b' });

    // `session` (single-view) is the focused slice...
    expect(selectFocusedSession(sessions).sessionId).toBe('sess-b');
    // ...but the backgrounded session's state is still reachable via `sessions`.
    expect(selectOpenSessionIds(sessions).sort()).toEqual(['sess-a', 'sess-b']);
    expect(sessions.slices['sess-a'].messages['ma']).toMatchObject({ text: 'A background' });

    // Re-focusing swaps what `session` renders without touching the slices.
    dispatch({ type: 'session_focused', key: 'sess-a' });
    expect(selectFocusedSession(sessions).sessionId).toBe('sess-a');
    expect(selectFocusedSession(sessions).messages['ma']).toMatchObject({ text: 'A background' });
  });
});

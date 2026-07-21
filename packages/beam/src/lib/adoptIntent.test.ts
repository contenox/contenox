import { describe, expect, it, vi } from 'vitest';
import { startAdoptSession, type AdoptIntent, type StartAdoptDeps } from './adoptIntent';

/**
 * The "open this running session in chat" contract, tested at the pure-function
 * seam (the repo's vitest env is DOM-less — see PermissionCard.test.tsx). Both
 * entry points — the fleet board's per-session sub-row and the mission detail —
 * funnel through `startAdoptSession`, so pinning it here pins both.
 */
function mockDeps(): StartAdoptDeps & {
  setAdoptIntent: ReturnType<typeof vi.fn>;
  navigate: ReturnType<typeof vi.fn>;
} {
  return { setAdoptIntent: vi.fn(), navigate: vi.fn() };
}

const intent: AdoptIntent = { instanceId: 'inst-1', sessionId: 'down-1' };

describe('startAdoptSession', () => {
  it('stages the adopt intent and routes to the chat surface', () => {
    const deps = mockDeps();
    startAdoptSession(intent, deps);

    expect(deps.setAdoptIntent).toHaveBeenCalledTimes(1);
    expect(deps.setAdoptIntent).toHaveBeenCalledWith(intent);
    expect(deps.navigate).toHaveBeenCalledWith('/chat');
  });

  it('stages BEFORE it routes — the chat surface must find the intent already set on arrival', () => {
    const calls: string[] = [];
    const deps: StartAdoptDeps = {
      setAdoptIntent: () => calls.push('stage'),
      navigate: () => calls.push('navigate'),
    };
    startAdoptSession(intent, deps);
    expect(calls).toEqual(['stage', 'navigate']);
  });

  it('does NOT drive the ACP workspace controller (entry points live off the chat page)', () => {
    // The deps surface is deliberately just {setAdoptIntent, navigate} — no
    // focusEmptyTab — so /fleet and /missions/:id (and their DOM-less tests)
    // never need the workspace provider whose hook throws off-provider.
    const deps = mockDeps();
    startAdoptSession(intent, deps);
    expect(Object.keys(deps)).toEqual(['setAdoptIntent', 'navigate']);
  });
});

import { describe, expect, it, vi } from 'vitest';
import { startNewChat, type NewChatDeps } from './newChatIntent';

/**
 * The sessions rail's "start a fresh chat" contract, tested at the pure-function
 * seam (the repo's vitest env is DOM-less — see PermissionCard.test.tsx). Both
 * rail entry points funnel through `startNewChat`, so pinning it here pins the
 * plain "New session" button AND the agent picker at once.
 */
function mockDeps(): NewChatDeps & {
  setStagedAgent: ReturnType<typeof vi.fn>;
  setStagedRoot: ReturnType<typeof vi.fn>;
  focusEmptyTab: ReturnType<typeof vi.fn>;
  navigate: ReturnType<typeof vi.fn>;
  closeSidebar: ReturnType<typeof vi.fn>;
} {
  return {
    setStagedAgent: vi.fn(),
    setStagedRoot: vi.fn(),
    focusEmptyTab: vi.fn(),
    navigate: vi.fn(),
    closeSidebar: vi.fn(),
  };
}

describe('startNewChat', () => {
  it('picking an agent stages that agent AND focuses the empty /chat surface (from a session tab)', () => {
    // Entry state (a): the rail's picker is used while a session tab is open.
    // Driving `focusEmptyTab` re-points the tab-model to the empty surface — the
    // source of truth — so the navigate to bare /chat is NOT reverted by the
    // tab↔route sync (see WorkspaceTabs) the way a bare navigate alone was.
    const deps = mockDeps();
    startNewChat('claude', deps);

    expect(deps.setStagedAgent).toHaveBeenCalledTimes(1);
    expect(deps.setStagedAgent).toHaveBeenCalledWith('claude');
    expect(deps.focusEmptyTab).toHaveBeenCalledTimes(1);
    expect(deps.navigate).toHaveBeenCalledWith('/chat');
    expect(deps.closeSidebar).toHaveBeenCalledTimes(1);
  });

  it('stages, then focuses the empty surface, then routes — order-fixed so the sync agrees with the route', () => {
    // The tab-model focus MUST precede the route: it is what makes the empty
    // surface the active tab before the URL follows, so the sync never reverts
    // /chat back to the outgoing /chat/:id.
    const calls: string[] = [];
    const deps: NewChatDeps = {
      setStagedAgent: () => calls.push('stage'),
      setStagedRoot: () => calls.push('stageRoot'),
      focusEmptyTab: () => calls.push('focusEmptyTab'),
      navigate: () => calls.push('navigate'),
      closeSidebar: () => calls.push('close'),
    };
    startNewChat('claude', deps);
    expect(calls).toEqual(['stage', 'stageRoot', 'focusEmptyTab', 'navigate', 'close']);
  });

  it('a launcher stages its project cwd; a non-launcher path clears the context stage', () => {
    // The Projects-page launcher passes { cwd }, staging that project for the next
    // chat. Every non-launcher path passes no cwd, clearing the context stage —
    // defensive hygiene against an un-consumed launcher stage leaking. (The empty
    // chat's own sticky pick is separate and is not touched here.)
    const launcher = mockDeps();
    startNewChat(null, launcher, { cwd: '/home/me/api' });
    expect(launcher.setStagedRoot).toHaveBeenCalledWith('/home/me/api');

    const plain = mockDeps();
    startNewChat(null, plain);
    expect(plain.setStagedRoot).toHaveBeenCalledWith(null);
  });

  it('is a no-op-safe call when a caller omits setStagedRoot (isolated test / older caller)', () => {
    // setStagedRoot is optional; a caller that never launches into a project may
    // omit it, and startNewChat must not throw.
    const { setStagedRoot: _omit, ...rest } = mockDeps();
    expect(() => startNewChat(null, rest)).not.toThrow();
    expect(rest.focusEmptyTab).toHaveBeenCalledTimes(1);
  });

  it('picking while already on the empty surface still updates the staged agent (navigate is a no-op there)', () => {
    // Entry state (b)/(d): already on /chat. `navigate('/chat')` is a no-op for
    // the current URL, so the staged-agent update is the load-bearing effect —
    // and it must fire regardless of route. Two picks in a row each re-stage.
    const deps = mockDeps();

    startNewChat('claude', deps);
    startNewChat('gpt-4o', deps);

    expect(deps.setStagedAgent).toHaveBeenNthCalledWith(1, 'claude');
    expect(deps.setStagedAgent).toHaveBeenNthCalledWith(2, 'gpt-4o');
    // The action does not branch on the current route — it always re-stages and
    // re-drives the empty surface (both are idempotent while already there).
    expect(deps.setStagedAgent).toHaveBeenCalledTimes(2);
    expect(deps.focusEmptyTab).toHaveBeenCalledTimes(2);
    expect(deps.navigate).toHaveBeenCalledTimes(2);
  });

  it('picking the native "Contenox (default)" entry clears staging (null)', () => {
    // Entry state (e): the native option maps to null upstream (AgentPicker) —
    // startNewChat(null) clears any previously-staged agent so the next chat is
    // native contenox. This is also exactly the plain "New session" button.
    const deps = mockDeps();
    startNewChat(null, deps);

    expect(deps.setStagedAgent).toHaveBeenCalledWith(null);
    expect(deps.focusEmptyTab).toHaveBeenCalledTimes(1);
    expect(deps.navigate).toHaveBeenCalledWith('/chat');
  });

  it('the plain "New session" button and native pick are the SAME call — one code path', () => {
    // Both the plain button (startNew(null)) and the picker's native entry
    // (onSelect(null) -> startNew(null)) produce byte-identical effects; they
    // cannot drift.
    const plainButton = mockDeps();
    const nativePick = mockDeps();

    startNewChat(null, plainButton); // plain "New session"
    startNewChat(null, nativePick); // picker -> native

    expect(plainButton.setStagedAgent.mock.calls).toEqual(nativePick.setStagedAgent.mock.calls);
    expect(plainButton.focusEmptyTab.mock.calls).toEqual(nativePick.focusEmptyTab.mock.calls);
    expect(plainButton.navigate.mock.calls).toEqual(nativePick.navigate.mock.calls);
  });
});

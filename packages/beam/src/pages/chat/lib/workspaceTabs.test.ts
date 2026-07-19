import { describe, expect, it } from 'vitest';
import {
  initialWorkspaceTabsState,
  routeForActiveTab,
  workspaceTabsReducer,
  type WorkspaceTabsState,
} from './workspaceTabs';

function reduce(state: WorkspaceTabsState, ...actions: Parameters<typeof workspaceTabsReducer>[1][]): WorkspaceTabsState {
  return actions.reduce(workspaceTabsReducer, state);
}

describe('workspaceTabsReducer', () => {
  it('starts with no tabs and the empty surface active', () => {
    expect(initialWorkspaceTabsState).toEqual({ tabs: [], activeId: null });
  });

  describe('open', () => {
    it('appends a new tab and makes it active', () => {
      const state = reduce(initialWorkspaceTabsState, { type: 'open', id: 'a' });
      expect(state).toEqual({ tabs: ['a'], activeId: 'a' });
    });

    it('keeps stable order when opening several, activating the latest', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'open', id: 'c' },
      );
      expect(state).toEqual({ tabs: ['a', 'b', 'c'], activeId: 'c' });
    });

    it('dedups by identity — re-opening an already-open tab only focuses it, no reorder', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'open', id: 'a' },
      );
      expect(state).toEqual({ tabs: ['a', 'b'], activeId: 'a' });
    });

    it('returns the same reference when re-opening the already-active tab', () => {
      const opened = reduce(initialWorkspaceTabsState, { type: 'open', id: 'a' });
      const again = workspaceTabsReducer(opened, { type: 'open', id: 'a' });
      expect(again).toBe(opened);
    });
  });

  describe('focus', () => {
    it('activates an already-open tab without wire traffic', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'focus', id: 'a' },
      );
      expect(state.activeId).toBe('a');
      expect(state.tabs).toEqual(['a', 'b']);
    });

    it('is a no-op for a tab that is not open', () => {
      const opened = reduce(initialWorkspaceTabsState, { type: 'open', id: 'a' });
      expect(workspaceTabsReducer(opened, { type: 'focus', id: 'ghost' })).toBe(opened);
    });
  });

  describe('focus_empty', () => {
    it('activates the empty surface while leaving open tabs untouched', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'focus_empty' },
      );
      expect(state).toEqual({ tabs: ['a', 'b'], activeId: null });
    });
  });

  describe('close', () => {
    it('removes a background tab, keeping the active one', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'open', id: 'c' },
        { type: 'focus', id: 'c' },
        { type: 'close', id: 'a' },
      );
      expect(state).toEqual({ tabs: ['b', 'c'], activeId: 'c' });
    });

    it('re-points focus to the left neighbor when closing the active tab', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'open', id: 'c' },
        { type: 'focus', id: 'b' },
        { type: 'close', id: 'b' },
      );
      expect(state).toEqual({ tabs: ['a', 'c'], activeId: 'a' });
    });

    it('re-points to the new first tab when closing the active leftmost tab', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'focus', id: 'a' },
        { type: 'close', id: 'a' },
      );
      expect(state).toEqual({ tabs: ['b'], activeId: 'b' });
    });

    it('falls back to the empty surface when closing the last tab', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'close', id: 'a' },
      );
      expect(state).toEqual({ tabs: [], activeId: null });
    });

    it('is a no-op for a tab that is not open', () => {
      const opened = reduce(initialWorkspaceTabsState, { type: 'open', id: 'a' });
      expect(workspaceTabsReducer(opened, { type: 'close', id: 'ghost' })).toBe(opened);
    });
  });

  describe('sync', () => {
    it('drops tabs whose session no longer exists (external delete)', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'open', id: 'c' },
        { type: 'focus', id: 'b' },
        { type: 'sync', openIds: ['a', 'c'] },
      );
      expect(state.tabs).toEqual(['a', 'c']);
      // active 'b' vanished -> left neighbor 'a'
      expect(state.activeId).toBe('a');
    });

    it('preserves existing order and appends newly-open ids at the end', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'open', id: 'b' },
        { type: 'sync', openIds: ['b', 'a', 'c'] },
      );
      expect(state.tabs).toEqual(['a', 'b', 'c']);
      expect(state.activeId).toBe('b');
    });

    it('keeps the empty surface active across a sync', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'focus_empty' },
        { type: 'sync', openIds: ['a'] },
      );
      expect(state).toEqual({ tabs: ['a'], activeId: null });
    });

    it('returns the same reference when nothing changes', () => {
      const opened = reduce(initialWorkspaceTabsState, { type: 'open', id: 'a' }, { type: 'open', id: 'b' });
      expect(workspaceTabsReducer(opened, { type: 'sync', openIds: ['a', 'b'] })).toBe(opened);
    });

    it('falls back to the empty surface when every open tab vanishes', () => {
      const state = reduce(
        initialWorkspaceTabsState,
        { type: 'open', id: 'a' },
        { type: 'sync', openIds: [] },
      );
      expect(state).toEqual({ tabs: [], activeId: null });
    });
  });
});

describe('routeForActiveTab (tab -> URL sync decision)', () => {
  // The half of WorkspaceTabs's route↔tab sync that decides "who navigates to
  // /chat/:focusedId, and when" — the exact seam where the "New session" /
  // agent-pick regression lived. When this fired with a STALE active tab it
  // reverted the sidebar's `navigate('/chat')` straight back to /chat/:id.

  it('targets a real active tab’s own /chat/:id', () => {
    // This IS the value that reverted the sidebar navigate — but it may only be
    // produced while the session is genuinely still the active tab.
    expect(routeForActiveTab('sess-a', false)).toBe('/chat/sess-a');
    // Intent is irrelevant once a real tab is active.
    expect(routeForActiveTab('sess-a', true)).toBe('/chat/sess-a');
  });

  it('targets bare /chat when the empty surface was reached intentionally', () => {
    // "New session" / last-tab-close: the focused session + startNewChat outcome
    // — active tab is now the empty surface, so the route target is /chat, NOT a
    // reverted /chat/:id.
    expect(routeForActiveTab(null, true)).toBe('/chat');
  });

  it('targets nothing (null) for the transient empty of deep-link adoption', () => {
    // On entry `activeId` is briefly null before the URL's tab opens; returning
    // null leaves the deep-link URL untouched instead of clobbering it with
    // /chat. This is also what makes the post-startNewChat empty surface (whose
    // move was driven by the tab-model, not this one-shot flag) NOT re-navigate:
    // the sidebar already routed to /chat and the sync adds nothing.
    expect(routeForActiveTab(null, false)).toBeNull();
  });
});

/**
 * Pure, framework-free tab-model for the chat workspace's in-app tabs
 * (workspace-tabs Slice 2). This owns ONLY the UI concern — which chat
 * surfaces have an open tab, their left-to-right order, and which one is
 * active — and knows nothing about subscriptions or the wire. `useWorkspaceTabs.ts`
 * pairs this reducer with the multi-session controller (`openSessionTab` /
 * `closeSessionTab` / `focusSession`): the reducer is the tab-model, the
 * controller is the subscription-model, kept cleanly separated so this logic
 * is unit-testable without React or a transport (see `workspaceTabs.test.ts`).
 *
 * `activeId === null` means the empty/new-chat surface is active — the
 * lazy-creation surface shown before any session exists on this tab (see D5 /
 * `AcpChatPage`). It is NOT a session id and never appears in `tabs`.
 */

export interface WorkspaceTabsState {
  /** Open session ids, in stable left-to-right tab order (never contains the empty-surface sentinel). */
  tabs: string[];
  /** The active tab's session id, or `null` for the empty/new-chat surface. */
  activeId: string | null;
}

export const initialWorkspaceTabsState: WorkspaceTabsState = {
  tabs: [],
  activeId: null,
};

export type WorkspaceTabsAction =
  /** Open `id` as a tab (append if absent — dedup by identity) and make it active. */
  | { type: 'open'; id: string }
  /** Close `id`'s tab; if it was active, focus moves to a neighbor (left, else right, else the empty surface). */
  | { type: 'close'; id: string }
  /** Make `id`'s already-open tab active (no-op if it has no tab). */
  | { type: 'focus'; id: string }
  /** Make the empty/new-chat surface active without touching the open tab list. */
  | { type: 'focus_empty' }
  /**
   * Reconcile the open-tab list with an authoritative set of open session ids
   * (e.g. after an external `session/delete` from the sidebar drops a slice):
   * keep existing tabs still present (preserving their order), append newly-open
   * ids, drop vanished ones. If the active tab vanished, focus falls back to a
   * neighbor (or the empty surface).
   */
  | { type: 'sync'; openIds: readonly string[] };

/** The neighbor to focus after `id` (at `index` in `tabs`) leaves: the tab to its left, else the new tab to its right, else the empty surface. */
function neighborAfterRemoval(tabs: string[], index: number): string | null {
  if (tabs.length === 0) return null;
  if (index > 0) return tabs[index - 1];
  return tabs[0];
}

export function workspaceTabsReducer(state: WorkspaceTabsState, action: WorkspaceTabsAction): WorkspaceTabsState {
  switch (action.type) {
    case 'open': {
      if (state.tabs.includes(action.id)) {
        return state.activeId === action.id ? state : { ...state, activeId: action.id };
      }
      return { tabs: [...state.tabs, action.id], activeId: action.id };
    }

    case 'close': {
      const index = state.tabs.indexOf(action.id);
      if (index === -1) return state;
      const tabs = state.tabs.filter(id => id !== action.id);
      const activeId = state.activeId === action.id ? neighborAfterRemoval(tabs, index) : state.activeId;
      return { tabs, activeId };
    }

    case 'focus':
      if (!state.tabs.includes(action.id)) return state;
      return state.activeId === action.id ? state : { ...state, activeId: action.id };

    case 'focus_empty':
      return state.activeId === null ? state : { ...state, activeId: null };

    case 'sync': {
      const openSet = new Set(action.openIds);
      const kept = state.tabs.filter(id => openSet.has(id));
      const appended = action.openIds.filter(id => !state.tabs.includes(id));
      const tabs = [...kept, ...appended];
      // Preserve the active tab unless it vanished; a null active (empty
      // surface) is always still valid.
      let activeId = state.activeId;
      if (activeId !== null && !openSet.has(activeId)) {
        const oldIndex = state.tabs.indexOf(activeId);
        activeId = neighborAfterRemoval(tabs, oldIndex === -1 ? 0 : oldIndex);
      }
      const unchanged =
        tabs.length === state.tabs.length &&
        tabs.every((id, i) => id === state.tabs[i]) &&
        activeId === state.activeId;
      return unchanged ? state : { tabs, activeId };
    }

    default:
      return state;
  }
}

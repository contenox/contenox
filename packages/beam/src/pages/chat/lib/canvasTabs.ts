/**
 * Pure, framework-free tab-model for the chat's secondary CANVAS region
 * (workspace-canvas Slice B1). This is the sibling of `workspaceTabs.ts`: where
 * that reducer owns the PRIMARY region's chat-as-tabs, this one owns the canvas's
 * own tabs — the terminal (B1) and, later, file/diff surfaces (B2+). It knows
 * ONLY the UI concern (which surfaces have an open canvas tab, their order, and
 * which is active) and nothing about the wire or the terminal/file streams.
 *
 * Kept as a parallel reducer rather than reusing `workspaceTabsReducer`: the
 * canvas tabs carry a `kind` (and later a payload like a file path) so dedup is
 * by a composite identity, and the canvas has no `sync`/`focus_empty` concerns
 * (its tabs are ephemeral UI, not mirrors of subscribed sessions). The
 * neighbor-after-close and dedup-by-identity semantics mirror `workspaceTabs.ts`.
 *
 * `activeId === null` means the canvas is EMPTY — no tab open, so the region
 * collapses and the chat takes the full width (see `CanvasRegion`).
 */

/** The kinds of surface the canvas can host. B1 ships `terminal`; `file`/`diff` land in B2+. */
export type CanvasTabKind = 'terminal';

export interface CanvasTab {
  /**
   * Stable identity used for dedup and for the `Tabs`/`TabPanel` wiring. For a
   * per-session singleton like the terminal this is a fixed string; a future
   * file tab would encode its path (e.g. `file:src/app.ts`).
   */
  id: string;
  kind: CanvasTabKind;
}

export interface CanvasTabsState {
  /** Open canvas tabs, in stable left-to-right order. */
  tabs: CanvasTab[];
  /** The active tab's id, or `null` when the canvas is empty (collapsed). */
  activeId: string | null;
}

export const initialCanvasTabsState: CanvasTabsState = {
  tabs: [],
  activeId: null,
};

export type CanvasTabsAction =
  /** Open `tab` (append if its id is absent — dedup by identity) and make it active. */
  | { type: 'open'; tab: CanvasTab }
  /** Close `id`'s tab; if it was active, focus moves to a neighbor (left, else right, else the empty/collapsed canvas). */
  | { type: 'close'; id: string }
  /** Make `id`'s already-open tab active (no-op if it has no tab). */
  | { type: 'focus'; id: string };

/** The neighbor to focus after the tab at `index` leaves: the tab to its left, else the new leftmost, else the empty canvas. */
function neighborAfterRemoval(tabs: CanvasTab[], index: number): string | null {
  if (tabs.length === 0) return null;
  if (index > 0) return tabs[index - 1].id;
  return tabs[0].id;
}

export function canvasTabsReducer(state: CanvasTabsState, action: CanvasTabsAction): CanvasTabsState {
  switch (action.type) {
    case 'open': {
      if (state.tabs.some(tab => tab.id === action.tab.id)) {
        return state.activeId === action.tab.id ? state : { ...state, activeId: action.tab.id };
      }
      return { tabs: [...state.tabs, action.tab], activeId: action.tab.id };
    }

    case 'close': {
      const index = state.tabs.findIndex(tab => tab.id === action.id);
      if (index === -1) return state;
      const tabs = state.tabs.filter(tab => tab.id !== action.id);
      const activeId = state.activeId === action.id ? neighborAfterRemoval(tabs, index) : state.activeId;
      return { tabs, activeId };
    }

    case 'focus':
      if (!state.tabs.some(tab => tab.id === action.id)) return state;
      return state.activeId === action.id ? state : { ...state, activeId: action.id };

    default:
      return state;
  }
}

/** The canvas's terminal surface — a per-session singleton (one PTY per session). */
export const TERMINAL_CANVAS_TAB: CanvasTab = { id: 'terminal', kind: 'terminal' };

import { useCallback, useReducer } from 'react';
import {
  canvasTabsReducer,
  initialCanvasTabsState,
  type CanvasTab,
} from '../pages/chat/lib/canvasTabs';

export interface UseCanvasTabsResult {
  /** Open canvas tabs, in stable left-to-right order. */
  tabs: CanvasTab[];
  /** The active tab's id, or `null` when the canvas is empty (collapsed). */
  activeId: string | null;
  /** Open `tab` (dedup by identity — focus if already open) and make it active. */
  open: (tab: CanvasTab) => void;
  /** Close `id`'s tab; focus falls back to a neighbor (or collapses the canvas). */
  close: (id: string) => void;
  /** Make an already-open tab active. */
  focus: (id: string) => void;
}

/**
 * The chat canvas's tab-model (workspace-canvas Slice B1). A thin binding over
 * the pure `canvasTabsReducer` — canvas tabs are ephemeral, session-scoped UI
 * state (no subscriptions, no wire), so unlike `useWorkspaceTabs` there is no
 * controller to keep in lockstep; this is just `useReducer` with stable action
 * callbacks. Owned per-session by `ChatSessionTab`, so each open chat tab has
 * its own canvas.
 */
export function useCanvasTabs(): UseCanvasTabsResult {
  const [state, dispatch] = useReducer(canvasTabsReducer, initialCanvasTabsState);

  const open = useCallback((tab: CanvasTab) => dispatch({ type: 'open', tab }), []);
  const close = useCallback((id: string) => dispatch({ type: 'close', id }), []);
  const focus = useCallback((id: string) => dispatch({ type: 'focus', id }), []);

  return { tabs: state.tabs, activeId: state.activeId, open, close, focus };
}

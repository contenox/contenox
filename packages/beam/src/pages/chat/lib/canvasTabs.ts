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

import type { PermissionOptionKind, RequestPermissionRequest } from '../../../lib/acp';

/**
 * The kinds of surface the canvas can host: the per-session terminal, read-only
 * file views, and maximized permission approvals (a pending permission gate
 * opened full-size so large diffs/commands can be read in the panel).
 */
export type CanvasTabKind = 'terminal' | 'file' | 'approval';

export interface CanvasTab {
  /**
   * Stable identity used for dedup and for the `Tabs`/`TabPanel` wiring. For a
   * per-session singleton like the terminal this is a fixed string; a file tab
   * encodes its path (`file:src/app.ts`) so re-opening the same file focuses its
   * existing tab instead of duplicating it; an approval tab encodes its tool-call
   * id (`approval:<toolCallId>`) so re-maximizing the same request focuses the
   * open tab and concurrent requests each get their own.
   */
  id: string;
  kind: CanvasTabKind;
  /** For `file` tabs: the workspace-relative path this view reads. */
  path?: string;
  /** For `file`/`approval` tabs: the display label shown on the tab strip. */
  title?: string;
  /**
   * For `approval` tabs: the SNAPSHOT of the permission request taken when the
   * gate was maximized. Snapshotted (not read live) so the tab keeps rendering
   * the payload after the request resolves — the live `pendingPermission` clears
   * to null on answer, but the maximized tab must not blank out mid-read (see
   * `approvalTabStatus`).
   */
  approval?: RequestPermissionRequest;
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

/** Prefix for a file tab's id, keeping file identities from colliding with the terminal singleton. */
export const FILE_CANVAS_TAB_PREFIX = 'file:';

/**
 * A read-only file-view canvas tab for `path`. The id is derived from the path
 * so re-opening the same file focuses its existing tab (dedup by identity); the
 * basename becomes the tab label.
 */
export function fileCanvasTab(path: string): CanvasTab {
  const name = path.split('/').filter(Boolean).pop() ?? path;
  return { id: `${FILE_CANVAS_TAB_PREFIX}${path}`, kind: 'file', path, title: name };
}

/** Prefix for an approval tab's id, keyed by tool-call id so concurrent approvals never collide. */
export const APPROVAL_CANVAS_TAB_PREFIX = 'approval:';

/**
 * A maximized-permission-approval canvas tab for `request`. The id is derived
 * from the request's tool-call id so re-maximizing the same pending request
 * focuses its existing tab (dedup by identity) and concurrent approvals each get
 * their own tab. The whole request is snapshotted onto the tab so the panel can
 * keep rendering the payload after the request resolves (see `CanvasTab.approval`).
 */
export function approvalCanvasTab(request: RequestPermissionRequest): CanvasTab {
  const { toolCall } = request;
  return {
    id: `${APPROVAL_CANVAS_TAB_PREFIX}${toolCall.toolCallId}`,
    kind: 'approval',
    title: toolCall.title ?? toolCall.toolCallId,
    approval: request,
  };
}

/** How an approval was resolved, for the tab's post-answer banner. */
export type ApprovalResolution = 'allowed' | 'denied';

/**
 * The lifecycle state a maximized-approval tab renders. `pending` shows the live
 * Allow/Deny actions + keymap; `resolved` shows a read-only banner (never a live
 * Allow button on a dead request). `resolution` is the outcome when it was
 * answered IN this tab, or `null` when the request resolved elsewhere (the modal,
 * another tab) or the turn was cancelled — an outcome the tab can't attribute.
 */
export type ApprovalTabStatus =
  | { state: 'pending' }
  | { state: 'resolved'; resolution: ApprovalResolution | null };

/**
 * Pure pending→resolved decision for a maximized-approval tab. The tab is `pending`
 * only while its snapshotted `toolCallId` still matches the session's live
 * `pendingToolCallId` AND it hasn't been answered here; once answered here the
 * local `answered` outcome wins, and any other divergence (live request gone or
 * swapped to a different tool call) reads as resolved-elsewhere.
 */
export function approvalTabStatus(
  toolCallId: string,
  pendingToolCallId: string | null,
  answered: ApprovalResolution | null,
): ApprovalTabStatus {
  if (answered !== null) return { state: 'resolved', resolution: answered };
  if (pendingToolCallId !== null && pendingToolCallId === toolCallId) return { state: 'pending' };
  return { state: 'resolved', resolution: null };
}

/** Maps a chosen permission option's kind to the resolution the tab records (allow* → allowed, else denied). */
export function approvalResolutionForOption(kind: PermissionOptionKind): ApprovalResolution {
  return kind.startsWith('allow') ? 'allowed' : 'denied';
}

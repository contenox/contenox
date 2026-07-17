import { describe, expect, it } from 'vitest';
import type { RequestPermissionRequest } from '../../../lib/acp';
import {
  approvalCanvasTab,
  approvalResolutionForOption,
  approvalTabStatus,
  APPROVAL_CANVAS_TAB_PREFIX,
  canvasTabsReducer,
  fileCanvasTab,
  FILE_CANVAS_TAB_PREFIX,
  initialCanvasTabsState,
  TERMINAL_CANVAS_TAB,
  type CanvasTab,
  type CanvasTabsState,
} from './canvasTabs';

function permReq(toolCallId: string, title?: string): RequestPermissionRequest {
  return {
    sessionId: 'sess-1',
    toolCall: { toolCallId, title },
    options: [
      { optionId: 'ok', name: 'Allow', kind: 'allow_once' },
      { optionId: 'no', name: 'Deny', kind: 'reject_once' },
    ],
  };
}

const term: CanvasTab = TERMINAL_CANVAS_TAB;
const a: CanvasTab = { id: 'a', kind: 'terminal' };
const b: CanvasTab = { id: 'b', kind: 'terminal' };
const c: CanvasTab = { id: 'c', kind: 'terminal' };

function reduce(state: CanvasTabsState, ...actions: Parameters<typeof canvasTabsReducer>[1][]): CanvasTabsState {
  return actions.reduce(canvasTabsReducer, state);
}

describe('canvasTabsReducer', () => {
  it('starts empty (collapsed canvas)', () => {
    expect(initialCanvasTabsState).toEqual({ tabs: [], activeId: null });
  });

  describe('open', () => {
    it('appends a new tab and makes it active', () => {
      const state = reduce(initialCanvasTabsState, { type: 'open', tab: term });
      expect(state).toEqual({ tabs: [term], activeId: 'terminal' });
    });

    it('keeps stable order when opening several, activating the latest', () => {
      const state = reduce(
        initialCanvasTabsState,
        { type: 'open', tab: a },
        { type: 'open', tab: b },
        { type: 'open', tab: c },
      );
      expect(state).toEqual({ tabs: [a, b, c], activeId: 'c' });
    });

    it('dedups by identity — re-opening an already-open tab only focuses it, no reorder or duplicate', () => {
      const state = reduce(
        initialCanvasTabsState,
        { type: 'open', tab: a },
        { type: 'open', tab: b },
        { type: 'open', tab: a },
      );
      expect(state).toEqual({ tabs: [a, b], activeId: 'a' });
    });

    it('returns the same reference when re-opening the already-active tab', () => {
      const opened = reduce(initialCanvasTabsState, { type: 'open', tab: a });
      const again = canvasTabsReducer(opened, { type: 'open', tab: a });
      expect(again).toBe(opened);
    });
  });

  describe('focus', () => {
    it('activates an already-open tab', () => {
      const state = reduce(
        initialCanvasTabsState,
        { type: 'open', tab: a },
        { type: 'open', tab: b },
        { type: 'focus', id: 'a' },
      );
      expect(state.activeId).toBe('a');
      expect(state.tabs).toEqual([a, b]);
    });

    it('is a no-op for a tab that is not open', () => {
      const opened = reduce(initialCanvasTabsState, { type: 'open', tab: a });
      expect(canvasTabsReducer(opened, { type: 'focus', id: 'ghost' })).toBe(opened);
    });
  });

  describe('close', () => {
    it('collapses the canvas when the only tab is closed', () => {
      const state = reduce(
        initialCanvasTabsState,
        { type: 'open', tab: term },
        { type: 'close', id: 'terminal' },
      );
      expect(state).toEqual({ tabs: [], activeId: null });
    });

    it('removes a background tab, keeping the active one', () => {
      const state = reduce(
        initialCanvasTabsState,
        { type: 'open', tab: a },
        { type: 'open', tab: b },
        { type: 'open', tab: c },
        { type: 'focus', id: 'c' },
        { type: 'close', id: 'a' },
      );
      expect(state).toEqual({ tabs: [b, c], activeId: 'c' });
    });

    it('re-points focus to the left neighbor when closing the active tab', () => {
      const state = reduce(
        initialCanvasTabsState,
        { type: 'open', tab: a },
        { type: 'open', tab: b },
        { type: 'open', tab: c },
        { type: 'focus', id: 'b' },
        { type: 'close', id: 'b' },
      );
      expect(state).toEqual({ tabs: [a, c], activeId: 'a' });
    });

    it('re-points to the new leftmost when closing the active leftmost tab', () => {
      const state = reduce(
        initialCanvasTabsState,
        { type: 'open', tab: a },
        { type: 'open', tab: b },
        { type: 'focus', id: 'a' },
        { type: 'close', id: 'a' },
      );
      expect(state).toEqual({ tabs: [b], activeId: 'b' });
    });

    it('is a no-op for a tab that is not open', () => {
      const opened = reduce(initialCanvasTabsState, { type: 'open', tab: a });
      expect(canvasTabsReducer(opened, { type: 'close', id: 'ghost' })).toBe(opened);
    });
  });
});

describe('fileCanvasTab', () => {
  it('derives a path-scoped id, the file kind, and the basename label', () => {
    expect(fileCanvasTab('src/app.ts')).toEqual({
      id: `${FILE_CANVAS_TAB_PREFIX}src/app.ts`,
      kind: 'file',
      path: 'src/app.ts',
      title: 'app.ts',
    });
  });

  it('never collides with the terminal singleton id', () => {
    expect(fileCanvasTab('terminal').id).not.toBe(TERMINAL_CANVAS_TAB.id);
  });
});

describe('file canvas tabs', () => {
  it('opens a file beside the terminal and dedups on re-open of the same path', () => {
    const term = TERMINAL_CANVAS_TAB;
    const fileA = fileCanvasTab('src/a.ts');
    const fileB = fileCanvasTab('src/b.ts');
    const state = reduce(
      initialCanvasTabsState,
      { type: 'open', tab: term },
      { type: 'open', tab: fileA },
      { type: 'open', tab: fileB },
      { type: 'open', tab: fileA }, // re-open A: focus, no duplicate/reorder
    );
    expect(state.tabs).toEqual([term, fileA, fileB]);
    expect(state.activeId).toBe(fileA.id);
  });
});

describe('approvalCanvasTab', () => {
  it('derives a tool-call-scoped id, the approval kind, the title label, and snapshots the request', () => {
    const req = permReq('call-42', 'Write file');
    expect(approvalCanvasTab(req)).toEqual({
      id: `${APPROVAL_CANVAS_TAB_PREFIX}call-42`,
      kind: 'approval',
      title: 'Write file',
      approval: req,
    });
  });

  it('falls back to the tool-call id as the label when the request has no title', () => {
    expect(approvalCanvasTab(permReq('call-7')).title).toBe('call-7');
  });

  it('never collides with a file tab id for a like-named path', () => {
    expect(approvalCanvasTab(permReq('a.ts')).id).not.toBe(fileCanvasTab('a.ts').id);
  });
});

describe('approval canvas tabs', () => {
  it('opens an approval beside the terminal/file and dedups on re-maximize of the same request', () => {
    const term = TERMINAL_CANVAS_TAB;
    const file = fileCanvasTab('src/a.ts');
    const approval = approvalCanvasTab(permReq('call-1'));
    const state = reduce(
      initialCanvasTabsState,
      { type: 'open', tab: term },
      { type: 'open', tab: file },
      { type: 'open', tab: approval },
      { type: 'open', tab: approval }, // re-maximize: focus, no duplicate/reorder
    );
    expect(state.tabs).toEqual([term, file, approval]);
    expect(state.activeId).toBe(approval.id);
  });

  it('keeps concurrent approvals as separate coexisting tabs, one per tool-call id', () => {
    const a1 = approvalCanvasTab(permReq('call-1'));
    const a2 = approvalCanvasTab(permReq('call-2'));
    const state = reduce(
      initialCanvasTabsState,
      { type: 'open', tab: a1 },
      { type: 'open', tab: a2 },
    );
    expect(state.tabs).toEqual([a1, a2]);
    expect(state.tabs.map(t => t.id)).toEqual([`${APPROVAL_CANVAS_TAB_PREFIX}call-1`, `${APPROVAL_CANVAS_TAB_PREFIX}call-2`]);
    expect(state.activeId).toBe(a2.id);
  });
});

describe('approvalTabStatus (pending → resolved transitions)', () => {
  it('is pending while the snapshotted request is still the live pending one and unanswered', () => {
    expect(approvalTabStatus('call-1', 'call-1', null)).toEqual({ state: 'pending' });
  });

  it('resolves with the local outcome once answered in this tab, regardless of the live pending id', () => {
    expect(approvalTabStatus('call-1', 'call-1', 'allowed')).toEqual({ state: 'resolved', resolution: 'allowed' });
    expect(approvalTabStatus('call-1', null, 'denied')).toEqual({ state: 'resolved', resolution: 'denied' });
  });

  it('resolves with an unknown outcome when the request left the pending slot elsewhere (cancel / modal / new request)', () => {
    expect(approvalTabStatus('call-1', null, null)).toEqual({ state: 'resolved', resolution: null });
    expect(approvalTabStatus('call-1', 'call-2', null)).toEqual({ state: 'resolved', resolution: null });
  });
});

describe('approvalResolutionForOption', () => {
  it('maps allow_* kinds to allowed and reject_* kinds to denied', () => {
    expect(approvalResolutionForOption('allow_once')).toBe('allowed');
    expect(approvalResolutionForOption('allow_always')).toBe('allowed');
    expect(approvalResolutionForOption('reject_once')).toBe('denied');
    expect(approvalResolutionForOption('reject_always')).toBe('denied');
  });
});

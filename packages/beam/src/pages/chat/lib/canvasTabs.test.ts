import { describe, expect, it } from 'vitest';
import {
  canvasTabsReducer,
  fileCanvasTab,
  FILE_CANVAS_TAB_PREFIX,
  initialCanvasTabsState,
  TERMINAL_CANVAS_TAB,
  type CanvasTab,
  type CanvasTabsState,
} from './canvasTabs';

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

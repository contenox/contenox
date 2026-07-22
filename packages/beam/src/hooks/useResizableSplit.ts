import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type CSSProperties,
  type PointerEvent as ReactPointerEvent,
  type RefObject,
} from 'react';

/**
 * A dependency-free two-pane resizable split (IDE-workflows blueprint, Arc 4).
 * A draggable handle sets
 * the first pane's size as a fraction of the container, clamped to sane bounds
 * and persisted to localStorage. `react-resizable-panels` is deliberately NOT a
 * dependency: a single split needs ~80 lines, not a docking subsystem.
 *
 * Wiring policy: built and tested now, but the mission inspector's tabs are a
 * single-pane surface, so there is no honest two-pane case to attach it to on
 * that page today. It is therefore EXPORTED for the first real side-by-side
 * consumer (e.g. a future changed-files list beside its diff) rather than
 * wired prematurely. All the arithmetic is factored into the pure helpers below
 * so the drag math is unit-testable without a DOM.
 */

/** Clamp a fraction into [min, max]; a NaN (bad persisted value) collapses to min. */
export function clampFraction(value: number, min: number, max: number): number {
  if (Number.isNaN(value)) return min;
  return Math.min(max, Math.max(min, value));
}

/**
 * The first pane's fraction (0..1) for a pointer at `pointer` within a container
 * spanning [containerStart, containerStart + containerSize]. A zero-size
 * container yields 0 (nothing to divide). Orientation-agnostic: pass
 * left/width/clientX for a horizontal split or top/height/clientY for a vertical
 * one.
 */
export function fractionFromPointer(
  containerStart: number,
  containerSize: number,
  pointer: number,
): number {
  if (containerSize <= 0) return 0;
  return (pointer - containerStart) / containerSize;
}

export interface ResizableSplitOptions {
  /** localStorage key for the persisted fraction. */
  storageKey: string;
  /** Initial fraction when nothing is persisted (0..1). Default 0.5. */
  defaultFraction?: number;
  /** Minimum first-pane fraction. Default 0.15. */
  min?: number;
  /** Maximum first-pane fraction. Default 0.85. */
  max?: number;
  /** Drag axis. `horizontal` = side-by-side (drag X); `vertical` = stacked (drag Y). Default horizontal. */
  orientation?: 'horizontal' | 'vertical';
}

export interface ResizableSplit {
  /** Current first-pane fraction (0..1), clamped. */
  fraction: number;
  /** Ref to attach to the flex container the split measures against. */
  containerRef: RefObject<HTMLDivElement | null>;
  /** Style for the first pane (a percentage width or height, per orientation). */
  firstPaneStyle: CSSProperties;
  /** Pointer-down handler for the drag handle. */
  onHandlePointerDown: (e: ReactPointerEvent) => void;
  /** True while a drag is in progress (for cursor / overlay styling). */
  isDragging: boolean;
}

function readPersistedFraction(key: string, fallback: number, min: number, max: number): number {
  try {
    if (typeof localStorage !== 'undefined') {
      const raw = localStorage.getItem(key);
      if (raw !== null) return clampFraction(parseFloat(raw), min, max);
    }
  } catch {
    /* best-effort */
  }
  return clampFraction(fallback, min, max);
}

function writePersistedFraction(key: string, value: number): void {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(key, value.toFixed(4));
  } catch {
    /* best-effort */
  }
}

export function useResizableSplit(options: ResizableSplitOptions): ResizableSplit {
  const { storageKey, defaultFraction = 0.5, min = 0.15, max = 0.85, orientation = 'horizontal' } =
    options;
  const containerRef = useRef<HTMLDivElement | null>(null);
  const [fraction, setFraction] = useState(() =>
    readPersistedFraction(storageKey, defaultFraction, min, max),
  );
  const [isDragging, setIsDragging] = useState(false);

  const onHandlePointerDown = useCallback((e: ReactPointerEvent) => {
    e.preventDefault();
    setIsDragging(true);
  }, []);

  useEffect(() => {
    if (!isDragging) return;
    const onMove = (e: PointerEvent) => {
      const el = containerRef.current;
      if (!el) return;
      const rect = el.getBoundingClientRect();
      const next =
        orientation === 'horizontal'
          ? fractionFromPointer(rect.left, rect.width, e.clientX)
          : fractionFromPointer(rect.top, rect.height, e.clientY);
      setFraction(clampFraction(next, min, max));
    };
    const onUp = () => setIsDragging(false);
    window.addEventListener('pointermove', onMove);
    window.addEventListener('pointerup', onUp);
    return () => {
      window.removeEventListener('pointermove', onMove);
      window.removeEventListener('pointerup', onUp);
    };
  }, [isDragging, orientation, min, max]);

  // Persist once a drag settles (not on every intermediate frame).
  useEffect(() => {
    if (!isDragging) writePersistedFraction(storageKey, fraction);
  }, [isDragging, fraction, storageKey]);

  const percent = `${(fraction * 100).toFixed(2)}%`;
  const firstPaneStyle: CSSProperties =
    orientation === 'horizontal' ? { width: percent } : { height: percent };

  return { fraction, containerRef, firstPaneStyle, onHandlePointerDown, isDragging };
}

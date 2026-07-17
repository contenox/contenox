import { useCallback, useSyncExternalStore } from 'react';

/**
 * Subscribes to a CSS media query and returns whether it currently matches,
 * re-rendering on change. Framework-thin wrapper over `window.matchMedia` via
 * `useSyncExternalStore` (SSR/test-safe: returns `false` when `matchMedia` is
 * unavailable). Used for the layout breakpoints the CANVAS region needs in JS —
 * a purely-CSS responsive rule can't switch between the side-by-side
 * `ResizablePanel` split and the narrow full-width takeover, since the split's
 * sizing lives in inline flex styles.
 */
export function useMediaQuery(query: string): boolean {
  const subscribe = useCallback(
    (onChange: () => void) => {
      if (typeof window === 'undefined' || !window.matchMedia) return () => {};
      const mql = window.matchMedia(query);
      mql.addEventListener('change', onChange);
      return () => mql.removeEventListener('change', onChange);
    },
    [query],
  );

  const getSnapshot = useCallback(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return false;
    return window.matchMedia(query).matches;
  }, [query]);

  return useSyncExternalStore(subscribe, getSnapshot, () => false);
}

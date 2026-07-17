import { useEffect, useMemo, useRef, useState } from 'react';
import type { WorkspaceFilePeek } from './useWorkspaceFiles';
import {
  previewEntryFromPeek,
  selectPreview,
  type FilePreviewCache,
  type FilePreviewState,
} from '../pages/chat/lib/filePreview';

/** Delay before an arrowed-to file is fetched, so fast browsing doesn't spam the API. */
const DEFAULT_DEBOUNCE_MS = 160;

/**
 * Live-preview data hook for the composer's `@`-mention browser: given the
 * currently highlighted FILE path (or `null` for a directory / closed menu),
 * fetches its content via `readFile`, DEBOUNCED so quick arrowing doesn't fire
 * a request per keystroke, and MEMOIZED by path so revisiting a file is
 * instant. Out-of-order responses are harmless — each fetch writes only its own
 * path's memo entry and `selectPreview` renders the highlighted path's entry,
 * so a late response for a file the user already left never shows.
 *
 * Pure state logic lives in `pages/chat/lib/filePreview.ts`; this hook only
 * owns the timer, the fetch, and the memo state.
 */
export function useFilePreview(
  path: string | null,
  readFile: (path: string) => Promise<WorkspaceFilePeek>,
  debounceMs: number = DEFAULT_DEBOUNCE_MS,
): FilePreviewState {
  const [cache, setCache] = useState<FilePreviewCache>({});
  // Read the memo without making it a fetch-effect dependency, so a resolving
  // fetch (which mutates `cache`) never restarts the debounce timer for the
  // path currently being browsed.
  const cacheRef = useRef(cache);
  cacheRef.current = cache;
  const inFlight = useRef<Set<string>>(new Set());

  // A new `readFile` identity means the workspace root changed: drop the memo
  // so a same-named path in a different root can't show stale content.
  useEffect(() => {
    setCache({});
    inFlight.current = new Set();
  }, [readFile]);

  useEffect(() => {
    if (!path) return;
    if (cacheRef.current[path] || inFlight.current.has(path)) return;
    const timer = setTimeout(() => {
      inFlight.current.add(path);
      void readFile(path)
        .then(peek => setCache(prev => ({ ...prev, [path]: previewEntryFromPeek(peek) })))
        .catch(() => setCache(prev => ({ ...prev, [path]: { kind: 'error' } })))
        .finally(() => inFlight.current.delete(path));
    }, debounceMs);
    return () => clearTimeout(timer);
  }, [path, readFile, debounceMs]);

  return useMemo(() => selectPreview(path, cache), [path, cache]);
}

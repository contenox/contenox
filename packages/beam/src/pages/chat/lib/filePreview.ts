/**
 * Pure logic for the composer's live file preview: the state machine and the
 * in-memory memo that back `useFilePreview`. No React, no timers, no DOM — the
 * hook wires debounce + fetch to this core, and these helpers are what the
 * tests exercise.
 *
 * The memo is keyed by path, which is also the out-of-order guard: a resolved
 * fetch writes only its own path's entry, and `selectPreview` always reads the
 * entry for the CURRENTLY highlighted path, so a stale response for a file the
 * user already arrowed away from can never win the display.
 */

/** A resolved file, normalized for the preview memo. */
export type FilePreviewEntry =
  | { kind: 'text'; text: string }
  | { kind: 'binary' }
  | { kind: 'error' };

/** The path-keyed memo of already-resolved previews. */
export type FilePreviewCache = Record<string, FilePreviewEntry>;

/** What the preview pane should render right now. */
export type FilePreviewState =
  | { status: 'hidden' }
  | { status: 'loading'; path: string }
  | { status: 'text'; path: string; text: string }
  | { status: 'binary'; path: string }
  | { status: 'error'; path: string };

/** Normalizes a fetched `WorkspaceFilePeek`-shaped result into a memo entry. */
export function previewEntryFromPeek(peek: { text: string; isBinary: boolean }): FilePreviewEntry {
  return peek.isBinary ? { kind: 'binary' } : { kind: 'text', text: peek.text };
}

/**
 * Derives what to display for the highlighted `path` from the memo: nothing
 * when there is no file highlighted, a loading state while its content has not
 * resolved yet, and the resolved text/binary/error entry once it has.
 */
export function selectPreview(path: string | null, cache: FilePreviewCache): FilePreviewState {
  if (!path) return { status: 'hidden' };
  const entry = cache[path];
  if (!entry) return { status: 'loading', path };
  if (entry.kind === 'text') return { status: 'text', path, text: entry.text };
  if (entry.kind === 'binary') return { status: 'binary', path };
  return { status: 'error', path };
}

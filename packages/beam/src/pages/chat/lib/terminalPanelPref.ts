/**
 * Tiny localStorage-backed preference for whether the chat page's terminal
 * panel is expanded — the sibling of `workspacePanelPref.ts`, so the second
 * IDE-style panel remembers its collapsed/expanded state across reloads.
 * Best-effort: degrades to a per-tab default when localStorage is unavailable
 * (private mode / SSR / tests).
 */
const STORAGE_KEY = 'beam_terminal_panel_open';

export function readTerminalPanelOpen(): boolean {
  try {
    if (typeof localStorage !== 'undefined') {
      return localStorage.getItem(STORAGE_KEY) === '1';
    }
  } catch {
    /* best-effort */
  }
  return false;
}

export function writeTerminalPanelOpen(open: boolean): void {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, open ? '1' : '0');
    }
  } catch {
    /* best-effort */
  }
}

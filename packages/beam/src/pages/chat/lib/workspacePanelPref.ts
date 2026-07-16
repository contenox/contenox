/**
 * Tiny localStorage-backed preference for whether the chat page's workspace
 * file panel is expanded — so the IDE-style panel remembers its collapsed/
 * expanded state across reloads. Best-effort: degrades to a per-tab default
 * when localStorage is unavailable (private mode / SSR / tests).
 */
const STORAGE_KEY = 'beam_workspace_panel_open';

export function readWorkspacePanelOpen(): boolean {
  try {
    if (typeof localStorage !== 'undefined') {
      return localStorage.getItem(STORAGE_KEY) === '1';
    }
  } catch {
    /* best-effort */
  }
  return false;
}

export function writeWorkspacePanelOpen(open: boolean): void {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, open ? '1' : '0');
    }
  } catch {
    /* best-effort */
  }
}

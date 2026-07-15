/**
 * Pure state-machine logic for the composer's slash-command completion popover
 * (SlashCommandMenu.tsx): deriving open/query from the raw draft text,
 * filtering entries, and mapping keydown events to navigation/accept/close
 * actions. No React, no DOM — the component wires this to `useState`/keydown
 * handlers; this module is what's under test.
 */

export interface SlashMenuEntry {
  /** Command name without the leading '/' (e.g. "compact"). */
  name: string;
  description: string;
  /** Usage hint, if any (e.g. "/compact [instructions]"). */
  usage?: string;
}

/**
 * Derives whether the popover should be open and what the user has typed so
 * far as the filter query, from the composer's current draft text. Only a
 * bare `/name` at the very start of the draft (no space yet — the user is
 * still composing the command's name) opens the menu; anything else
 * (a leading space, text before the slash, or a space after the name meaning
 * args have started) closes it.
 */
export function deriveSlashMenuQuery(draft: string): { open: boolean; query: string } {
  const match = /^\/([a-zA-Z0-9_-]*)$/.exec(draft);
  if (!match) return { open: false, query: '' };
  return { open: true, query: match[1] ?? '' };
}

/** Case-insensitive prefix filter over `entries` by `name`. */
export function filterSlashCommands(entries: SlashMenuEntry[], query: string): SlashMenuEntry[] {
  const q = query.toLowerCase();
  return entries.filter(e => e.name.toLowerCase().startsWith(q));
}

export type SlashMenuKeyAction = { type: 'move'; delta: 1 | -1 } | { type: 'accept' } | { type: 'close' };

/** Maps a keydown's `key` to the popover action it drives, or null if the key is not one the popover intercepts (in which case the composer should handle it normally). */
export function slashMenuKeyFromEvent(key: string): SlashMenuKeyAction | null {
  switch (key) {
    case 'ArrowDown':
      return { type: 'move', delta: 1 };
    case 'ArrowUp':
      return { type: 'move', delta: -1 };
    case 'Tab':
    case 'Enter':
      return { type: 'accept' };
    case 'Escape':
      return { type: 'close' };
    default:
      return null;
  }
}

/** Wraps `index + delta` into `[0, length)`. Returns 0 for an empty list (nothing to clamp onto). */
export function clampIndex(index: number, length: number): number {
  if (length <= 0) return 0;
  return ((index % length) + length) % length;
}

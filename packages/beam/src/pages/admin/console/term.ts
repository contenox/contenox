/**
 * Console layout class helpers (surface, borders, tones) for the parts of the
 * console shell not yet covered by a @contenox/ui component — the status line,
 * page container, and composer frame. Explicit light/dark pairs pick specific
 * shades for the terminal look; bare semantic tokens would auto-flip to their
 * `dark-*` twin of the same step, which is not always the shade wanted here.
 *
 * Line/prompt/markdown/diff rendering lives in @contenox/ui (TerminalLine,
 * TerminalPromptInput, terminalMarkdownComponents, DiffView) — not here.
 */
export const TERM = {
  text: 'text-text dark:text-dark-text',
  dim: 'text-text-muted dark:text-dark-text-muted',
  accent: 'text-success dark:text-dark-success',
  ok: 'text-success dark:text-dark-success',
  err: 'text-error dark:text-dark-error',
  surface: 'bg-surface-50 dark:bg-dark-surface-100',
  inset: 'bg-surface-100 dark:bg-dark-surface-200',
  border: 'border-surface-300 dark:border-dark-surface-400',
  font: 'font-mono text-[13px] leading-[1.55]',
  /* One step down from `font` — status line, hints, notices. Only these two
     sizes exist in the console; anything else is drift. */
  small: 'font-mono text-[12px] leading-[1.5]',
} as const;

/** Compact one-line summary of a tool call's JSON arguments. */
export function summarizeArgs(argsJSON: string | undefined, cap = 80): string {
  if (!argsJSON) return '';
  let obj: unknown;
  try {
    obj = JSON.parse(argsJSON);
  } catch {
    return argsJSON.length > cap ? argsJSON.slice(0, cap) + '…' : argsJSON;
  }
  if (obj && typeof obj === 'object') {
    const parts = Object.entries(obj as Record<string, unknown>).map(([k, v]) => {
      const val = typeof v === 'string' ? v : JSON.stringify(v);
      return `${k}: ${val}`;
    });
    const joined = parts.join(', ');
    return joined.length > cap ? joined.slice(0, cap) + '…' : joined;
  }
  const s = String(obj);
  return s.length > cap ? s.slice(0, cap) + '…' : s;
}

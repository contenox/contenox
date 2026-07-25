/**
 * Shared formatting for `execute`-kind tool calls (`local_shell`, `run`/`exec`
 * tools — see `runtime/acpsvc/events.go` `toolKindFor`). Used both by the
 * transcript's tool-call detail (`TranscriptItems.tsx`, renders one call's
 * command+output as a mini terminal) and by the reducer (`acpSessionState.ts`,
 * appends the same text into the session's shared terminal panel scrollback)
 * so a `local_shell` call reads like the shell it ran in wherever it's shown.
 */

/** Best-effort `$ command arg1 arg2` line from a raw input `{command, args}` (see `summarizeToolCallArgs` on the backend). */
export function shellCommandLine(rawInput: unknown): string | null {
  if (rawInput == null || typeof rawInput !== 'object') return null;
  const obj = rawInput as Record<string, unknown>;
  const command = typeof obj.command === 'string' ? obj.command : null;
  if (!command) return null;
  const args = Array.isArray(obj.args) ? obj.args.filter((a): a is string => typeof a === 'string') : [];
  return ['$', command, ...args].join(' ');
}

/** Shell tool output is a plain string (`json.RawMessage(jsonString(ev.Content))` on the backend); split into terminal lines. */
export function shellOutputLines(rawOutput: unknown): string[] | null {
  return typeof rawOutput === 'string' ? rawOutput.split('\n') : null;
}

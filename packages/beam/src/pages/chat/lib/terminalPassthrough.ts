/**
 * `!` passthrough parsing for the composer. A draft whose first non-whitespace
 * character is `!` is NOT a prompt: the rest of the line is a shell command run
 * in the session's persistent shell (no LLM turn, no tokens). This is a pure
 * function so the interception logic is unit-testable without a component.
 *
 * Kept deliberately simple (see shell-sessions blueprint): a single leading `!`,
 * the remainder is the command verbatim. `!!foo` therefore runs `!foo` as the
 * shell line — no special `!!` handling. A bare `!` (nothing after it) is not a
 * command and returns null, so the composer falls through to normal behavior.
 */
export function parseTerminalPassthrough(draft: string): string | null {
  const trimmed = draft.trimStart();
  if (!trimmed.startsWith('!')) return null;
  const command = trimmed.slice(1).trim();
  return command.length > 0 ? command : null;
}

/**
 * Splits accumulated terminal scrollback into display lines, dropping carriage
 * returns (PTYs emit CRLF). An empty buffer yields no lines.
 */
export function splitTerminalLines(text: string): string[] {
  if (text === '') return [];
  return text.replace(/\r/g, '').split('\n');
}

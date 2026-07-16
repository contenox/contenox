/**
 * Terminal escape-sequence sanitizing for PTY output rendered in the DOM.
 *
 * A real PTY emits far more than SGR color codes: bracketed-paste toggles
 * (`ESC[?2004h`), OSC window-title sets (`ESC]0;…BEL`), cursor moves, screen
 * clears, and stray control bytes. Rendered verbatim in a `<pre>` these show
 * up as literal garbage (`[?2004h`, `]0;host: ~`, …). This
 * module removes everything non-visual while PRESERVING SGR sequences
 * (`ESC[…m`) so a downstream colorizer can still interpret bold/colors.
 */

// OSC: ESC ] ... terminated by BEL (\x07) or ST (ESC \).
const OSC_RE = /\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g;
// CSI that is NOT SGR: ESC [ (optional private/param bytes) final byte in
// @A–Z[-`a–z{–~ EXCEPT 'm'. Captures ESC[?2004h, ESC[2J, cursor moves, etc.
const CSI_NON_SGR_RE = /\x1b\[[0-9;?]*[@-ln-~]/g;
// Other single-char escapes: ESC followed by one byte in the intermediate/
// final range (e.g. ESC=, ESC>, ESCM) — not '[' (CSI) or ']' (OSC).
const ESC_SINGLE_RE = /\x1b[@-Zut\\-_=><]/g;
// Stray control chars except \n, \t, and ESC (0x1b) — ESC is preserved so the
// SGR sequences left intact above keep their introducer. BEL, backspace,
// vertical tab, carriage returns (spurious breaks in a pre), etc. go.
// eslint-disable-next-line no-control-regex
const STRAY_CONTROL_RE = /[\x00-\x08\x0b-\x1a\x1c-\x1f\x7f]/g;

/**
 * Strips non-SGR terminal control sequences, leaving printable text and SGR
 * color codes (`ESC[…m`) intact. Pure and idempotent.
 */
export function sanitizeTerminalText(text: string): string {
  if (text.indexOf("\x1b") === -1 && !/[\x00-\x08\x0b-\x1f\x7f]/.test(text)) {
    return text;
  }
  return text
    .replace(OSC_RE, "")
    .replace(CSI_NON_SGR_RE, "")
    .replace(ESC_SINGLE_RE, "")
    .replace(STRAY_CONTROL_RE, "");
}

/**
 * Full strip including SGR — plain text with no escapes at all. For contexts
 * that do not colorize (copy-to-clipboard, plain excerpts).
 */
export function stripAnsi(text: string): string {
  // eslint-disable-next-line no-control-regex
  return sanitizeTerminalText(text).replace(/\x1b\[[0-9;]*m/g, "");
}

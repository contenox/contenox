/** Glyphs used to lead terminal-style scrollback lines. */
export const TERMINAL_GLYPH = {
  /** The command prompt marker. */
  prompt: "❯",
  /** A tool/step action line. */
  tool: "⏺",
  /** A continuation / result line under an action. */
  cont: "⎿",
  /** A paused/approval line. */
  approval: "⏸",
  /** The blinking run indicator. */
  running: "▍",
} as const;

export type TerminalGlyph = (typeof TERMINAL_GLYPH)[keyof typeof TERMINAL_GLYPH];

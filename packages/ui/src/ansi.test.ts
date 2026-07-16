import { describe, expect, it } from "vitest";
import { sanitizeTerminalText, stripAnsi } from "./ansi";

describe("sanitizeTerminalText", () => {
  it("returns plain text unchanged", () => {
    expect(sanitizeTerminalText("hello world")).toBe("hello world");
  });

  it("removes bracketed-paste toggles", () => {
    expect(sanitizeTerminalText("\x1b[?2004hhi\x1b[?2004l")).toBe("hi");
  });

  it("removes OSC window-title sequences (BEL terminated)", () => {
    expect(sanitizeTerminalText("\x1b]0;naro@nox16: ~\x07$ ls")).toBe("$ ls");
  });

  it("preserves SGR color codes for a downstream colorizer", () => {
    const input = "\x1b[01;32mnaro@nox16\x1b[00m$ echo hi";
    const out = sanitizeTerminalText(input);
    expect(out).toContain("\x1b[01;32m");
    expect(out).toContain("\x1b[00m");
    expect(out).toContain("naro@nox16");
    expect(out).not.toContain("2004");
  });

  it("cleans the exact live bash prompt garbage end to end", () => {
    const raw =
      "echo hallo\r\n\x1b[?2004h\x1b]0;naro@nox16: ~\x07\x1b[01;32mnaro@nox16\x1b[00m:\x1b[01;34m~\x1b[00m$ ";
    const out = sanitizeTerminalText(raw);
    expect(out).not.toContain("2004");
    expect(out).not.toContain("\x07");
    expect(out).not.toContain("\r");
    expect(out).toContain("naro@nox16");
    // color codes survive
    expect(out).toContain("\x1b[01;34m");
  });

  it("strips stray control bytes and carriage returns", () => {
    expect(sanitizeTerminalText("a\x07b\rc")).toBe("abc");
  });

  it("keeps newlines and tabs", () => {
    expect(sanitizeTerminalText("a\nb\tc")).toBe("a\nb\tc");
  });

  it("is idempotent", () => {
    const raw = "\x1b[?2004h\x1b[32mgreen\x1b[0m\x1b[?2004l";
    expect(sanitizeTerminalText(sanitizeTerminalText(raw))).toBe(
      sanitizeTerminalText(raw),
    );
  });
});

describe("stripAnsi", () => {
  it("also removes SGR color codes for plain contexts", () => {
    expect(stripAnsi("\x1b[01;32mnaro\x1b[00m$ x")).toBe("naro$ x");
  });
});

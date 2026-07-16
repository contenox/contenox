import { describe, expect, it } from 'vitest';
import { parseTerminalPassthrough, splitTerminalLines } from './terminalPassthrough';

describe('parseTerminalPassthrough', () => {
  it('extracts the command from a leading-! draft', () => {
    expect(parseTerminalPassthrough('!echo hallo-welt')).toBe('echo hallo-welt');
  });

  it('tolerates leading whitespace before the !', () => {
    expect(parseTerminalPassthrough('   !ls -la')).toBe('ls -la');
  });

  it('trims the command after the !', () => {
    expect(parseTerminalPassthrough('!  go test ./...  ')).toBe('go test ./...');
  });

  it('returns null for a normal prompt', () => {
    expect(parseTerminalPassthrough('what is the weather?')).toBeNull();
  });

  it('returns null for a slash command', () => {
    expect(parseTerminalPassthrough('/model')).toBeNull();
  });

  it('returns null for a bare ! with no command', () => {
    expect(parseTerminalPassthrough('!')).toBeNull();
    expect(parseTerminalPassthrough('!   ')).toBeNull();
  });

  it('keeps a second ! as part of the command (no special !! handling)', () => {
    expect(parseTerminalPassthrough('!!foo')).toBe('!foo');
  });
});

describe('splitTerminalLines', () => {
  it('returns no lines for empty input', () => {
    expect(splitTerminalLines('')).toEqual([]);
  });

  it('splits on newlines and strips carriage returns', () => {
    expect(splitTerminalLines('a\r\nb\r\nc')).toEqual(['a', 'b', 'c']);
  });
});

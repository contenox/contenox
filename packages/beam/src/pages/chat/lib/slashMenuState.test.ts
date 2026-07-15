import { describe, expect, it } from 'vitest';
import {
  clampIndex,
  deriveSlashMenuQuery,
  filterSlashCommands,
  slashMenuKeyFromEvent,
  type SlashMenuEntry,
} from './slashMenuState';

describe('deriveSlashMenuQuery', () => {
  it('opens with an empty query for a bare "/"', () => {
    expect(deriveSlashMenuQuery('/')).toEqual({ open: true, query: '' });
  });

  it('opens with the typed name while composing the command', () => {
    expect(deriveSlashMenuQuery('/comp')).toEqual({ open: true, query: 'comp' });
  });

  it('closes once a space (start of args) appears', () => {
    expect(deriveSlashMenuQuery('/compact now')).toEqual({ open: false, query: '' });
  });

  it('closes for plain text with no leading slash', () => {
    expect(deriveSlashMenuQuery('hello')).toEqual({ open: false, query: '' });
  });

  it('closes when the slash is not at the start of the draft', () => {
    expect(deriveSlashMenuQuery('hi /compact')).toEqual({ open: false, query: '' });
  });

  it('closes for an empty draft', () => {
    expect(deriveSlashMenuQuery('')).toEqual({ open: false, query: '' });
  });
});

describe('filterSlashCommands', () => {
  const entries: SlashMenuEntry[] = [
    { name: 'compact', description: 'Compact history' },
    { name: 'clear', description: 'Clear session' },
    { name: 'help', description: 'List commands' },
  ];

  it('returns all entries for an empty query', () => {
    expect(filterSlashCommands(entries, '')).toHaveLength(3);
  });

  it('filters by case-insensitive name prefix', () => {
    expect(filterSlashCommands(entries, 'c').map(e => e.name)).toEqual(['compact', 'clear']);
    expect(filterSlashCommands(entries, 'CO').map(e => e.name)).toEqual(['compact']);
  });

  it('returns an empty list when nothing matches', () => {
    expect(filterSlashCommands(entries, 'zzz')).toEqual([]);
  });
});

describe('slashMenuKeyFromEvent', () => {
  it('maps arrows to move actions', () => {
    expect(slashMenuKeyFromEvent('ArrowDown')).toEqual({ type: 'move', delta: 1 });
    expect(slashMenuKeyFromEvent('ArrowUp')).toEqual({ type: 'move', delta: -1 });
  });

  it('maps Tab and Enter to accept', () => {
    expect(slashMenuKeyFromEvent('Tab')).toEqual({ type: 'accept' });
    expect(slashMenuKeyFromEvent('Enter')).toEqual({ type: 'accept' });
  });

  it('maps Escape to close', () => {
    expect(slashMenuKeyFromEvent('Escape')).toEqual({ type: 'close' });
  });

  it('returns null for keys the popover does not intercept', () => {
    expect(slashMenuKeyFromEvent('a')).toBeNull();
    expect(slashMenuKeyFromEvent('Shift')).toBeNull();
  });
});

describe('clampIndex', () => {
  it('wraps forward past the end', () => {
    expect(clampIndex(3, 3)).toBe(0);
  });

  it('wraps backward past the start', () => {
    expect(clampIndex(-1, 3)).toBe(2);
  });

  it('passes through in-range indices', () => {
    expect(clampIndex(1, 3)).toBe(1);
  });

  it('returns 0 for an empty list', () => {
    expect(clampIndex(5, 0)).toBe(0);
  });
});

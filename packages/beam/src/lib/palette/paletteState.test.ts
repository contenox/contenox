import { describe, expect, it } from 'vitest';
import type { FrecencyStore } from './frecency';
import {
  computeResults,
  createPaletteSearcher,
  INITIAL_PALETTE_STATE,
  paletteItemFields,
  paletteReducer,
  wrapIndex,
} from './paletteState';
import type { PaletteItem, PaletteItemType } from './types';

const item = (over: Partial<PaletteItem> & { id: string; type: PaletteItemType }): PaletteItem => ({
  title: over.id,
  icon: null,
  action: () => {},
  ...over,
});

const NOW = 1_000_000_000_000;

describe('paletteReducer', () => {
  it('open resets to a blank query at the top', () => {
    const s = paletteReducer({ open: false, query: 'x', selected: 3 }, { type: 'open' });
    expect(s).toEqual({ open: true, query: '', selected: 0 });
  });

  it('toggle flips open/closed', () => {
    expect(paletteReducer(INITIAL_PALETTE_STATE, { type: 'toggle' }).open).toBe(true);
    expect(paletteReducer({ open: true, query: '', selected: 0 }, { type: 'toggle' }).open).toBe(false);
  });

  it('typing re-ranks from the top', () => {
    const s = paletteReducer({ open: true, query: '', selected: 5 }, { type: 'setQuery', query: 'de' });
    expect(s).toEqual({ open: true, query: 'de', selected: 0 });
  });

  it('move wraps around the result list', () => {
    const up = paletteReducer({ open: true, query: '', selected: 0 }, { type: 'move', delta: -1, count: 3 });
    expect(up.selected).toBe(2);
    const down = paletteReducer({ open: true, query: '', selected: 2 }, { type: 'move', delta: 1, count: 3 });
    expect(down.selected).toBe(0);
  });

  it('setSelected clamps into range', () => {
    expect(paletteReducer(INITIAL_PALETTE_STATE, { type: 'setSelected', index: 9, count: 3 }).selected).toBe(2);
  });
});

describe('wrapIndex', () => {
  it('returns 0 for an empty list', () => {
    expect(wrapIndex(3, 0)).toBe(0);
  });
  it('wraps both directions', () => {
    expect(wrapIndex(-1, 4)).toBe(3);
    expect(wrapIndex(4, 4)).toBe(0);
  });
});

describe('paletteItemFields', () => {
  it('weights the title as the sole highlight field and always includes the id', () => {
    const fields = paletteItemFields(item({ id: 'x1', type: 'mission', title: 'Deploy', keywords: ['k'] }));
    expect(fields[0]).toMatchObject({ text: 'Deploy', highlight: true });
    expect(fields.some(f => f.text === 'x1')).toBe(true);
    expect(fields.some(f => f.text === 'k')).toBe(true);
  });
});

describe('computeResults — recents (blank query)', () => {
  it('surfaces pending asks before actions when nothing has been used', () => {
    const items = [item({ id: 'a', type: 'action' }), item({ id: 'i', type: 'inbox' })];
    const results = computeResults(createPaletteSearcher(items), items, '', {}, NOW);
    expect(results.map(r => r.item.id)).toEqual(['i', 'a']);
  });

  it('lets frecency override the type priority', () => {
    const items = [item({ id: 'i', type: 'inbox' }), item({ id: 'a', type: 'action' })];
    const store: FrecencyStore = { a: { count: 5, lastUsed: NOW } };
    const results = computeResults(createPaletteSearcher(items), items, '', store, NOW);
    expect(results[0].item.id).toBe('a');
  });
});

describe('computeResults — query ranking', () => {
  it('blends frecency into an otherwise-equal match', () => {
    const items = [
      item({ id: 'A', type: 'mission', title: 'deploy' }),
      item({ id: 'B', type: 'mission', title: 'deploy' }),
    ];
    const store: FrecencyStore = { B: { count: 4, lastUsed: NOW } };
    const results = computeResults(createPaletteSearcher(items), items, 'deploy', store, NOW);
    expect(results[0].item.id).toBe('B');
  });

  it('never lets frecency resurrect an unmatched item', () => {
    const items = [
      item({ id: 'match', type: 'mission', title: 'deploy' }),
      item({ id: 'hot', type: 'mission', title: 'unrelated' }),
    ];
    const store: FrecencyStore = { hot: { count: 50, lastUsed: NOW } };
    const results = computeResults(createPaletteSearcher(items), items, 'deploy', store, NOW);
    expect(results.map(r => r.item.id)).toEqual(['match']);
  });

  it('caps the result count', () => {
    const items = Array.from({ length: 60 }, (_, i) =>
      item({ id: `x${i}`, type: 'mission', title: `deploy ${i}` }),
    );
    const results = computeResults(createPaletteSearcher(items), items, 'deploy', {}, NOW, 5);
    expect(results).toHaveLength(5);
  });
});

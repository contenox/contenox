import { describe, expect, it } from 'vitest';
import {
  blend,
  frecencyBoost,
  pruneStore,
  recordUsage,
  type FrecencyStore,
} from './frecency';

const HALF_LIFE_MS = 3 * 24 * 60 * 60 * 1000;
const NOW = 1_000_000_000_000;

describe('frecencyBoost — neutrality', () => {
  it('is exactly 1 for a never-used item', () => {
    expect(frecencyBoost(undefined, NOW)).toBe(1);
  });

  it('is exactly 1 for a zeroed entry', () => {
    expect(frecencyBoost({ count: 0, lastUsed: NOW }, NOW)).toBe(1);
  });
});

describe('frecencyBoost — usage lifts, recency decays', () => {
  it('grows with use count at equal recency', () => {
    const once = frecencyBoost({ count: 1, lastUsed: NOW }, NOW);
    const many = frecencyBoost({ count: 8, lastUsed: NOW }, NOW);
    expect(many).toBeGreaterThan(once);
    expect(once).toBeGreaterThan(1);
  });

  it('decays toward 1 as the last use recedes', () => {
    const fresh = frecencyBoost({ count: 4, lastUsed: NOW }, NOW);
    const oneHalfLife = frecencyBoost({ count: 4, lastUsed: NOW - HALF_LIFE_MS }, NOW);
    const ancient = frecencyBoost({ count: 4, lastUsed: NOW - 100 * HALF_LIFE_MS }, NOW);
    expect(fresh).toBeGreaterThan(oneHalfLife);
    expect(oneHalfLife).toBeGreaterThan(ancient);
    expect(ancient).toBeCloseTo(1, 5);
  });

  it('halves the extra boost at exactly one half-life', () => {
    const extraFresh = frecencyBoost({ count: 4, lastUsed: NOW }, NOW) - 1;
    const extraHalf = frecencyBoost({ count: 4, lastUsed: NOW - HALF_LIFE_MS }, NOW) - 1;
    expect(extraHalf).toBeCloseTo(extraFresh / 2, 6);
  });
});

describe('recordUsage', () => {
  it('increments the count and stamps the time without mutating the input', () => {
    const store: FrecencyStore = { a: { count: 2, lastUsed: 1 } };
    const next = recordUsage(store, 'a', NOW);
    expect(next.a).toEqual({ count: 3, lastUsed: NOW });
    expect(store.a).toEqual({ count: 2, lastUsed: 1 }); // unchanged
  });

  it('creates an entry for a first-time id', () => {
    expect(recordUsage({}, 'b', NOW).b).toEqual({ count: 1, lastUsed: NOW });
  });
});

describe('blend', () => {
  it('multiplies the match score by the boost', () => {
    expect(blend(0.5, 1.6)).toBeCloseTo(0.8, 10);
  });
});

describe('pruneStore', () => {
  it('keeps the most-recently-used entries when over the cap', () => {
    const store: FrecencyStore = {};
    for (let i = 0; i < 260; i++) store[`id${i}`] = { count: 1, lastUsed: i };
    const pruned = pruneStore(store);
    expect(Object.keys(pruned).length).toBe(250);
    expect(pruned.id259).toBeDefined(); // newest kept
    expect(pruned.id0).toBeUndefined(); // oldest dropped
  });

  it('leaves a small store untouched', () => {
    const store: FrecencyStore = { a: { count: 1, lastUsed: 1 } };
    expect(pruneStore(store)).toBe(store);
  });
});

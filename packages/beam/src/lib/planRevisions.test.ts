import { describe, expect, it } from 'vitest';
import { formatRevisionDelta, latestRevision, revisionsNewestFirst } from './planRevisions';
import type { PlanRevisionSummary } from './types';

function rev(over: Partial<PlanRevisionSummary> = {}): PlanRevisionSummary {
  return {
    revision: 1,
    added: 0,
    removed: 0,
    pending: 0,
    inProgress: 0,
    completed: 0,
    at: new Date().toISOString(),
    ...over,
  };
}

describe('revisionsNewestFirst', () => {
  it('reverses the wire order (oldest-first) to newest-first without mutating the input', () => {
    const wire = [rev({ revision: 1 }), rev({ revision: 2 }), rev({ revision: 3 })];
    const out = revisionsNewestFirst(wire);
    expect(out.map(r => r.revision)).toEqual([3, 2, 1]);
    // input untouched
    expect(wire.map(r => r.revision)).toEqual([1, 2, 3]);
  });

  it('returns an empty array for an absent or empty ring (renders nothing)', () => {
    expect(revisionsNewestFirst(undefined)).toEqual([]);
    expect(revisionsNewestFirst(null)).toEqual([]);
    expect(revisionsNewestFirst([])).toEqual([]);
  });
});

describe('latestRevision', () => {
  it('is the final (newest) element of the oldest-first ring', () => {
    expect(latestRevision([rev({ revision: 1 }), rev({ revision: 2 })])?.revision).toBe(2);
  });
  it('is undefined for no history', () => {
    expect(latestRevision(undefined)).toBeUndefined();
  });
});

describe('formatRevisionDelta', () => {
  it('formats both sides with a real minus sign, omitting zero sides', () => {
    expect(formatRevisionDelta(2, 1)).toBe('+2/−1');
    expect(formatRevisionDelta(2, 0)).toBe('+2');
    expect(formatRevisionDelta(0, 3)).toBe('−3');
  });
  it('renders a no-op revision (reorder / status-only) as ±0, never blank', () => {
    expect(formatRevisionDelta(0, 0)).toBe('±0');
  });
});

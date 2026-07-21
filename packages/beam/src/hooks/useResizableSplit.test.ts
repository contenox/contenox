import { describe, expect, it } from 'vitest';
import { clampFraction, fractionFromPointer } from './useResizableSplit';

describe('clampFraction', () => {
  it('clamps below the minimum and above the maximum', () => {
    expect(clampFraction(0.05, 0.15, 0.85)).toBe(0.15);
    expect(clampFraction(0.95, 0.15, 0.85)).toBe(0.85);
  });

  it('passes an in-range value through', () => {
    expect(clampFraction(0.42, 0.15, 0.85)).toBe(0.42);
  });

  it('collapses a NaN (corrupt persisted value) to the minimum', () => {
    expect(clampFraction(NaN, 0.2, 0.8)).toBe(0.2);
  });

  it('treats the bounds as inclusive', () => {
    expect(clampFraction(0.15, 0.15, 0.85)).toBe(0.15);
    expect(clampFraction(0.85, 0.15, 0.85)).toBe(0.85);
  });
});

describe('fractionFromPointer', () => {
  it('maps a pointer at the container start to 0 and the end to 1', () => {
    expect(fractionFromPointer(100, 400, 100)).toBe(0);
    expect(fractionFromPointer(100, 400, 500)).toBe(1);
  });

  it('maps the midpoint to 0.5', () => {
    expect(fractionFromPointer(100, 400, 300)).toBe(0.5);
  });

  it('can return out-of-range values (the caller clamps)', () => {
    expect(fractionFromPointer(100, 400, 50)).toBeCloseTo(-0.125);
    expect(fractionFromPointer(100, 400, 700)).toBeCloseTo(1.5);
  });

  it('guards a zero-size container', () => {
    expect(fractionFromPointer(0, 0, 42)).toBe(0);
  });
});

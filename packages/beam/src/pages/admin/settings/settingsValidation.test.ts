import { describe, expect, it } from 'vitest';
import {
  isValidMaxTokens,
  resolveTelemetryEnabled,
  resolveUpdateCheckEnabled,
} from './settingsValidation';

describe('isValidMaxTokens', () => {
  it('accepts empty (clears the override)', () => {
    expect(isValidMaxTokens('')).toBe(true);
    expect(isValidMaxTokens('   ')).toBe(true);
  });

  it('accepts non-negative integers', () => {
    expect(isValidMaxTokens('0')).toBe(true);
    expect(isValidMaxTokens('8192')).toBe(true);
  });

  it('rejects negative numbers, decimals, and non-numeric text', () => {
    expect(isValidMaxTokens('-1')).toBe(false);
    expect(isValidMaxTokens('3.14')).toBe(false);
    expect(isValidMaxTokens('abc')).toBe(false);
  });
});

describe('resolveTelemetryEnabled', () => {
  it('is opt-in: off unless literally "true"', () => {
    expect(resolveTelemetryEnabled(undefined)).toBe(false);
    expect(resolveTelemetryEnabled('')).toBe(false);
    expect(resolveTelemetryEnabled('false')).toBe(false);
    expect(resolveTelemetryEnabled('TRUE')).toBe(false);
    expect(resolveTelemetryEnabled('true')).toBe(true);
  });
});

describe('resolveUpdateCheckEnabled', () => {
  it('is opt-out: on unless literally "false"', () => {
    expect(resolveUpdateCheckEnabled(undefined)).toBe(true);
    expect(resolveUpdateCheckEnabled('')).toBe(true);
    expect(resolveUpdateCheckEnabled('true')).toBe(true);
    expect(resolveUpdateCheckEnabled('false')).toBe(false);
  });
});

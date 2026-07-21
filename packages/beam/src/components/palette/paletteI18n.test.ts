import { describe, expect, it } from 'vitest';
import i18n from '../../i18n';

/**
 * The palette namespace is new, so pin the house rule that every key ships in
 * BOTH languages with an identical key set and no blank German strings — a
 * German operator must never fall through to English or a raw key in the one
 * surface reachable from every page.
 */
type Bundle = Record<string, Record<string, unknown>>;

describe('palette i18n — en/de parity', () => {
  it('defines the palette namespace in both languages with matching keys', () => {
    const en = i18n.getResourceBundle('en', 'translation') as Bundle;
    const de = i18n.getResourceBundle('de', 'translation') as Bundle;
    expect(en.palette).toBeDefined();
    expect(de.palette).toBeDefined();
    expect(Object.keys(de.palette).sort()).toEqual(Object.keys(en.palette).sort());
  });

  it('has a non-empty German string for every palette key', () => {
    const de = i18n.getResourceBundle('de', 'translation') as Bundle;
    for (const [key, value] of Object.entries(de.palette)) {
      expect(typeof value, `palette.${key} type`).toBe('string');
      expect((value as string).trim().length, `palette.${key} non-empty`).toBeGreaterThan(0);
    }
  });
});

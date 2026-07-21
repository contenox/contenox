import { describe, expect, it } from 'vitest';
import i18n from '../../../i18n';

/**
 * The inbox namespace gained new keys today (the operator-inbox merge's
 * "supervisor session ended" marker — see inboxPresentation.ts /
 * mergeOperatorInboxReports). Pin the house rule for the WHOLE namespace, not
 * just the new keys: every key ships in both languages with an identical key
 * set and no blank German strings, so a German operator never falls through
 * to English or a raw key on the inbox — the primary overnight-batch surface.
 */
type Bundle = Record<string, Record<string, unknown>>;

describe('inbox i18n — en/de parity', () => {
  it('defines the inbox namespace in both languages with matching keys', () => {
    const en = i18n.getResourceBundle('en', 'translation') as Bundle;
    const de = i18n.getResourceBundle('de', 'translation') as Bundle;
    expect(en.inbox).toBeDefined();
    expect(de.inbox).toBeDefined();
    expect(Object.keys(de.inbox).sort()).toEqual(Object.keys(en.inbox).sort());
  });

  it('has a non-empty German string for every inbox key', () => {
    const de = i18n.getResourceBundle('de', 'translation') as Bundle;
    for (const [key, value] of Object.entries(de.inbox)) {
      expect(typeof value, `inbox.${key} type`).toBe('string');
      expect((value as string).trim().length, `inbox.${key} non-empty`).toBeGreaterThan(0);
    }
  });
});

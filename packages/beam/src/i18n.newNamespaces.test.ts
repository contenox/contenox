import { describe, expect, it } from 'vitest';
import i18n from './i18n';

/**
 * The Beam Tier-1 slices add three namespaces (workspace-root picker, mission
 * inspector tabs, host terminal); the Tier-2/3 flagship slices add four more
 * (changed-files review, scope surfacing, workspace search, plan-revision feed).
 * This pins the house rule that every key ships in BOTH languages: each new
 * namespace must exist in en and de with an identical key set, so a German
 * operator never falls through to an English string (or a raw key).
 */
const NEW_NAMESPACES = [
  'roots',
  'projects',
  'inspector',
  'hostTerminal',
  'changes',
  'scope',
  'workspaceSearch',
  'revisions',
] as const;

type Bundle = Record<string, Record<string, unknown>>;

describe('i18n Tier-1 namespaces — en/de parity', () => {
  it('defines each new namespace in both languages with matching keys', () => {
    const en = i18n.getResourceBundle('en', 'translation') as Bundle;
    const de = i18n.getResourceBundle('de', 'translation') as Bundle;

    for (const ns of NEW_NAMESPACES) {
      expect(en[ns], `en.${ns} present`).toBeDefined();
      expect(de[ns], `de.${ns} present`).toBeDefined();
      expect(Object.keys(de[ns]).sort(), `${ns} key parity`).toEqual(
        Object.keys(en[ns]).sort(),
      );
    }
  });

  it('has non-empty German strings for every new key (no blank fallthroughs)', () => {
    const de = i18n.getResourceBundle('de', 'translation') as Bundle;
    for (const ns of NEW_NAMESPACES) {
      for (const [key, value] of Object.entries(de[ns])) {
        expect(typeof value, `${ns}.${key} type`).toBe('string');
        expect((value as string).trim().length, `${ns}.${key} non-empty`).toBeGreaterThan(0);
      }
    }
  });
});

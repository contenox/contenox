import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../i18n';
import type { MissionChangeScope } from '../lib/types';
import { ScopeAnomalyChip, ScopeBadge } from './ScopeStatus';

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

describe('ScopeBadge', () => {
  it('renders the quiet file/directory count', () => {
    const scope: MissionChangeScope = { files: 3, dirs: 2, anomaly: false };
    const html = renderToStaticMarkup(createElement(ScopeBadge, { scope }));
    expect(html).toContain('3 files / 2 directories');
  });
});

describe('ScopeAnomalyChip', () => {
  it('renders the loud advice chip only when anomaly is set', () => {
    const anomalous: MissionChangeScope = { files: 5, dirs: 4, anomaly: true };
    const html = renderToStaticMarkup(createElement(ScopeAnomalyChip, { scope: anomalous }));
    expect(html).toContain('Out of scope');
    // Advice, not a verdict — the tooltip says so.
    expect(html).toContain('Advice, not a verdict');
  });

  it('renders nothing when there is no anomaly', () => {
    const clean: MissionChangeScope = { files: 5, dirs: 4, anomaly: false };
    const html = renderToStaticMarkup(createElement(ScopeAnomalyChip, { scope: clean }));
    expect(html).toBe('');
  });
});

describe('ScopeStatus — German parity', () => {
  it('renders the German count label', async () => {
    await i18n.changeLanguage('de');
    const html = renderToStaticMarkup(
      createElement(ScopeBadge, { scope: { files: 3, dirs: 2, anomaly: false } }),
    );
    expect(html).toContain('3 Dateien / 2 Verzeichnisse');
    await i18n.changeLanguage('en');
  });
});

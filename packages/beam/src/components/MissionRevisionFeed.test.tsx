import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../i18n';
import type { PlanRevisionSummary } from '../lib/types';
import { MissionRevisionFeed, PlanRevisionLine } from './MissionRevisionFeed';

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

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

describe('MissionRevisionFeed', () => {
  it('renders revisions newest-first with delta and explanation', () => {
    const html = renderToStaticMarkup(
      createElement(MissionRevisionFeed, {
        revisions: [
          rev({ revision: 1, added: 3, removed: 0, explanation: 'Initial plan' }),
          rev({ revision: 2, added: 1, removed: 2, explanation: 'Reordered after the runner came back' }),
        ],
      }),
    );
    expect(html).toContain('Plan revisions');
    expect(html).toContain('+1/−2');
    expect(html).toContain('Reordered after the runner came back');
    // newest (revision 2) appears before oldest (revision 1)
    expect(html.indexOf('Reordered after the runner came back')).toBeLessThan(
      html.indexOf('Initial plan'),
    );
  });

  it('renders an honest placeholder when a revision carries no explanation', () => {
    const html = renderToStaticMarkup(
      createElement(MissionRevisionFeed, { revisions: [rev({ revision: 1, added: 2 })] }),
    );
    expect(html).toContain('(no explanation given)');
  });

  it('renders NOTHING for an absent ring (legacy / never-planned mission)', () => {
    expect(renderToStaticMarkup(createElement(MissionRevisionFeed, { revisions: undefined }))).toBe(
      '',
    );
    expect(renderToStaticMarkup(createElement(MissionRevisionFeed, { revisions: [] }))).toBe('');
  });
});

describe('PlanRevisionLine', () => {
  it('shows only the newest revision as a compact one-liner', () => {
    const html = renderToStaticMarkup(
      createElement(PlanRevisionLine, {
        revisions: [rev({ revision: 1, added: 2 }), rev({ revision: 2, added: 0, removed: 1, explanation: 'Trimmed a step' })],
      }),
    );
    expect(html).toContain('−1');
    expect(html).toContain('Trimmed a step');
  });

  it('renders nothing without history', () => {
    expect(renderToStaticMarkup(createElement(PlanRevisionLine, { revisions: null }))).toBe('');
  });
});

import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../i18n';
import type { MissionPlan, MissionPlanEntry } from '../lib/types';
import { MissionPlanPanel } from './MissionPlanPanel';

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function entry(over: Partial<MissionPlanEntry> & { id: string }): MissionPlanEntry {
  return { content: 'do a thing', status: 'pending', priority: 'medium', ...over };
}

function render(plan?: MissionPlan | null): string {
  return renderToStaticMarkup(createElement(MissionPlanPanel, { plan }));
}

describe('MissionPlanPanel — the mission detail plan checklist', () => {
  it('renders NO panel for an unplanned mission (absent, or the zero Plan)', () => {
    expect(render(undefined)).toBe('');
    expect(render({ entries: null, revision: 0 })).toBe('');
    expect(render({ entries: [], revision: 0 })).toBe('');
  });

  it('renders the revision, the entries, and their status/priority for a mixed plan', () => {
    const html = render({
      entries: [
        entry({ id: '1', content: 'Clone the repository', status: 'completed', priority: 'high' }),
        entry({ id: '2', content: 'Reproduce the flake', status: 'in_progress', priority: 'medium' }),
        entry({ id: '3', content: 'Write the fix', status: 'pending', priority: 'low' }),
      ],
      revision: 3,
    });
    expect(html).toContain('Plan');
    expect(html).toContain('Revision 3');
    expect(html).toContain('1/3 steps');
    expect(html).toContain('Clone the repository');
    expect(html).toContain('Reproduce the flake');
    expect(html).toContain('Write the fix');
    // Status chips for each lifecycle state, and priority markers.
    expect(html).toContain('Done');
    expect(html).toContain('In progress');
    expect(html).toContain('Pending');
    expect(html).toContain('High');
    expect(html).toContain('Low');
  });

  it('shows the latest explanation as the "why it changed" line when present, omits it otherwise', () => {
    const withWhy = render({
      entries: [entry({ id: '1' })],
      revision: 2,
      explanation: 'Dropped the DB step — it was already done upstream.',
    });
    expect(withWhy).toContain('Why it changed');
    expect(withWhy).toContain('Dropped the DB step — it was already done upstream.');

    const withoutWhy = render({ entries: [entry({ id: '1' })], revision: 1 });
    expect(withoutWhy).not.toContain('Why it changed');
  });
});

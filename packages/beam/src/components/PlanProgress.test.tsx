import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../i18n';
import type { MissionPlan, MissionPlanEntry } from '../lib/types';
import { PlanProgress } from './PlanProgress';

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function entry(over: Partial<MissionPlanEntry> & { id: string }): MissionPlanEntry {
  return { content: 'do a thing', status: 'pending', priority: 'medium', ...over };
}

function render(plan?: MissionPlan | null): string {
  return renderToStaticMarkup(createElement(PlanProgress, { plan }));
}

describe('PlanProgress — the compact step fragment', () => {
  it('renders NOTHING when there is no plan (no "0/0" shell)', () => {
    expect(render(undefined)).toBe('');
    expect(render({ entries: [], revision: 0 })).toBe('');
    expect(render({ entries: null, revision: 0 })).toBe('');
  });

  it('shows the completed/total step count', () => {
    const html = render({
      entries: [
        entry({ id: '1', status: 'completed' }),
        entry({ id: '2', status: 'pending' }),
        entry({ id: '3', status: 'pending' }),
      ],
      revision: 2,
    });
    expect(html).toContain('1/3 steps');
  });

  it('surfaces an in-progress marker only when a step is mid-flight', () => {
    const active = render({
      entries: [entry({ id: '1', status: 'in_progress' }), entry({ id: '2', status: 'pending' })],
      revision: 1,
    });
    // The active-state title names the in-progress count; the idle one does not.
    expect(active).toContain('1 in progress');

    const idle = render({
      entries: [entry({ id: '1', status: 'completed' }), entry({ id: '2', status: 'pending' })],
      revision: 1,
    });
    expect(idle).not.toContain('in progress');
  });
});

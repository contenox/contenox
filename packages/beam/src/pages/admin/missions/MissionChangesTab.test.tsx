import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import { missionKeys } from '../../../lib/queryKeys';
import type { MissionChangesData } from '../../../hooks/useMissionChanges';
import MissionChangesTab from './MissionChangesTab';

// The changes fetch is never exercised — every case seeds the query cache — but
// mock the module so an unseeded path can never hit the network in a test.
vi.mock('../../../lib/api', () => ({
  api: { getMissionChanges: vi.fn(async () => null), getMissionChangeDiff: vi.fn() },
}));

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function render(id: string, data: MissionChangesData): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(missionKeys.changes(id), data);
  return renderToStaticMarkup(
    createElement(
      QueryClientProvider,
      { client },
      createElement(MissionChangesTab, { missionId: id }),
    ),
  );
}

describe('MissionChangesTab', () => {
  it('degrades to a graceful "unavailable" state when change tracking is absent (404 → null)', () => {
    const html = render('m1', null);
    expect(html).toContain('Change tracking unavailable');
  });

  it('says so plainly when a mission has no changes', () => {
    const html = render('m1', { files: [], incomplete: false, scope: { files: 0, dirs: 0, anomaly: false } });
    expect(html).toContain('No file changes recorded');
  });

  it('lists changed files in server (DOI) order, with the attention hint and status chips', () => {
    const html = render('m1', {
      files: [
        { path: '/repo/src/hot.ts', status: 'modified', score: 40 },
        { path: '/repo/README.md', status: 'added', score: 5 },
      ],
      incomplete: false,
      scope: { files: 2, dirs: 2, anomaly: false },
    });
    expect(html).toContain('Sorted by attention');
    expect(html).toContain('hot.ts');
    expect(html).toContain('README.md');
    // DOI order preserved: the higher-score file renders first.
    expect(html.indexOf('hot.ts')).toBeLessThan(html.indexOf('README.md'));
    // Rows are collapsed by default — no Monaco/diff in the initial paint.
    expect(html).not.toContain('Loading diff');
  });

  it('states the capped-list truth honestly when incomplete', () => {
    const html = render('m1', {
      files: [{ path: '/repo/a.ts', status: 'modified', score: 1 }],
      incomplete: true,
      scope: { files: 1, dirs: 1, anomaly: false },
    });
    expect(html).toContain('Showing the first 100 changed files');
  });
});

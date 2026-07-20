import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import { missionKeys } from '../../../lib/queryKeys';
import type { Mission, MissionReport } from '../../../lib/types';
import MissionDetailPage from './MissionDetailPage';

vi.mock('../../../lib/api', () => ({
  api: {
    getMission: vi.fn(),
    listMissionReports: vi.fn(async () => []),
  },
}));

/**
 * `@testing-library/react` is not a dependency of `packages/beam` (see
 * PermissionCard.test.tsx). The page reads its id via useParams, which only
 * resolves through an actual route match — MemoryRouter alone is not enough
 * (verified empirically: a bare child of MemoryRouter sees an undefined
 * param), so every render here goes through a real <Routes><Route
 * path="/missions/:id" .../></Routes>.
 */
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function baseMission(over: Partial<Mission> = {}): Mission {
  return {
    id: 'mission-1',
    intent: 'Investigate the flaky nightly test',
    agentName: 'Researcher',
    hitlPolicyName: 'hitl-policy-dev.json',
    status: 'open',
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    ...over,
  };
}

function renderDetail(
  id: string,
  mission: Mission | undefined,
  reports: MissionReport[] = [],
): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  if (mission) client.setQueryData(missionKeys.detail(id), mission);
  client.setQueryData(missionKeys.reports(id), reports);
  return renderToStaticMarkup(
    createElement(
      MemoryRouter,
      { initialEntries: [`/missions/${id}`] },
      createElement(
        QueryClientProvider,
        { client },
        createElement(
          Routes,
          null,
          createElement(Route, {
            path: '/missions/:id',
            element: createElement(MissionDetailPage),
          }),
        ),
      ),
    ),
  );
}

/**
 * A disabled/never-fetched query (no data seeded) reads as `isLoading`, not
 * as `error` — a real 404 is an error status the query cache only carries
 * once a fetch has actually resolved, which this DOM-less harness cannot
 * await (see PermissionCard.test.tsx's doc comment on the harness's limits).
 * `setState` on a built-but-unfetched query is the closest faithful
 * reproduction: it puts the cache in exactly the shape a real 404 response
 * would leave it in, without needing a live fetch to get there.
 *
 * `retryOnMount: false` is required here (verified empirically): with the
 * default `true`, mounting an observer over an already-errored query
 * schedules an immediate retry and the FIRST synchronous render already
 * reflects that as `status: 'pending'` / `isLoading: true` — the seeded
 * error is real but invisible on the very first paint this harness can see.
 */
function renderDetailWithError(id: string): string {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, retryOnMount: false } },
  });
  client
    .getQueryCache()
    .build(client, { queryKey: missionKeys.detail(id) })
    .setState({
      status: 'error',
      error: new Error('mission not found'),
      fetchStatus: 'idle',
      errorUpdateCount: 1,
      errorUpdatedAt: Date.now(),
      dataUpdatedAt: 0,
    });
  client.setQueryData(missionKeys.reports(id), []);
  return renderToStaticMarkup(
    createElement(
      MemoryRouter,
      { initialEntries: [`/missions/${id}`] },
      createElement(
        QueryClientProvider,
        { client },
        createElement(
          Routes,
          null,
          createElement(Route, {
            path: '/missions/:id',
            element: createElement(MissionDetailPage),
          }),
        ),
      ),
    ),
  );
}

describe('MissionDetailPage — facts', () => {
  it('renders the intent as the page heading, plus agent, envelope, and status', () => {
    const html = renderDetail(
      'mission-1',
      baseMission({ intent: 'Migrate the staging database', status: 'landed' }),
    );
    expect(html).toContain('Migrate the staging database');
    expect(html).toContain('Researcher');
    expect(html).toContain('hitl-policy-dev.json');
    expect(html).toContain('Landed');
  });

  it('shows session and instance ids when bound, and a plain placeholder when not', () => {
    const bound = renderDetail(
      'mission-1',
      baseMission({ sessionId: 'sess-1', instanceId: 'inst-1' }),
    );
    expect(bound).toContain('sess-1');
    expect(bound).toContain('inst-1');

    const unbound = renderDetail(
      'mission-1',
      baseMission({ sessionId: undefined, instanceId: undefined }),
    );
    expect(unbound).toContain('—');
  });

  it('is honest that the mission has never reported', () => {
    const html = renderDetail('mission-1', baseMission({ lastHeartbeat: undefined }));
    expect(html).toContain('Never reported');
  });

  it('renders a real heartbeat as relative time, not the never-reported label', () => {
    const html = renderDetail(
      'mission-1',
      baseMission({ lastHeartbeat: new Date(Date.now() - 10 * 60 * 1000).toISOString() }),
    );
    expect(html).not.toContain('Never reported');
    expect(html).toContain('minute');
  });

  it('surfaces LastError prominently, and omits the notice entirely when absent', () => {
    const withError = renderDetail(
      'mission-1',
      baseMission({ lastError: 'connection to backend lost' }),
    );
    expect(withError).toContain('role="alert"');
    expect(withError).toContain('connection to backend lost');

    const withoutError = renderDetail('mission-1', baseMission({ lastError: undefined }));
    expect(withoutError).not.toContain('role="alert"');
  });

  it('renders a loading state, not a blank page or fabricated content, before data arrives', () => {
    const html = renderDetail('mission-1', undefined);
    expect(html).toContain('Loading missions…');
    expect(html).not.toContain('Mission not found');
  });

  it('renders an honest not-found state for a mission id that fails to load, fabricating nothing', () => {
    const html = renderDetailWithError('does-not-exist');
    expect(html).toContain('Mission not found');
  });
});

describe('MissionDetailPage — reports', () => {
  const reports: MissionReport[] = [
    {
      id: 'r-2',
      missionId: 'mission-1',
      kind: 'blocker',
      summary: 'Cannot reach the CI runner',
      detail: 'The runner has been unreachable for 10 minutes.',
      refs: ['/var/log/ci-runner.log'],
      createdAt: new Date().toISOString(),
    },
    {
      id: 'r-1',
      missionId: 'mission-1',
      kind: 'progress',
      summary: 'Cloned the repository',
      createdAt: new Date(Date.now() - 60_000).toISOString(),
    },
  ];

  it('renders every report — summary, kind label, detail toggle, and refs as plain text', () => {
    const html = renderDetail('mission-1', baseMission(), reports);

    expect(html).toContain('Cannot reach the CI runner');
    expect(html).toContain('Cloned the repository');
    expect(html).toContain('Blocker');
    expect(html).toContain('Progress');
    expect(html).toContain('Show detail');
    expect(html).toContain('The runner has been unreachable for 10 minutes.');
    expect(html).toContain('/var/log/ci-runner.log');
    // Refs are plain references, never rendered as a clickable anchor.
    expect(html).not.toContain('href="/var/log/ci-runner.log"');
  });

  it('preserves the API’s newest-first order rather than re-sorting client-side', () => {
    const html = renderDetail('mission-1', baseMission(), reports);
    expect(html.indexOf('Cannot reach the CI runner')).toBeLessThan(
      html.indexOf('Cloned the repository'),
    );
  });

  it('says so plainly when a mission has no reports yet', () => {
    const html = renderDetail('mission-1', baseMission(), []);
    expect(html).toContain('No reports yet.');
  });

  it('omits the detail toggle and refs list when a report carries neither', () => {
    const html = renderDetail('mission-1', baseMission(), [
      {
        id: 'r-1',
        missionId: 'mission-1',
        kind: 'result',
        summary: 'Finished successfully',
        createdAt: new Date().toISOString(),
      },
    ]);
    expect(html).toContain('Finished successfully');
    expect(html).not.toContain('Show detail');
    expect(html).not.toContain('Refs');
  });
});

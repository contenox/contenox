import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import { missionKeys } from '../../../lib/queryKeys';
import type { Mission } from '../../../lib/types';
import MissionListPage from './MissionListPage';

vi.mock('../../../lib/api', () => ({
  api: {
    listMissions: vi.fn(async () => []),
  },
}));

/**
 * `@testing-library/react` is not a dependency of `packages/beam` (see
 * PermissionCard.test.tsx / FleetPage.test.tsx). The list is rendered to
 * static markup with the mission list pre-seeded into the query cache, which
 * is enough to pin what a row shows for each fact this slice requires:
 * intent as the primary, linked fact; agent; status; envelope; and liveness
 * (heartbeat, rendered honestly when it has never been reported).
 */
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function baseMission(over: Partial<Mission> & { id: string }): Mission {
  return {
    intent: 'Investigate the flaky nightly test',
    agentName: 'Researcher',
    hitlPolicyName: 'hitl-policy-dev.json',
    status: 'open',
    createdAt: new Date().toISOString(),
    updatedAt: new Date().toISOString(),
    ...over,
  };
}

function renderList(missions: Mission[]): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(missionKeys.list(), missions);
  return renderToStaticMarkup(
    createElement(
      MemoryRouter,
      null,
      createElement(QueryClientProvider, { client }, createElement(MissionListPage)),
    ),
  );
}

describe('MissionListPage', () => {
  it('renders an empty fleet honestly, with no fabricated rows', () => {
    const html = renderList([]);
    expect(html).toContain('No missions yet');
  });

  it('shows intent as the primary, linked fact, plus agent, status, and envelope', () => {
    const html = renderList([
      baseMission({
        id: 'mission-1',
        intent: 'Investigate the flaky nightly test',
        agentName: 'Researcher',
        hitlPolicyName: 'hitl-policy-dev.json',
        status: 'landed',
      }),
    ]);

    expect(html).toContain('href="/missions/mission-1"');
    expect(html).toContain('Investigate the flaky nightly test');
    expect(html).toContain('Researcher');
    expect(html).toContain('Landed');
    expect(html).toContain('hitl-policy-dev.json');
  });

  it('is honest that a mission has never reported — distinct from a stale heartbeat', () => {
    const html = renderList([baseMission({ id: 'mission-1', lastHeartbeat: undefined })]);
    expect(html).toContain('Never reported');
  });

  it('renders an actual heartbeat as relative time, not the never-reported label', () => {
    const html = renderList([
      baseMission({
        id: 'mission-1',
        lastHeartbeat: new Date(Date.now() - 2 * 60 * 1000).toISOString(),
      }),
    ]);
    expect(html).not.toContain('Never reported');
    expect(html).toContain('minute');
  });

  it('shows LastError when present, and omits any error line when absent', () => {
    const withError = renderList([
      baseMission({ id: 'mission-1', lastError: 'agent process exited unexpectedly' }),
    ]);
    expect(withError).toContain('Last error');
    expect(withError).toContain('agent process exited unexpectedly');

    const withoutError = renderList([baseMission({ id: 'mission-1' })]);
    expect(withoutError).not.toContain('Last error');
  });

  it('offers a Fire a mission action', () => {
    const html = renderList([baseMission({ id: 'mission-1' })]);
    expect(html).toContain('Fire a mission');
  });
});

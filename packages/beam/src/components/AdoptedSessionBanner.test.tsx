import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../i18n';
import { missionKeys } from '../lib/queryKeys';
import type { Mission } from '../lib/types';
import { AdoptedSessionBanner } from './AdoptedSessionBanner';

vi.mock('../lib/api', () => ({
  api: { listMissions: vi.fn(async () => []) },
}));

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function mission(over: Partial<Mission> & { id: string; instanceId: string }): Mission {
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

function render(
  props: { instanceId: string; controller: boolean; agentName: string | null },
  missions: Mission[] = [],
): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(missionKeys.list(), missions);
  return renderToStaticMarkup(
    createElement(
      MemoryRouter,
      null,
      createElement(
        QueryClientProvider,
        { client },
        createElement(AdoptedSessionBanner, props),
      ),
    ),
  );
}

describe('AdoptedSessionBanner', () => {
  it('labels a controlling adopter "Taken over" and shows the durability caveat', () => {
    const html = render({ instanceId: 'inst-1', controller: true, agentName: 'Researcher' });
    expect(html).toContain('Taken over');
    expect(html).not.toContain('Observing');
    expect(html).toContain('Researcher');
    // The durability caveat is always present — the transcript begins at adoption.
    expect(html).toContain('durable from take-over onward');
  });

  it('labels an observer "Observing" (never assuming control it was not granted)', () => {
    const html = render({ instanceId: 'inst-1', controller: false, agentName: 'Researcher' });
    expect(html).toContain('Observing');
    expect(html).not.toContain('Taken over');
  });

  it('links back to the mission when one claims the adopted instance', () => {
    const html = render({ instanceId: 'inst-1', controller: true, agentName: 'Researcher' }, [
      mission({ id: 'mission-9', instanceId: 'inst-1' }),
    ]);
    expect(html).toContain('Back to the mission');
    expect(html).toContain('/missions/mission-9');
  });

  it('omits the mission link for an adopted board session that is not a mission', () => {
    const html = render({ instanceId: 'inst-orphan', controller: true, agentName: null }, [
      mission({ id: 'mission-9', instanceId: 'inst-other' }),
    ]);
    expect(html).not.toContain('Back to the mission');
  });
});

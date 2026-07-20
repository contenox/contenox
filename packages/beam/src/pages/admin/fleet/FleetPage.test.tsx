import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import { fleetKeys, missionKeys } from '../../../lib/queryKeys';
import type { FleetEntry, FleetInstanceState, InstanceStatus, Mission } from '../../../lib/types';
import FleetPage, { createStopFlow, stopConfirmCopy } from './FleetPage';

vi.mock('../../../lib/api', () => ({
  api: {
    getFleet: vi.fn(),
    stopInstance: vi.fn(async () => 'deleted'),
    cancelInstance: vi.fn(async () => 'cancelled'),
    listMissions: vi.fn(async () => []),
  },
}));

const { api } = await import('../../../lib/api');

/**
 * `@testing-library/react` is not a dependency of `packages/beam` (see
 * PermissionCard.test.tsx). The board is rendered to static markup with the
 * fleet pre-seeded into the query cache, which is enough to pin WHAT the board
 * offers per row; the two behaviours a click would exercise — the Stop confirm
 * gate and the cancel call shape — are pinned against the exported
 * `createStopFlow` / `stopConfirmCopy` the page actually uses, so no simulated
 * DOM event is needed.
 */
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function instance(over: Partial<InstanceStatus> & { id: string }): InstanceStatus {
  return {
    agentId: 'agent-1',
    agentName: 'Researcher',
    kind: 'claude-code',
    state: 'running',
    sessions: 0,
    viewers: 0,
    startedAt: new Date().toISOString(),
    sessionIds: [],
    ...over,
  };
}

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

/**
 * Renders the board with the fleet (and, optionally, the mission list — see
 * the M2 board-integration join) pre-seeded into the query cache. Wrapped in
 * a MemoryRouter: FleetPage now renders a `<Link>` to a matching mission's
 * detail page (react-router-dom throws when Link/useNavigate render with no
 * Router ancestor at all, regardless of whether that Link's branch is taken
 * in a given test).
 */
function renderBoard(fleet: FleetEntry[], missions: Mission[] = []): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(fleetKeys.list(), fleet);
  client.setQueryData(missionKeys.list(), missions);
  return renderToStaticMarkup(
    createElement(
      MemoryRouter,
      null,
      createElement(QueryClientProvider, { client }, createElement(FleetPage)),
    ),
  );
}

describe('FleetPage — session rows and per-instance actions', () => {
  it('renders every attached session as its own addressable row with its own Cancel', () => {
    const html = renderBoard([
      {
        agentId: 'agent-1',
        agentName: 'Researcher',
        kind: 'claude-code',
        instances: [instance({ id: 'inst-1', sessions: 2, sessionIds: ['sess-a', 'sess-b'] })],
      },
    ]);

    expect(html).toContain('sess-a');
    expect(html).toContain('sess-b');
    // One cancel affordance per session, addressed by session id.
    expect(html).toContain('Cancel the in-flight turn on session sess-a');
    expect(html).toContain('Cancel the in-flight turn on session sess-b');
  });

  it('offers Cancel all only once an instance has more than one session', () => {
    const many = renderBoard([
      {
        agentId: 'agent-1',
        agentName: 'Researcher',
        kind: 'claude-code',
        instances: [instance({ id: 'inst-1', sessions: 2, sessionIds: ['sess-a', 'sess-b'] })],
      },
    ]);
    const one = renderBoard([
      {
        agentId: 'agent-1',
        agentName: 'Researcher',
        kind: 'claude-code',
        instances: [instance({ id: 'inst-1', sessions: 1, sessionIds: ['sess-a'] })],
      },
    ]);

    expect(many).toContain('Cancel all turns');
    expect(one).not.toContain('Cancel all turns');
    // A single session still gets its own cancel — nothing is lost.
    expect(one).toContain('Cancel the in-flight turn on session sess-a');
  });

  it('says so plainly when an instance has no attached session', () => {
    const html = renderBoard([
      {
        agentId: 'agent-1',
        agentName: 'Researcher',
        kind: 'claude-code',
        instances: [instance({ id: 'inst-1' })],
      },
    ]);
    expect(html).toContain('No attached sessions.');
    expect(html).not.toContain('Cancel all turns');
  });

  it('offers Stop in every state — including the dead ones, where it is the only way to clear the row', () => {
    const states: FleetInstanceState[] = ['starting', 'running', 'stopped', 'warning', 'error'];
    for (const state of states) {
      const html = renderBoard([
        {
          agentId: 'agent-1',
          agentName: 'Researcher',
          kind: 'claude-code',
          instances: [instance({ id: `inst-${state}`, state })],
        },
      ]);
      expect(html, `Stop must be offered in state ${state}`).toContain(
        `Stop instance inst-${state}`,
      );
    }
  });

  it('keeps the attention strip: error/warning instances stay hoisted above the per-agent sections', () => {
    const html = renderBoard([
      {
        agentId: 'agent-1',
        agentName: 'Researcher',
        kind: 'claude-code',
        instances: [instance({ id: 'inst-bad', state: 'error' })],
      },
    ]);
    expect(html).toContain('Needs attention');
    expect(html.indexOf('Needs attention')).toBeLessThan(html.lastIndexOf('inst-bad'));
  });
});

describe('FleetPage — the Stop confirm gate', () => {
  it('renders no confirm dialog and issues no stop merely by rendering the board', () => {
    const html = renderBoard([
      {
        agentId: 'agent-1',
        agentName: 'Researcher',
        kind: 'claude-code',
        instances: [instance({ id: 'inst-1' })],
      },
    ]);

    expect(html).not.toContain('Stop this instance?');
    expect(html).not.toContain('Clear this dead instance?');
    expect(html).not.toContain('aria-modal');
    expect(api.stopInstance).not.toHaveBeenCalled();
  });

  it('requesting a stop opens the confirm and does NOT call the mutation', () => {
    const stop = vi.fn();
    const setTarget = vi.fn();
    const flow = createStopFlow(stop, setTarget);
    const inst = instance({ id: 'inst-1' });

    flow.request(inst);

    expect(setTarget).toHaveBeenCalledWith(inst);
    expect(stop).not.toHaveBeenCalled();
  });

  it('only confirming stops — and it closes the dialog so a second click cannot re-fire it', () => {
    const stop = vi.fn();
    const setTarget = vi.fn();
    const flow = createStopFlow(stop, setTarget);
    const inst = instance({ id: 'inst-1' });

    flow.confirm(inst);

    expect(stop).toHaveBeenCalledExactlyOnceWith('inst-1');
    expect(setTarget).toHaveBeenCalledWith(null);
  });

  it('declining closes the dialog without stopping anything', () => {
    const stop = vi.fn();
    const setTarget = vi.fn();
    createStopFlow(stop, setTarget).dismiss();

    expect(setTarget).toHaveBeenCalledWith(null);
    expect(stop).not.toHaveBeenCalled();
  });
});

describe('stopConfirmCopy — honest about which cost is being paid', () => {
  it('a live instance gets the kill-a-working-agent copy', () => {
    for (const state of ['running', 'starting'] as FleetInstanceState[]) {
      expect(stopConfirmCopy(state)).toEqual({
        title: 'fleet.stop_confirm_title',
        body: 'fleet.stop_confirm_body',
        confirm: 'fleet.stop_confirm_confirm',
      });
    }
  });

  it('a dead instance gets the clear-the-row copy instead', () => {
    for (const state of ['stopped', 'warning', 'error'] as FleetInstanceState[]) {
      expect(stopConfirmCopy(state)).toEqual({
        title: 'fleet.stop_confirm_title_dead',
        body: 'fleet.stop_confirm_body_dead',
        confirm: 'fleet.stop_confirm_confirm_dead',
      });
    }
  });

  it('both bodies admit the context loss and the board’s own blind spot', () => {
    const live = i18n.t(stopConfirmCopy('running').body);
    const dead = i18n.t(stopConfirmCopy('error').body);

    expect(live).toContain('conversation context');
    expect(live).toContain('no restart action');
    expect(live).toContain('another tab'); // it cannot know whether a chat is open
    expect(dead).toContain('context is lost');
    expect(dead).toContain('no restart action');
    expect(dead).toContain('cannot tell');
  });
});

describe('FleetPage — mission intent joined onto rows (M2 board integration)', () => {
  it('shows the intent of the mission bound to a row’s instance', () => {
    const html = renderBoard(
      [
        {
          agentId: 'agent-1',
          agentName: 'Researcher',
          kind: 'claude-code',
          instances: [instance({ id: 'inst-1' })],
        },
      ],
      [
        mission({
          id: 'mission-1',
          instanceId: 'inst-1',
          intent: 'Investigate the flaky nightly test',
        }),
      ],
    );

    expect(html).toContain('Investigate the flaky nightly test');
    expect(html).toContain('href="/missions/mission-1"');
  });

  it('renders nothing extra for an instance with no bound mission', () => {
    const html = renderBoard(
      [
        {
          agentId: 'agent-1',
          agentName: 'Researcher',
          kind: 'claude-code',
          instances: [instance({ id: 'inst-1' })],
        },
      ],
      [mission({ id: 'mission-1', instanceId: 'inst-other', intent: 'Unrelated mission' })],
    );

    expect(html).not.toContain('Unrelated mission');
    expect(html).not.toContain('undefined');
  });

  it('joins onto the attention strip too, so an erroring instance still shows what it was sent to do', () => {
    const html = renderBoard(
      [
        {
          agentId: 'agent-1',
          agentName: 'Researcher',
          kind: 'claude-code',
          instances: [instance({ id: 'inst-bad', state: 'error' })],
        },
      ],
      [
        mission({
          id: 'mission-1',
          instanceId: 'inst-bad',
          intent: 'Migrate the staging database',
        }),
      ],
    );

    expect(html).toContain('Needs attention');
    expect(html).toContain('Migrate the staging database');
  });

  it('degrades silently when the mission feed itself has no data — the fleet board never depends on it', () => {
    const html = renderBoard([
      {
        agentId: 'agent-1',
        agentName: 'Researcher',
        kind: 'claude-code',
        instances: [instance({ id: 'inst-1' })],
      },
    ]);

    expect(html).toContain('inst-1');
    expect(html).not.toContain('undefined');
  });
});

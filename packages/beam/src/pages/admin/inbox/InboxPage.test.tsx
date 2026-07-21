import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { MemoryRouter } from 'react-router-dom';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import { approvalKeys, fleetKeys, missionKeys, operatorInboxKeys } from '../../../lib/queryKeys';
import type {
  FleetEntry,
  HITLApproval,
  InstanceStatus,
  Mission,
  MissionReport,
  OperatorInboxItem,
} from '../../../lib/types';
import InboxPage from './InboxPage';

vi.mock('../../../lib/api', () => ({
  api: {
    listApprovals: vi.fn(async () => []),
    answerApproval: vi.fn(async () => 'approved'),
    listMissions: vi.fn(async () => []),
    listMissionReports: vi.fn(async () => []),
    getFleet: vi.fn(async () => []),
    stopInstance: vi.fn(async () => 'deleted'),
    cancelInstance: vi.fn(async () => 'cancelled'),
    getOperatorInbox: vi.fn(async () => []),
  },
}));

/**
 * `@testing-library/react` is not a dependency of packages/beam (see
 * FleetPage.test.tsx). The inbox is rendered to static markup with its five
 * feeds pre-seeded into the query cache — approvals, the mission list, each
 * mission's reports, the operator inbox, and the fleet — which is enough to
 * pin the triage layout: what each tier offers and in what order. The
 * answer-click behaviour (a direct mutation, no confirm gate) is pinned
 * separately against useAnswerApproval in hooks/useApprovals.test.tsx.
 * lastSeen-driven "new" badges depend on localStorage, absent under static
 * render, so those are covered in inboxPresentation.test.ts instead.
 */
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function approval(over: Partial<HITLApproval> & { id: string }): HITLApproval {
  return {
    toolsName: 'local_fs',
    toolName: 'write_file',
    state: 'pending',
    createdAt: new Date().toISOString(),
    expiresAt: new Date(Date.now() + 3_600_000).toISOString(),
    ...over,
  };
}

function mission(over: Partial<Mission> & { id: string }): Mission {
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

function report(over: Partial<MissionReport> & { id: string; missionId: string }): MissionReport {
  return { kind: 'progress', summary: 'made progress', createdAt: new Date().toISOString(), ...over };
}

function instance(over: Partial<InstanceStatus> & { id: string }): InstanceStatus {
  return {
    agentId: 'a1',
    agentName: 'Researcher',
    kind: 'chain',
    state: 'running',
    sessions: 0,
    viewers: 0,
    startedAt: new Date().toISOString(),
    sessionIds: [],
    ...over,
  };
}

function renderInbox(opts: {
  approvals?: HITLApproval[];
  missions?: Mission[];
  reports?: Record<string, MissionReport[]>;
  fleet?: FleetEntry[];
  operatorInbox?: OperatorInboxItem[];
}): string {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  client.setQueryData(approvalKeys.list(), opts.approvals ?? []);
  client.setQueryData(missionKeys.list(), opts.missions ?? []);
  client.setQueryData(fleetKeys.list(), opts.fleet ?? []);
  client.setQueryData(operatorInboxKeys.list(), opts.operatorInbox ?? []);
  // Seed a reports feed for every mission so useInboxReports is not stuck
  // "loading" a mission's reports under static render.
  for (const m of opts.missions ?? []) {
    client.setQueryData(missionKeys.reports(m.id), opts.reports?.[m.id] ?? []);
  }
  return renderToStaticMarkup(
    createElement(
      MemoryRouter,
      null,
      createElement(QueryClientProvider, { client }, createElement(InboxPage)),
    ),
  );
}

describe('InboxPage — pending asks (tier 1, grouped by mission)', () => {
  it('renders an ask with its tool, policy, agent, grouped under its mission intent, with Allow + Deny', () => {
    const html = renderInbox({
      approvals: [
        approval({
          id: 'ask-1',
          policyName: 'hitl-policy-default.json',
          matchedRule: 1,
          agentName: 'reviewer',
          missionId: 'mission-1',
        }),
      ],
      missions: [mission({ id: 'mission-1', intent: 'Migrate the staging database' })],
    });

    expect(html).toContain('local_fs / write_file');
    expect(html).toContain('hitl-policy-default.json #1');
    expect(html).toContain('reviewer');
    // The mission intent is the group header, linking to the mission.
    expect(html).toContain('Migrate the staging database');
    expect(html).toContain('href="/missions/mission-1"');
    expect(html).toContain('Allow local_fs / write_file');
    expect(html).toContain('Deny local_fs / write_file');
    // Counts strip reflects one pending ask.
    expect(html).toContain('1 need you');
  });

  it('groups asks with no mission under the unattributed header, no blank rows', () => {
    const html = renderInbox({ approvals: [approval({ id: 'ask-1' })] });
    expect(html).toContain('local_fs / write_file');
    expect(html).toContain('Not tied to a mission');
    expect(html).not.toContain('undefined');
  });
});

describe('InboxPage — stalled units (tier 2)', () => {
  it('lists an error/warning unit with its agent, mission, and a link to the board', () => {
    const html = renderInbox({
      fleet: [
        {
          agentId: 'a1',
          agentName: 'Researcher',
          kind: 'chain',
          instances: [instance({ id: 'inst-bad', state: 'error' })],
        },
      ],
      missions: [mission({ id: 'mission-1', instanceId: 'inst-bad', intent: 'Crashed job' })],
    });

    expect(html).toContain('Stalled or failed units');
    expect(html).toContain('Crashed job');
    expect(html).toContain('View on the fleet board');
    expect(html).toContain('1 stalled');
  });
});

describe('InboxPage — the all-clear state', () => {
  it('says no unit needs you and how many are still running, not a generic empty', () => {
    const html = renderInbox({
      approvals: [],
      fleet: [
        {
          agentId: 'a1',
          agentName: 'Researcher',
          kind: 'chain',
          instances: [instance({ id: 'ok-1', state: 'running' }), instance({ id: 'ok-2', state: 'running' })],
        },
      ],
    });

    expect(html).toContain('No unit needs you.');
    expect(html).toContain('2 still running.');
    expect(html).toContain('2 running');
    // The still-running line links to the board rather than duplicating it.
    expect(html).toContain('Open the fleet board');
    // No answer buttons when nothing is pending.
    expect(html).not.toContain('Allow ');
  });
});

describe('InboxPage — what came back (tier 3, grouped by mission)', () => {
  it('clusters reports under their mission with kind and a link back', () => {
    const html = renderInbox({
      missions: [mission({ id: 'mission-1', intent: 'Migrate the staging database' })],
      reports: {
        'mission-1': [
          report({ id: 'r1', missionId: 'mission-1', kind: 'blocker', summary: 'disk full' }),
        ],
      },
    });

    expect(html).toContain('disk full');
    expect(html).toContain('Blocker');
    expect(html).toContain('Migrate the staging database');
    expect(html).toContain('href="/missions/mission-1"');
  });

  it('says so plainly when no reports have come back', () => {
    const html = renderInbox({ missions: [mission({ id: 'mission-1' })], reports: { 'mission-1': [] } });
    expect(html).toContain('No reports yet.');
    expect(html).not.toContain('undefined');
  });
});

describe('InboxPage — operator inbox merge (reports that reached no live supervisor)', () => {
  it('marks a parent_gone report louder without duplicating it', () => {
    const r1 = report({
      id: 'r1',
      missionId: 'mission-1',
      kind: 'result',
      summary: 'landed the migration',
    });
    const html = renderInbox({
      missions: [mission({ id: 'mission-1', intent: 'Migrate the staging database' })],
      reports: { 'mission-1': [r1] },
      operatorInbox: [
        {
          id: 'op1',
          missionId: 'mission-1',
          reason: 'parent_gone',
          parentSessionId: 'sess-gone',
          report: r1,
          createdAt: r1.createdAt,
        },
      ],
    });

    expect(html).toContain('Supervisor session ended');
    // The report renders once, not twice.
    expect(html.match(/landed the migration/g)).toHaveLength(1);
  });

  it('does not mark an ordinary operator_fired report — it merges as a first-class row, unbadged', () => {
    const r1 = report({
      id: 'r1',
      missionId: 'mission-1',
      kind: 'progress',
      summary: 'still working',
    });
    const html = renderInbox({
      missions: [mission({ id: 'mission-1' })],
      reports: { 'mission-1': [r1] },
      operatorInbox: [
        {
          id: 'op1',
          missionId: 'mission-1',
          reason: 'operator_fired',
          report: r1,
          createdAt: r1.createdAt,
        },
      ],
    });

    expect(html).toContain('still working');
    expect(html).not.toContain('Supervisor session ended');
  });
});

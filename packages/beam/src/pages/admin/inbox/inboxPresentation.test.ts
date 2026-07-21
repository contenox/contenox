import { describe, expect, it } from 'vitest';
import type {
  FleetEntry,
  FleetInstanceState,
  HITLApproval,
  InstanceStatus,
  Mission,
  MissionReport,
  OperatorInboxItem,
} from '../../../lib/types';
import {
  approvalAttribution,
  approvalPolicyLabel,
  approvalToolLabel,
  countNewReports,
  groupApprovalsByMission,
  groupReportsByMission,
  isNewReport,
  joinInboxReports,
  mergeOperatorInboxReports,
  missionsByInstanceId,
  missionsById,
  newestReportAt,
  runningSummary,
  stalledUnits,
  type InboxReportItem,
} from './inboxPresentation';

function mission(over: Partial<Mission> & { id: string }): Mission {
  return {
    intent: 'Investigate the flaky nightly test',
    agentName: 'Researcher',
    hitlPolicyName: 'hitl-policy-dev.json',
    status: 'open',
    createdAt: '2026-07-20T10:00:00Z',
    updatedAt: '2026-07-20T10:00:00Z',
    ...over,
  };
}

function report(over: Partial<MissionReport> & { id: string; createdAt: string }): MissionReport {
  return { missionId: 'm1', kind: 'progress', summary: 'did a thing', ...over };
}

function approval(over: Partial<HITLApproval> & { id: string }): HITLApproval {
  return {
    toolsName: 'local_fs',
    toolName: 'write_file',
    state: 'pending',
    createdAt: '2026-07-20T10:00:00Z',
    expiresAt: '2026-07-20T11:00:00Z',
    ...over,
  };
}

function instance(over: Partial<InstanceStatus> & { id: string }): InstanceStatus {
  return {
    agentId: 'a1',
    agentName: 'Researcher',
    kind: 'chain',
    state: 'running' as FleetInstanceState,
    sessions: 0,
    viewers: 0,
    startedAt: '2026-07-20T10:00:00Z',
    sessionIds: [],
    ...over,
  };
}

function entry(agentName: string, instances: InstanceStatus[]): FleetEntry {
  return { agentId: `${agentName}-id`, agentName, kind: 'chain', instances };
}

describe('joinInboxReports', () => {
  it('flattens, attributes each row to its mission, and sorts newest-first', () => {
    const m1 = mission({ id: 'm1' });
    const m2 = mission({ id: 'm2' });
    const byId = new Map<string, MissionReport[]>([
      ['m1', [report({ id: 'old', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' })]],
      ['m2', [report({ id: 'new', missionId: 'm2', createdAt: '2026-07-20T12:00:00Z' })]],
    ]);

    const items = joinInboxReports([m1, m2], byId);

    expect(items.map(i => i.report.id)).toEqual(['new', 'old']);
    expect(items.map(i => i.mission.id)).toEqual(['m2', 'm1']);
  });

  it('drops a report whose mission is not in the list and respects the limit', () => {
    const m1 = mission({ id: 'm1' });
    const byId = new Map<string, MissionReport[]>([
      [
        'm1',
        [
          report({ id: 'a', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' }),
          report({ id: 'b', missionId: 'm1', createdAt: '2026-07-20T11:00:00Z' }),
        ],
      ],
      ['ghost', [report({ id: 'g', missionId: 'ghost', createdAt: '2026-07-20T12:00:00Z' })]],
    ]);

    expect(joinInboxReports([m1], byId, 1).map(i => i.report.id)).toEqual(['b']);
  });

  it('is total over empty input', () => {
    expect(joinInboxReports([], new Map())).toEqual([]);
    expect(joinInboxReports([mission({ id: 'm1' })], new Map())).toEqual([]);
  });
});

describe('groupReportsByMission', () => {
  const items: InboxReportItem[] = [
    { mission: mission({ id: 'm2' }), report: report({ id: 'r2', missionId: 'm2', createdAt: '2026-07-20T12:00:00Z' }) },
    { mission: mission({ id: 'm1' }), report: report({ id: 'r1a', missionId: 'm1', createdAt: '2026-07-20T11:00:00Z' }) },
    { mission: mission({ id: 'm1' }), report: report({ id: 'r1b', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' }) },
  ];

  it('clusters by mission and orders clusters by their newest report', () => {
    const groups = groupReportsByMission(items);
    expect(groups.map(g => g.mission.id)).toEqual(['m2', 'm1']);
    expect(groups[1].items.map(i => i.report.id)).toEqual(['r1a', 'r1b']);
    expect(groups[0].newestAt).toBe('2026-07-20T12:00:00Z');
  });

  it('counts per-group new reports against lastSeen', () => {
    const groups = groupReportsByMission(items, '2026-07-20T10:00:00Z');
    const m1 = groups.find(g => g.mission.id === 'm1')!;
    const m2 = groups.find(g => g.mission.id === 'm2')!;
    expect(m2.newCount).toBe(1); // 12:00 > 10:00
    expect(m1.newCount).toBe(1); // 11:00 > 10:00, 09:00 not
  });
});

describe('isNewReport / countNewReports', () => {
  it('treats a first visit (no lastSeen) as nothing-new', () => {
    expect(isNewReport('2026-07-20T12:00:00Z', undefined)).toBe(false);
    const items: InboxReportItem[] = [
      { mission: mission({ id: 'm1' }), report: report({ id: 'r', missionId: 'm1', createdAt: '2026-07-20T12:00:00Z' }) },
    ];
    expect(countNewReports(items, undefined)).toBe(0);
  });

  it('marks only reports strictly newer than lastSeen', () => {
    expect(isNewReport('2026-07-20T12:00:00Z', '2026-07-20T11:00:00Z')).toBe(true);
    expect(isNewReport('2026-07-20T10:00:00Z', '2026-07-20T11:00:00Z')).toBe(false);
  });

  it('newestReportAt returns the max, or undefined when empty', () => {
    expect(newestReportAt([])).toBeUndefined();
    const items: InboxReportItem[] = [
      { mission: mission({ id: 'm1' }), report: report({ id: 'a', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' }) },
      { mission: mission({ id: 'm1' }), report: report({ id: 'b', missionId: 'm1', createdAt: '2026-07-20T12:00:00Z' }) },
    ];
    expect(newestReportAt(items)).toBe('2026-07-20T12:00:00Z');
  });
});

function operatorItem(
  over: Partial<OperatorInboxItem> & { id: string; missionId: string; report: MissionReport },
): OperatorInboxItem {
  return { reason: 'operator_fired', createdAt: '2026-07-20T10:00:00Z', ...over };
}

describe('mergeOperatorInboxReports', () => {
  it('passes items through unmarked when operatorItems is null/undefined/empty', () => {
    const items: InboxReportItem[] = [
      { mission: mission({ id: 'm1' }), report: report({ id: 'r1', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' }) },
    ];
    expect(mergeOperatorInboxReports(items, null, new Map())).toBe(items);
    expect(mergeOperatorInboxReports(items, undefined, new Map())).toBe(items);
    expect(mergeOperatorInboxReports(items, [], new Map())).toBe(items);
  });

  it('marks a report the join already carries with its operator-inbox provenance instead of duplicating it', () => {
    const r1 = report({ id: 'r1', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' });
    const items: InboxReportItem[] = [{ mission: mission({ id: 'm1' }), report: r1 }];
    const opItems = [
      operatorItem({
        id: 'op1',
        missionId: 'm1',
        report: r1,
        reason: 'parent_gone',
        parentSessionId: 'sess-gone',
      }),
    ];

    const merged = mergeOperatorInboxReports(items, opItems, new Map());

    expect(merged).toHaveLength(1);
    expect(merged[0].report.id).toBe('r1');
    expect(merged[0].operatorInbox).toEqual({
      reason: 'parent_gone',
      parentSessionId: 'sess-gone',
    });
  });

  it('adds an operator-inbox item the join has not fetched yet, attributed via missionById', () => {
    const m1 = mission({ id: 'm1' });
    const r1 = report({ id: 'r1', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' });
    const opItems = [
      operatorItem({ id: 'op1', missionId: 'm1', report: r1, reason: 'operator_fired' }),
    ];

    const merged = mergeOperatorInboxReports([], opItems, missionsById([m1]));

    expect(merged).toHaveLength(1);
    expect(merged[0].mission.id).toBe('m1');
    expect(merged[0].operatorInbox).toEqual({
      reason: 'operator_fired',
      parentSessionId: undefined,
    });
  });

  it('drops an operator-inbox item whose mission is not known locally, rather than fabricating one', () => {
    const r1 = report({ id: 'r1', missionId: 'ghost', createdAt: '2026-07-20T09:00:00Z' });
    const opItems = [operatorItem({ id: 'op1', missionId: 'ghost', report: r1 })];

    expect(mergeOperatorInboxReports([], opItems, new Map())).toEqual([]);
  });

  it('matches by mission id + report id, not report id alone', () => {
    const r1 = report({ id: 'r1', missionId: 'm1', createdAt: '2026-07-20T09:00:00Z' });
    const items: InboxReportItem[] = [{ mission: mission({ id: 'm1' }), report: r1 }];
    // Same report id, different mission — must not cross-match.
    const opItems = [
      operatorItem({
        id: 'op1',
        missionId: 'm2',
        report: report({ id: 'r1', missionId: 'm2', createdAt: '2026-07-20T09:00:00Z' }),
      }),
    ];

    const merged = mergeOperatorInboxReports(items, opItems, missionsById([mission({ id: 'm2' })]));

    expect(merged).toHaveLength(2);
    expect(merged.find(i => i.mission.id === 'm1')?.operatorInbox).toBeUndefined();
    expect(merged.find(i => i.mission.id === 'm2')?.operatorInbox?.reason).toBe('operator_fired');
  });
});

describe('groupApprovalsByMission', () => {
  it('clusters asks by mission, orders by freshest ask, and trails the unattributed group last', () => {
    const missionById = missionsById([mission({ id: 'm1', intent: 'One' }), mission({ id: 'm2', intent: 'Two' })]);
    const approvals = [
      approval({ id: 'a-m1', missionId: 'm1', createdAt: '2026-07-20T10:00:00Z' }),
      approval({ id: 'a-m2', missionId: 'm2', createdAt: '2026-07-20T12:00:00Z' }),
      approval({ id: 'a-none', createdAt: '2026-07-20T13:00:00Z' }), // no mission
    ];

    const groups = groupApprovalsByMission(approvals, missionById);

    // m2 (12:00) before m1 (10:00); the unattributed group is always last.
    expect(groups.map(g => g.missionId ?? '(none)')).toEqual(['m2', 'm1', '(none)']);
    expect(groups[0].mission?.intent).toBe('Two');
    expect(groups[2].mission).toBeUndefined();
    expect(groups[2].approvals.map(a => a.id)).toEqual(['a-none']);
  });

  it('keeps a group even when the mission record is not loaded', () => {
    const groups = groupApprovalsByMission(
      [approval({ id: 'a1', missionId: 'ghost' })],
      new Map(),
    );
    expect(groups).toHaveLength(1);
    expect(groups[0].missionId).toBe('ghost');
    expect(groups[0].mission).toBeUndefined();
  });
});

describe('stalledUnits', () => {
  it('surfaces only error/warning units, error first then newest, joined to their mission', () => {
    const fleet: FleetEntry[] = [
      entry('A', [
        instance({ id: 'ok', state: 'running' }),
        instance({ id: 'warn', state: 'warning', startedAt: '2026-07-20T10:00:00Z' }),
      ]),
      entry('B', [
        instance({ id: 'err-new', state: 'error', startedAt: '2026-07-20T12:00:00Z' }),
        instance({ id: 'err-old', state: 'error', startedAt: '2026-07-20T09:00:00Z' }),
      ]),
    ];
    const byInstance = missionsByInstanceId([
      mission({ id: 'm1', instanceId: 'err-new', intent: 'Migrate db' }),
    ]);

    const units = stalledUnits(fleet, byInstance);

    // error before warning; within error, newest startedAt first.
    expect(units.map(u => u.instance.id)).toEqual(['err-new', 'err-old', 'warn']);
    expect(units[0].mission?.intent).toBe('Migrate db');
    expect(units[1].mission).toBeUndefined();
  });

  it('is empty for a healthy fleet', () => {
    const fleet: FleetEntry[] = [entry('A', [instance({ id: 'ok', state: 'running' })])];
    expect(stalledUnits(fleet, new Map())).toEqual([]);
  });
});

describe('runningSummary', () => {
  it('counts running and starting units as live, ignoring the rest', () => {
    const fleet: FleetEntry[] = [
      entry('A', [
        instance({ id: '1', state: 'running' }),
        instance({ id: '2', state: 'starting' }),
        instance({ id: '3', state: 'error' }),
        instance({ id: '4', state: 'stopped' }),
      ]),
      { agentId: 'idle', agentName: 'Idle', kind: 'chain', instances: null },
    ];
    expect(runningSummary(fleet)).toEqual({ running: 1, starting: 1, live: 2 });
  });
});

describe('approvalToolLabel', () => {
  it('joins tools provider and tool, or shows just the present half', () => {
    expect(approvalToolLabel({ toolsName: 'local_fs', toolName: 'write_file' })).toBe(
      'local_fs / write_file',
    );
    expect(approvalToolLabel({ toolsName: '', toolName: 'write_file' })).toBe('write_file');
  });
});

describe('approvalPolicyLabel', () => {
  it('names policy and rule, policy alone, or nothing', () => {
    expect(approvalPolicyLabel({ policyName: 'p.json', matchedRule: 2 })).toBe('p.json #2');
    expect(approvalPolicyLabel({ policyName: 'p.json' })).toBe('p.json');
    expect(approvalPolicyLabel({})).toBeUndefined();
  });
});

describe('approvalAttribution', () => {
  it('omits absent/whitespace fields rather than rendering them blank', () => {
    expect(
      approvalAttribution(approval({ id: 'a1', agentName: 'reviewer', missionId: 'm1' })),
    ).toEqual({ agentName: 'reviewer', missionId: 'm1', instanceId: undefined, sessionId: undefined });
    expect(approvalAttribution(approval({ id: 'a1', agentName: '  ' })).agentName).toBeUndefined();
  });
});

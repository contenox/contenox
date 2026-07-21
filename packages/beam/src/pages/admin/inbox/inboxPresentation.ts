import type { TranslationKey } from '../../../i18n';
import type {
  FleetEntry,
  FleetInstanceState,
  HITLApproval,
  InstanceStatus,
  Mission,
  MissionReport,
  OperatorInboxItem,
  OperatorInboxReason,
} from '../../../lib/types';

/**
 * The inbox is the operating console for an overnight batch: an operator fires
 * many units in the evening, skims this page roughly once an hour, operates
 * entirely from here (answering asks) until everything lands, then reads the
 * reports to assess outcomes. Every helper below serves that hourly skim —
 * triage ordering, grouping by mission so concurrent missions do not interleave
 * into noise, and "what is new since my last look".
 *
 * All of it is pure and total, kept out of the page so the join and the triage
 * logic are testable without react-query and swappable for a server-side inbox
 * endpoint later (see useInboxReports and the report on the ideal endpoint).
 */

// ── Report feed ────────────────────────────────────────────────────────────

/** One row of the report feed: a report paired with the mission it belongs to. */
export type InboxReportItem = {
  report: MissionReport;
  mission: Mission;
  /**
   * Present when this report reached no live supervisor (runtime/operatorinbox)
   * — set by {@link mergeOperatorInboxReports}. `parentSessionId` is present
   * only for the `parent_gone` reason (the now-unreachable supervisor the
   * report was meant for).
   */
  operatorInbox?: { reason: OperatorInboxReason; parentSessionId?: string };
};

/** A mission's reports, clustered so 20 concurrent missions do not interleave. */
export type ReportGroup = {
  mission: Mission;
  items: InboxReportItem[];
  /** The group's newest report time — what the groups are ordered by. */
  newestAt: string;
  /** How many of the group's reports arrived since the operator's last look. */
  newCount: number;
};

/** How many joined report rows the inbox assembles at once, newest-first. */
export const DEFAULT_INBOX_REPORT_LIMIT = 200;

/**
 * Flattens every mission's reports into one newest-first feed, each row carrying
 * its mission. A report whose mission is absent from `missions` is dropped (it
 * cannot be attributed); an unparseable createdAt sorts last rather than
 * throwing.
 */
export function joinInboxReports(
  missions: Mission[],
  reportsByMissionId: Map<string, MissionReport[]>,
  limit: number = DEFAULT_INBOX_REPORT_LIMIT,
): InboxReportItem[] {
  const items: InboxReportItem[] = [];
  for (const mission of missions) {
    const reports = reportsByMissionId.get(mission.id) ?? [];
    for (const report of reports) {
      items.push({ report, mission });
    }
  }
  items.sort((a, b) => sortableTime(b.report.createdAt) - sortableTime(a.report.createdAt));
  return limit >= 0 ? items.slice(0, limit) : items;
}

/**
 * Clusters the flat report feed by mission and orders the clusters by their
 * newest report — so the mission that reported most recently is on top and its
 * rows stay together. `lastSeen` (an ISO timestamp, or undefined for a first
 * visit) drives the per-group `newCount` used to mark what arrived since the
 * previous skim.
 */
export function groupReportsByMission(
  items: InboxReportItem[],
  lastSeen?: string,
): ReportGroup[] {
  const byMission = new Map<string, ReportGroup>();
  for (const item of items) {
    const id = item.mission.id;
    let group = byMission.get(id);
    if (!group) {
      group = { mission: item.mission, items: [], newestAt: item.report.createdAt, newCount: 0 };
      byMission.set(id, group);
    }
    group.items.push(item);
    if (sortableTime(item.report.createdAt) > sortableTime(group.newestAt)) {
      group.newestAt = item.report.createdAt;
    }
    if (isNewReport(item.report.createdAt, lastSeen)) group.newCount += 1;
  }
  return [...byMission.values()].sort(
    (a, b) => sortableTime(b.newestAt) - sortableTime(a.newestAt),
  );
}

/**
 * Whether a report arrived after the operator's last look. A first visit
 * (undefined lastSeen) treats nothing as new — a fresh inbox is not a wall of
 * "new" badges — while a stored timestamp marks anything strictly newer.
 */
export function isNewReport(createdAt: string, lastSeen?: string): boolean {
  if (!lastSeen) return false;
  return sortableTime(createdAt) > sortableTime(lastSeen);
}

/** How many reports in the feed are new since `lastSeen` — the header count. */
export function countNewReports(items: InboxReportItem[], lastSeen?: string): number {
  return items.reduce((n, i) => (isNewReport(i.report.createdAt, lastSeen) ? n + 1 : n), 0);
}

/** The newest report time across the feed, or undefined when it is empty. */
export function newestReportAt(items: InboxReportItem[]): string | undefined {
  let newest: string | undefined;
  for (const { report } of items) {
    if (!newest || sortableTime(report.createdAt) > sortableTime(newest)) newest = report.createdAt;
  }
  return newest;
}

// ── Operator inbox (reports that reached no live supervisor) ────────────────

/**
 * Folds the operator-inbox feed (runtime/operatorinbox) into the flat report
 * feed so a report BOTH surfaces know about renders once — as the richer
 * operator-inbox row — instead of twice. The two feeds are independent reads
 * of overlapping data (the client-side join here walks every mission's
 * reports; the operator inbox is the server's own durable record of "reached
 * no supervisor"), so a report commonly appears in both. Matched by mission
 * id + the report's own id — the identity both an operator-inbox Item and a
 * mission-reports join row carry.
 *
 * Where a report appears in both, this PREFERS the operator-inbox row: the
 * report/mission payload is unchanged (both sides describe the same report),
 * but the merged item carries the operator-inbox row's provenance — `reason`
 * and, for `parent_gone`, the parent session id — as `operatorInbox`, which is
 * what lets the page mark it. An operator-inbox item the mission-reports join
 * has not (yet) fetched — a race between the two polls — is still added, as
 * its own row, attributed via `missionById`; one whose mission is not known
 * locally is dropped rather than rendered with a synthetic mission, mirroring
 * {@link joinInboxReports}'s own "unattributable, so dropped" rule.
 *
 * `operatorItems` is `null`/`undefined` on a first load or an absent feature
 * (see useOperatorInbox) — both are a no-op: `items` passes through
 * unmarked, exactly as before this merge existed.
 */
export function mergeOperatorInboxReports(
  items: InboxReportItem[],
  operatorItems: OperatorInboxItem[] | null | undefined,
  missionById: Map<string, Mission>,
): InboxReportItem[] {
  if (!operatorItems || operatorItems.length === 0) return items;

  const byIdentity = new Map<string, OperatorInboxItem>();
  for (const opItem of operatorItems) {
    byIdentity.set(reportIdentity(opItem.missionId, opItem.report.id), opItem);
  }

  const seen = new Set<string>();
  const merged: InboxReportItem[] = items.map(item => {
    const key = reportIdentity(item.report.missionId, item.report.id);
    const opItem = byIdentity.get(key);
    if (!opItem) return item;
    seen.add(key);
    return {
      ...item,
      operatorInbox: { reason: opItem.reason, parentSessionId: opItem.parentSessionId },
    };
  });

  for (const opItem of operatorItems) {
    const key = reportIdentity(opItem.missionId, opItem.report.id);
    if (seen.has(key)) continue;
    const mission = missionById.get(opItem.missionId);
    if (!mission) continue;
    merged.push({
      report: opItem.report,
      mission,
      operatorInbox: { reason: opItem.reason, parentSessionId: opItem.parentSessionId },
    });
  }

  return merged;
}

function reportIdentity(missionId: string, reportId: string): string {
  return `${missionId}::${reportId}`;
}

// ── Pending asks ─────────────────────────────────────────────────────────────

/** A mission's pending asks, clustered like the reports. */
export type ApprovalGroup = {
  /** The mission that raised these asks, when the ask carries one. */
  mission?: Mission;
  /** The mission id, present even when the mission record was not found. */
  missionId?: string;
  approvals: HITLApproval[];
};

/**
 * Clusters pending asks by mission so a mission's asks stay together, and orders
 * the clusters by their most-recent ask (freshest blocked mission on top).
 * Asks with no mission — a native chain turn, a direct API caller — collect in a
 * single trailing group with no mission, rather than one noisy group each.
 */
export function groupApprovalsByMission(
  approvals: HITLApproval[],
  missionById: Map<string, Mission>,
): ApprovalGroup[] {
  const attributed = new Map<string, ApprovalGroup>();
  const unattributed: HITLApproval[] = [];
  for (const approval of approvals) {
    const missionId = approval.missionId?.trim();
    if (!missionId) {
      unattributed.push(approval);
      continue;
    }
    let group = attributed.get(missionId);
    if (!group) {
      group = { mission: missionById.get(missionId), missionId, approvals: [] };
      attributed.set(missionId, group);
    }
    group.approvals.push(approval);
  }
  const groups = [...attributed.values()].sort(
    (a, b) => newestApprovalTime(b.approvals) - newestApprovalTime(a.approvals),
  );
  if (unattributed.length > 0) groups.push({ approvals: unattributed });
  return groups;
}

function newestApprovalTime(approvals: HITLApproval[]): number {
  return approvals.reduce((max, a) => Math.max(max, sortableTime(a.createdAt)), -Infinity);
}

// ── Stalled / failed units and the running summary ──────────────────────────

/** A live unit that has failed or gone into warning — the second triage tier. */
export type StalledUnit = {
  instance: InstanceStatus;
  agentName: string;
  kind: string;
  mission?: Mission;
};

// A unit in one of these states needs an operator: it crashed (error) or gave
// up (warning). Mirrors FleetPage's own ATTENTION_STATES.
const STALLED_STATES: FleetInstanceState[] = ['error', 'warning'];
const stalledRank = (s: FleetInstanceState): number => (s === 'error' ? 0 : 1);

/** Live-unit-count summary — what the "still running" line reports. */
export type RunningSummary = { running: number; starting: number; live: number };

const LIVE_STATES: FleetInstanceState[] = ['running', 'starting'];

/**
 * Failed/stalled units across the fleet, error before warning then most-recently
 * started first, each joined to the mission it was dispatched for (when one
 * claims its instance). This is the fleet's own attention read, surfaced on the
 * inbox so an operator answering asks also sees units that fell over overnight.
 */
export function stalledUnits(
  fleet: FleetEntry[],
  missionByInstanceId: Map<string, Mission>,
): StalledUnit[] {
  const units: StalledUnit[] = [];
  for (const entry of fleet) {
    for (const instance of entry.instances ?? []) {
      if (!STALLED_STATES.includes(instance.state)) continue;
      units.push({
        instance,
        agentName: entry.agentName,
        kind: entry.kind,
        mission: missionByInstanceId.get(instance.id),
      });
    }
  }
  units.sort((a, b) => {
    const byState = stalledRank(a.instance.state) - stalledRank(b.instance.state);
    if (byState !== 0) return byState;
    return sortableTime(b.instance.startedAt) - sortableTime(a.instance.startedAt);
  });
  return units;
}

/** Counts of live units — the compact "still running" summary. */
export function runningSummary(fleet: FleetEntry[]): RunningSummary {
  let running = 0;
  let starting = 0;
  for (const entry of fleet) {
    for (const instance of entry.instances ?? []) {
      if (instance.state === 'running') running += 1;
      else if (instance.state === 'starting') starting += 1;
    }
  }
  return { running, starting, live: running + starting };
}

/** Maps every mission that names an instance to that instance id. */
export function missionsByInstanceId(missions: Mission[]): Map<string, Mission> {
  const map = new Map<string, Mission>();
  for (const mission of missions) {
    if (mission.instanceId) map.set(mission.instanceId, mission);
  }
  return map;
}

/** Maps every mission by its own id, for ask attribution. */
export function missionsById(missions: Mission[]): Map<string, Mission> {
  return new Map(missions.map(m => [m.id, m]));
}

// ── Approval row presentation ────────────────────────────────────────────────

/**
 * The tool an ask is gating, as "toolsName / toolName" — or just one of them
 * when the other is absent. The primary fact of an approval row.
 */
export function approvalToolLabel(approval: Pick<HITLApproval, 'toolsName' | 'toolName'>): string {
  const parts = [approval.toolsName, approval.toolName].map(p => p?.trim()).filter(Boolean);
  return parts.join(' / ');
}

/**
 * Present-only attribution for an ask: an absent field is omitted, never shown
 * blank (the attribution set is best-effort — an ask from a native chain turn
 * carries none of it).
 */
export function approvalAttribution(approval: HITLApproval): {
  agentName?: string;
  missionId?: string;
  instanceId?: string;
  sessionId?: string;
} {
  return {
    agentName: nonEmpty(approval.agentName),
    missionId: nonEmpty(approval.missionId),
    instanceId: nonEmpty(approval.instanceId),
    sessionId: nonEmpty(approval.sessionId),
  };
}

/**
 * The rule an ask names, as "policy #rule" — or just the policy when no rule
 * index was recorded, or undefined when neither is present. Naming which policy
 * gated an action is a first-class inbox requirement (fleet-consolidation.md C2).
 */
export function approvalPolicyLabel(
  approval: Pick<HITLApproval, 'policyName' | 'matchedRule'>,
): string | undefined {
  const policy = nonEmpty(approval.policyName);
  if (!policy) return undefined;
  return approval.matchedRule != null ? `${policy} #${approval.matchedRule}` : policy;
}

function nonEmpty(v?: string): string | undefined {
  const t = v?.trim();
  return t ? t : undefined;
}

// An unparseable/absent timestamp sorts to the bottom (oldest) rather than
// poisoning a sort with NaN comparisons.
function sortableTime(iso: string): number {
  const parsed = Date.parse(iso);
  return Number.isNaN(parsed) ? -Infinity : parsed;
}

// ── Shared vocab ─────────────────────────────────────────────────────────────

// The report-kind badge variants and labels are shared with the mission detail
// page — one source of truth for how a blocker reads versus a progress note.
export {
  REPORT_KIND_BADGE_VARIANT,
  REPORT_KIND_LABEL_KEY,
} from '../missions/missionPresentation';

/** Which answer a button sends. Mirrors PermissionCard's option styling. */
export const APPROVAL_ANSWER_VARIANT: Record<'allow' | 'deny', 'primary' | 'danger'> = {
  allow: 'primary',
  deny: 'danger',
};

export const APPROVAL_ANSWER_LABEL_KEY: Record<'allow' | 'deny', TranslationKey> = {
  allow: 'inbox.answer_allow',
  deny: 'inbox.answer_deny',
};

// Stalled-unit state → the fleet's own status label key, reused verbatim.
export const STALLED_STATE_LABEL_KEY: Record<'error' | 'warning', TranslationKey> = {
  error: 'fleet.state.error',
  warning: 'fleet.state.warning',
};

export { LIVE_STATES, STALLED_STATES };

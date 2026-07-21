import {
  Badge,
  Button,
  Card,
  Collapsible,
  ErrorState,
  H1,
  H3,
  InlineNotice,
  KeyValue,
  LoadingState,
  P,
  Page,
  Section,
  Span,
} from '@contenox/ui';
import { useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { PlanProgress } from '../../../components/PlanProgress';
import { PlanRevisionLine } from '../../../components/MissionRevisionFeed';
import { UnitStatus } from '../../../components/UnitStatus';
import { useApprovals, useAnswerApproval, useInboxReports } from '../../../hooks/useApprovals';
import { useFleet } from '../../../hooks/useFleet';
import { useMissions } from '../../../hooks/useMissions';
import { relativeTime } from '../../../lib/relativeTime';
import type { HITLApproval, Mission } from '../../../lib/types';
import { blockedMissionIds } from '../../../lib/unitStatus';
import { cn } from '../../../lib/utils';
import {
  APPROVAL_ANSWER_LABEL_KEY,
  APPROVAL_ANSWER_VARIANT,
  REPORT_KIND_BADGE_VARIANT,
  REPORT_KIND_LABEL_KEY,
  type ApprovalGroup,
  type ReportGroup,
  type StalledUnit,
  approvalAttribution,
  approvalPolicyLabel,
  approvalToolLabel,
  countNewReports,
  groupApprovalsByMission,
  groupReportsByMission,
  isNewReport,
  missionsById,
  missionsByInstanceId,
  newestReportAt,
  runningSummary,
  stalledUnits,
} from './inboxPresentation';

// The newest report time the operator has already seen, persisted so a return
// visit shows what arrived since — the "what is new in the last hour" the hourly
// skim runs on. localStorage is guarded so a non-browser render (tests, SSR)
// simply treats every visit as a first look.
const LAST_SEEN_KEY = 'contenox:inbox:lastSeenReportAt';

function readLastSeen(): string | undefined {
  try {
    if (typeof localStorage === 'undefined') return undefined;
    return localStorage.getItem(LAST_SEEN_KEY) ?? undefined;
  } catch {
    return undefined;
  }
}

function writeLastSeen(iso?: string): void {
  try {
    if (typeof localStorage === 'undefined' || !iso) return;
    localStorage.setItem(LAST_SEEN_KEY, iso);
  } catch {
    /* private mode / disabled storage — the badge just resets to "all seen". */
  }
}

/**
 * The attention inbox (docs/development/blueprints/acp/fleet-consolidation.md,
 * slices C2 + M3) — the operating console for an overnight batch. The operator
 * fires many units in the evening and skims this page about once an hour,
 * operating entirely from here until everything lands, then reads the reports.
 * The page is ordered for that skim, top to bottom:
 *
 *   1. Pending asks — loudest. A blocked unit wastes the night; answering asks
 *      inline is the core operate-from-here action.
 *   2. Stalled / failed units — what fell over while unattended.
 *   3. What came back — reports, newest-first, "new since last look" marked.
 *   4. A compact still-running line linking to the fleet board (not duplicated).
 *
 * Counts sit in the header so the skim can start before the scroll, and asks and
 * reports are grouped by mission so concurrent missions do not interleave.
 * Loading/error gate on the approvals feed (the primary signal); the report and
 * fleet feeds degrade within their own sections.
 */
export default function InboxPage() {
  const { t, i18n } = useTranslation();
  const approvalsQuery = useApprovals();
  const missionsQuery = useMissions();
  const fleetQuery = useFleet();
  const reports = useInboxReports();
  const answer = useAnswerApproval();

  // Captured once at mount; the report feed keeps polling, so anything newer
  // than this is "new" for the whole visit. Persisted on unmount.
  const [lastSeen] = useState<string | undefined>(readLastSeen);
  const newestSeenRef = useRef<string | undefined>(undefined);
  newestSeenRef.current = newestReportAt(reports.items);
  useEffect(() => () => writeLastSeen(newestSeenRef.current), []);

  // One mutation serves every ask, so pending/error is scoped back to a row by
  // comparing the in-flight `variables.id` — the FleetPage idiom.
  const answerPending = (id: string) => answer.isPending && answer.variables?.id === id;
  const answerFailed = (id: string) => answer.error !== null && answer.variables?.id === id;

  const absoluteTime = (iso: string) => {
    const parsed = Date.parse(iso);
    return Number.isNaN(parsed) ? iso : new Date(parsed).toLocaleString(i18n.language);
  };
  const relative = (iso: string) => relativeTime(iso, i18n.language, t('inbox.just_now'));

  if (approvalsQuery.isLoading) {
    return (
      <Page bodyScroll="auto">
        <LoadingState message={t('inbox.loading')} />
      </Page>
    );
  }

  if (approvalsQuery.error) {
    return (
      <Page bodyScroll="auto">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 p-4 md:p-6">
          <ErrorState
            error={approvalsQuery.error}
            onRetry={() => void approvalsQuery.refetch()}
            title={t('inbox.approvals_load_error')}
          />
        </div>
      </Page>
    );
  }

  const approvals = approvalsQuery.data ?? [];
  const missions = missionsQuery.data ?? [];
  const fleet = fleetQuery.data ?? [];

  const approvalGroups = groupApprovalsByMission(approvals, missionsById(missions));
  const stalled = stalledUnits(fleet, missionsByInstanceId(missions));
  const running = runningSummary(fleet);
  const reportGroups = groupReportsByMission(reports.items, lastSeen);
  const newReportCount = countNewReports(reports.items, lastSeen);
  // Missions whose latest report is a blocker — the same signal the board and
  // mission pages surface, folded into each stalled unit's composed status.
  const blocked = blockedMissionIds(reports.items.map(i => i.report));

  const attentionClear = approvals.length === 0 && stalled.length === 0;

  return (
    <Page bodyScroll="auto">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 p-4 md:p-6">
        <div>
          <H1 variant="page">{t('inbox.title')}</H1>
          <P variant="muted" className="mt-2">
            {t('inbox.description')}
          </P>
          {/* Counts, so the skim starts before the scroll. */}
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Badge variant={approvals.length > 0 ? 'error' : 'secondary'} size="sm">
              {t('inbox.count_pending', { n: approvals.length })}
            </Badge>
            <Badge variant={stalled.length > 0 ? 'warning' : 'secondary'} size="sm">
              {t('inbox.count_stalled', { n: stalled.length })}
            </Badge>
            <Badge variant={newReportCount > 0 ? 'accent' : 'secondary'} size="sm">
              {t('inbox.count_new_reports', { n: newReportCount })}
            </Badge>
            <Badge variant="secondary" size="sm">
              {t('inbox.count_running', { n: running.live })}
            </Badge>
          </div>
        </div>

        {/* 1 — Pending asks, loudest, grouped by mission. */}
        {approvals.length > 0 && (
          <Section
            title={t('inbox.approvals_title')}
            description={t('inbox.approvals_description')}>
            <div className="mt-4 space-y-6">
              {approvalGroups.map(group => (
                <ApprovalMissionGroup
                  key={group.missionId ?? '__unattributed__'}
                  group={group}
                  answerPending={answerPending}
                  answerFailed={answerFailed}
                  errorMessage={answer.error?.message}
                  absoluteTime={absoluteTime}
                  relative={relative}
                  onAnswer={(id, approved) => answer.mutate({ id, approved })}
                />
              ))}
            </div>
          </Section>
        )}

        {/* 2 — Stalled / failed units. */}
        {stalled.length > 0 && (
          <Section title={t('inbox.stalled_title')} description={t('inbox.stalled_description')}>
            <div className="mt-4 space-y-2">
              {stalled.map(unit => (
                <StalledUnitRow
                  key={unit.instance.id}
                  unit={unit}
                  blocked={unit.mission ? blocked.has(unit.mission.id) : false}
                  relative={relative}
                />
              ))}
            </div>
          </Section>
        )}

        {/* All-clear: operationally true, not a generic "nothing here". */}
        {attentionClear && (
          <InlineNotice variant="info">
            <span className="font-medium">{t('inbox.all_clear_title')}</span>{' '}
            {t('inbox.all_clear_body', { running: running.live })}
          </InlineNotice>
        )}

        {/* 3 — What came back, grouped by mission, newest-first. */}
        <Section title={t('inbox.reports_title')} description={t('inbox.reports_description')}>
          <div className="mt-4 space-y-6">
            {reports.isLoading ? (
              <LoadingState message={t('inbox.reports_loading')} />
            ) : reports.error ? (
              <ErrorState error={reports.error} title={t('inbox.reports_load_error')} />
            ) : reportGroups.length === 0 ? (
              <P variant="muted">{t('inbox.reports_empty')}</P>
            ) : (
              reportGroups.map(group => (
                <ReportMissionGroup
                  key={group.mission.id}
                  group={group}
                  lastSeen={lastSeen}
                  absoluteTime={absoluteTime}
                  relative={relative}
                />
              ))
            )}
          </div>
        </Section>

        {/* 4 — Compact still-running summary, linking to the board it does not
            duplicate. */}
        {running.live > 0 && (
          <P variant="muted" className="text-sm">
            {t('inbox.running_summary', { n: running.live })}{' '}
            <Link to="/fleet" className="text-primary dark:text-dark-primary hover:underline">
              {t('inbox.running_view_board')}
            </Link>
          </P>
        )}
      </div>
    </Page>
  );
}

/** A mission's pending asks under one header — its intent, linked, or the
 *  trailing "not tied to a mission" cluster for asks that carry no mission. */
function ApprovalMissionGroup({
  group,
  answerPending,
  answerFailed,
  errorMessage,
  absoluteTime,
  relative,
  onAnswer,
}: {
  group: ApprovalGroup;
  answerPending: (id: string) => boolean;
  answerFailed: (id: string) => boolean;
  errorMessage?: string;
  absoluteTime: (iso: string) => string;
  relative: (iso: string) => string;
  onAnswer: (id: string, approved: boolean) => void;
}) {
  return (
    <div>
      <div className="mb-2 flex flex-wrap items-center gap-x-3 gap-y-1">
        <MissionGroupHeader mission={group.mission} missionId={group.missionId} />
        <PlanProgress plan={group.mission?.plan} />
        <PlanRevisionLine revisions={group.mission?.planRevisions} />
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {group.approvals.map(approval => (
          <ApprovalCard
            key={approval.id}
            approval={approval}
            pending={answerPending(approval.id)}
            failed={answerFailed(approval.id)}
            errorMessage={errorMessage}
            absoluteTime={absoluteTime}
            relative={relative}
            onAnswer={approved => onAnswer(approval.id, approved)}
          />
        ))}
      </div>
    </div>
  );
}

/** The header above a mission's grouped asks: its intent linked to the mission,
 *  the raw id when the record was not loaded, or the unattributed label. */
function MissionGroupHeader({
  mission,
  missionId,
}: {
  mission?: Mission;
  missionId?: string;
}) {
  const { t } = useTranslation();
  if (mission) {
    return (
      <Link
        to={`/missions/${mission.id}`}
        className="text-primary dark:text-dark-primary font-medium hover:underline"
        title={mission.intent}>
        {mission.intent}
      </Link>
    );
  }
  if (missionId) {
    return (
      <Link
        to={`/missions/${missionId}`}
        className="text-primary dark:text-dark-primary font-mono text-sm hover:underline">
        {missionId}
      </Link>
    );
  }
  return (
    <Span variant="muted" className="text-sm font-medium">
      {t('inbox.unattributed_group')}
    </Span>
  );
}

/**
 * One pending ask. Answering is a direct click — no confirm gate — because
 * allow/deny IS the intended primary action here (unlike FleetPage's Stop,
 * which destroys state and is gated). Mirrors the chat PermissionCard: the only
 * way to answer is an explicit button, and an unanswered ask stays pending. The
 * card always names the tool and, when the ask carries them, the policy that
 * gated it and who is asking.
 */
function ApprovalCard({
  approval,
  pending,
  failed,
  errorMessage,
  absoluteTime,
  relative,
  onAnswer,
}: {
  approval: HITLApproval;
  pending: boolean;
  failed: boolean;
  errorMessage?: string;
  absoluteTime: (iso: string) => string;
  relative: (iso: string) => string;
  onAnswer: (approved: boolean) => void;
}) {
  const { t } = useTranslation();
  const policy = approvalPolicyLabel(approval);
  const attribution = approvalAttribution(approval);
  const tool = approvalToolLabel(approval);

  return (
    <Card variant="surface" statusBorder="warning">
      <div className="flex items-center justify-between gap-3">
        <Span variant="status" className="text-warning-800 dark:text-dark-text-muted">
          {t('inbox.approval_card_title')}
        </Span>
        {policy && (
          <Badge variant="outline" size="sm" title={t('inbox.approval_policy_label')}>
            {policy}
          </Badge>
        )}
      </div>

      <H3 className="mt-2 font-mono text-sm break-all">{tool}</H3>

      <div className="mt-2 space-y-1">
        {approval.argsSummary && (
          <KeyValue
            label={t('inbox.approval_args_label')}
            value={<span className="font-mono text-xs break-all">{approval.argsSummary}</span>}
          />
        )}
        {attribution.agentName && (
          <KeyValue label={t('inbox.approval_agent_label')} value={attribution.agentName} />
        )}
        {attribution.instanceId && (
          <KeyValue
            label={t('inbox.approval_instance_label')}
            value={<span className="font-mono text-xs break-all">{attribution.instanceId}</span>}
          />
        )}
        <KeyValue
          label={t('inbox.approval_requested_label')}
          value={<span title={absoluteTime(approval.createdAt)}>{relative(approval.createdAt)}</span>}
        />
      </div>

      {approval.diff && (
        <Collapsible title={t('inbox.approval_diff_label')} className="mt-3">
          <pre className="bg-surface-100 dark:bg-dark-surface-300 text-text dark:text-dark-text mt-2 max-h-60 overflow-auto rounded p-2 text-xs whitespace-pre-wrap">
            {approval.diff}
          </pre>
        </Collapsible>
      )}

      {failed && (
        <InlineNotice variant="error" role="alert" className="mt-3">
          {`${t('inbox.answer_error')} ${errorMessage ?? ''}`}
        </InlineNotice>
      )}

      <div className="mt-4 flex flex-col gap-2 sm:flex-row sm:justify-end">
        <Button
          type="button"
          variant={APPROVAL_ANSWER_VARIANT.deny}
          size="sm"
          className="w-full sm:w-auto"
          aria-label={t('inbox.answer_deny_aria', { tool })}
          isLoading={pending}
          disabled={pending}
          onClick={() => onAnswer(false)}>
          {t(APPROVAL_ANSWER_LABEL_KEY.deny)}
        </Button>
        <Button
          type="button"
          variant={APPROVAL_ANSWER_VARIANT.allow}
          size="sm"
          className="w-full sm:w-auto"
          aria-label={t('inbox.answer_allow_aria', { tool })}
          isLoading={pending}
          disabled={pending}
          onClick={() => onAnswer(true)}>
          {t(APPROVAL_ANSWER_LABEL_KEY.allow)}
        </Button>
      </div>
    </Card>
  );
}

/** A failed/stalled unit as a compact row: its state, agent, the mission it was
 *  on, and a link to the board where it can be stopped. */
function StalledUnitRow({
  unit,
  blocked,
  relative,
}: {
  unit: StalledUnit;
  blocked: boolean;
  relative: (iso: string) => string;
}) {
  const { t } = useTranslation();
  return (
    <div className="border-surface-200 dark:border-dark-surface-600 space-y-2 rounded-lg border p-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0">
          <div className="font-medium">{unit.agentName}</div>
          {unit.mission ? (
            <Link
              to={`/missions/${unit.mission.id}`}
              className="text-text-muted dark:text-dark-text-muted block max-w-full truncate text-xs hover:underline"
              title={unit.mission.intent}>
              {unit.mission.intent}
            </Link>
          ) : (
            <span className="text-text-muted dark:text-dark-text-muted font-mono text-xs break-all">
              {unit.instance.id}
            </span>
          )}
        </div>
        <div className="flex flex-col items-start gap-1 sm:items-end">
          <UnitStatus
            facts={{
              instanceState: unit.instance.state,
              missionStatus: unit.mission?.status,
              lastHeartbeat: unit.mission?.lastHeartbeat,
              blocked,
            }}
            className="sm:justify-end"
          />
          <PlanProgress plan={unit.mission?.plan} />
        </div>
      </div>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <span className="text-text-muted dark:text-dark-text-muted text-xs">
          {relative(unit.instance.startedAt)}
        </span>
        <Link to="/fleet" className="text-primary dark:text-dark-primary text-xs hover:underline">
          {t('inbox.stalled_view_on_board')}
        </Link>
      </div>
    </div>
  );
}

/** A mission's reports clustered under its intent, with a "new" count when the
 *  cluster has reports the operator has not seen. */
function ReportMissionGroup({
  group,
  lastSeen,
  absoluteTime,
  relative,
}: {
  group: ReportGroup;
  lastSeen?: string;
  absoluteTime: (iso: string) => string;
  relative: (iso: string) => string;
}) {
  const { t } = useTranslation();
  return (
    <div>
      <div className="mb-2 flex flex-wrap items-center gap-2">
        <Link
          to={`/missions/${group.mission.id}`}
          className="text-primary dark:text-dark-primary font-medium hover:underline"
          title={group.mission.intent}>
          {group.mission.intent}
        </Link>
        {group.newCount > 0 && (
          <Badge variant="accent" size="sm">
            {t('inbox.report_group_new', { n: group.newCount })}
          </Badge>
        )}
        <PlanProgress plan={group.mission.plan} />
        <PlanRevisionLine revisions={group.mission.planRevisions} />
      </div>
      <div className="space-y-3">
        {group.items.map(({ report, operatorInbox }) => {
          const fresh = isNewReport(report.createdAt, lastSeen);
          // The louder case of the operator inbox (runtime/operatorinbox): the
          // report's parent session had already ended by the time it arrived,
          // so it landed here instead of with a live supervisor. Marked, not
          // alarmed — a warning-tinted card and a named badge, matching the
          // "slightly louder than ordinary reports" ask.
          const supervisorGone = operatorInbox?.reason === 'parent_gone';
          return (
            <div
              key={report.id}
              className={cn(
                'rounded-lg border p-4',
                supervisorGone
                  ? 'border-warning-300 bg-warning-50 dark:border-dark-surface-500 dark:bg-dark-surface-200'
                  : 'border-surface-200 dark:border-dark-surface-600',
              )}>
              <div className="flex flex-wrap items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <Badge variant={REPORT_KIND_BADGE_VARIANT[report.kind]} size="sm">
                    {t(REPORT_KIND_LABEL_KEY[report.kind])}
                  </Badge>
                  {supervisorGone && (
                    <Badge
                      variant="warning"
                      size="sm"
                      title={t('inbox.operator_inbox_parent_gone_title')}>
                      {t('inbox.operator_inbox_parent_gone_badge')}
                    </Badge>
                  )}
                  {fresh && (
                    <Badge variant="accent" size="sm">
                      {t('inbox.report_new_badge')}
                    </Badge>
                  )}
                  <span className="font-medium">{report.summary}</span>
                </div>
                <span
                  className="text-text-muted dark:text-dark-text-muted text-xs"
                  title={absoluteTime(report.createdAt)}>
                  {relative(report.createdAt)}
                </span>
              </div>

              {report.detail && (
                <Collapsible title={t('inbox.report_detail_toggle')} className="mt-3">
                  <P className="mt-2 whitespace-pre-wrap">{report.detail}</P>
                </Collapsible>
              )}

              {report.refs && report.refs.length > 0 && (
                <div className="mt-3">
                  <Span variant="muted" className="text-xs">
                    {t('inbox.report_refs_label')}:
                  </Span>
                  <ul className="mt-1 space-y-0.5">
                    {report.refs.map((ref, i) => (
                      <li key={i} className="font-mono text-xs break-all">
                        {ref}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}

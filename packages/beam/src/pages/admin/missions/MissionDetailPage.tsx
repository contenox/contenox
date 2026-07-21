import {
  Badge,
  Button,
  Collapsible,
  ErrorState,
  H1,
  InlineNotice,
  KeyValue,
  LoadingState,
  P,
  Page,
  Section,
  Span,
} from '@contenox/ui';
import { lazy, Suspense, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate, useParams } from 'react-router-dom';
import { MissionPlanPanel } from '../../../components/MissionPlanPanel';
import { MissionRevisionFeed } from '../../../components/MissionRevisionFeed';
import { ScopeAnomalyChip, ScopeBadge } from '../../../components/ScopeStatus';
import { UnitStatus } from '../../../components/UnitStatus';
import { startAdoptSession, useAdoptIntent } from '../../../lib/adoptIntent';
import { useFleet } from '../../../hooks/useFleet';
import { useMissionChanges } from '../../../hooks/useMissionChanges';
import { useMission, useMissionReports } from '../../../hooks/useMissions';
import { relativeTime } from '../../../lib/relativeTime';
import type { FleetInstanceState } from '../../../lib/types';
import { blockedMissionIds } from '../../../lib/unitStatus';
import { MissionInspectorTabs } from './MissionInspectorTabs';
import { MISSION_INSPECTOR_TABS, useInspectorTab } from './inspectorTabs';
import {
  heartbeatLabel,
  REPORT_KIND_BADGE_VARIANT,
  REPORT_KIND_LABEL_KEY,
} from './missionPresentation';

// Lazily code-split so xterm (Arc 3) never enters the mission-detail bundle
// unless the Terminal tab is actually opened.
const MissionTerminalTab = lazy(() => import('./MissionTerminalTab'));
// Lazily code-split so Monaco (Arc 1's diff view) never enters the
// mission-detail bundle unless the Changes tab is actually opened.
const MissionChangesTab = lazy(() => import('./MissionChangesTab'));
const MissionSearchTab = lazy(() => import('./MissionSearchTab'));

/**
 * Mission mode's detail surface (docs/development/blueprints/acp/
 * fleet-consolidation.md, "Mission mode", slice M2): the mission's facts plus
 * its reports newest-first. The status line under the intent is the one
 * composed truth (`UnitStatus`) — process state (joined from the fleet),
 * verdict, liveness, and blocker — the same picture the board and the list
 * show, so nothing here contradicts them. Reports are the record of unattended
 * work; a `blocker` report is styled to be unmistakable next to a `progress`
 * one, and Detail / Refs are rendered plainly rather than as something
 * clickable into a document the board cannot actually resolve.
 */
export default function MissionDetailPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { setAdoptIntent } = useAdoptIntent();
  const { id } = useParams<{ id: string }>();
  const missionQuery = useMission(id ?? '');
  const reportsQuery = useMissionReports(id ?? '');
  const fleetQuery = useFleet();
  // The attention layer's changes/scope — fetched at the page level so the
  // scope + anomaly chips surface beside the status regardless of the active
  // tab (async enhancement that never blocks the header), and so the Search
  // tab's click-through knows the changed-files set. Shared by cache key with
  // the Changes tab, so opening it reuses this fetch.
  const changes = useMissionChanges(id ?? '');
  const [activeTab, setActiveTab] = useInspectorTab();
  // A search hit routed into the Changes tab focuses its file.
  const [focusChangePath, setFocusChangePath] = useState<string | null>(null);

  const absoluteTime = (iso?: string) => {
    if (!iso) return undefined;
    const parsed = Date.parse(iso);
    return Number.isNaN(parsed) ? iso : new Date(parsed).toLocaleString(i18n.language);
  };

  if (missionQuery.isLoading) {
    return (
      <Page bodyScroll="auto">
        <LoadingState message={t('missions.loading')} />
      </Page>
    );
  }

  if (missionQuery.error || !missionQuery.data) {
    return (
      <Page bodyScroll="auto">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 p-4 md:p-6">
          <ErrorState
            error={missionQuery.error ?? undefined}
            onRetry={() => void missionQuery.refetch()}
            title={t('missions.not_found_title')}
            description={t('missions.not_found_description')}
          />
        </div>
      </Page>
    );
  }

  const m = missionQuery.data;
  const reports = reportsQuery.data ?? [];

  // Process state for THIS mission's instance, when it is still live on the
  // board; blocker state from THIS mission's own reports (its latest report is
  // a blocker). Both fold into the one composed status.
  let instanceState: FleetInstanceState | undefined;
  for (const entry of fleetQuery.data ?? []) {
    for (const inst of entry.instances ?? []) {
      if (inst.id === m.instanceId) instanceState = inst.state;
    }
  }
  const blocked = blockedMissionIds(reports).has(m.id);
  // Present only when serve exposes change tracking and the fetch has resolved;
  // absent/loading simply shows no scope chips (never a placeholder).
  const scope = changes.data?.scope;
  const changedFiles = changes.data?.files ?? [];
  const openInChanges = (absolutePath: string) => {
    setFocusChangePath(absolutePath);
    setActiveTab('changes');
  };
  // "Open session" adopts the running dispatch into the chat surface. It needs a
  // live instance and a bound downstream session; the runtime rejects adopting a
  // stopped one, so the affordance is gated on the joined process state.
  const canAdopt = instanceState === 'running' && !!m.sessionId && !!m.instanceId;
  const openSession = () => {
    if (!m.instanceId || !m.sessionId) return;
    startAdoptSession({ instanceId: m.instanceId, sessionId: m.sessionId }, { setAdoptIntent, navigate });
  };

  return (
    <Page bodyScroll="auto">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 p-4 md:p-6">
        <div>
          <Link
            to="/missions"
            className="text-text-muted dark:text-dark-text-muted text-sm hover:underline">
            {t('missions.back_to_list')}
          </Link>
          {/* Intent as the primary fact, per the blueprint's mission-first
              framing — it is the H1, not a KeyValue row among many. */}
          <H1 variant="page" className="mt-2">
            {m.intent}
          </H1>
          {/* The composed status, with the attention layer's scope surfacing
              beside it — loudest-first, chip-adjacent, matching how PlanProgress
              is placed. The anomaly chip is ADVICE, not a verdict. */}
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <UnitStatus
              facts={{
                instanceState,
                missionStatus: m.status,
                lastHeartbeat: m.lastHeartbeat,
                blocked,
              }}
            />
            {scope && <ScopeAnomalyChip scope={scope} />}
            {scope && <ScopeBadge scope={scope} />}
          </div>
          {/* The scope-anomaly detail: an early-warning ADVICE notice naming the
              paths the unit touched outside its expected scope. */}
          {scope?.anomaly && (
            <InlineNotice variant="warning" className="mt-3">
              <p className="font-medium">{t('scope.anomaly_title')}</p>
              <p className="mt-1 text-xs">{t('scope.anomaly_body')}</p>
              {scope.outsidePaths && scope.outsidePaths.length > 0 && (
                <div className="mt-2">
                  <p className="text-xs font-medium">{t('scope.outside_label')}</p>
                  <ul className="mt-1 space-y-0.5">
                    {scope.outsidePaths.map(p => (
                      <li key={p} className="font-mono text-xs break-all">
                        {p}
                      </li>
                    ))}
                  </ul>
                </div>
              )}
            </InlineNotice>
          )}
          {canAdopt && (
            <div className="mt-4">
              <Button variant="primary" size="sm" onClick={openSession}>
                {t('missions.open_session')}
              </Button>
            </div>
          )}
        </div>

        <MissionInspectorTabs
          tabs={MISSION_INSPECTOR_TABS}
          activeTab={activeTab}
          onTabChange={setActiveTab}
        />

        {activeTab === 'changes' && (
          <Suspense fallback={<LoadingState message={t('changes.loading')} />}>
            <MissionChangesTab missionId={m.id} focusPath={focusChangePath} />
          </Suspense>
        )}

        {activeTab === 'search' && (
          <Suspense fallback={<LoadingState message={t('workspaceSearch.searching')} />}>
            <MissionSearchTab changedFiles={changedFiles} onOpenInChanges={openInChanges} />
          </Suspense>
        )}

        {activeTab === 'terminal' && (
          <Suspense fallback={<LoadingState message={t('hostTerminal.connecting')} />}>
            <MissionTerminalTab missionId={m.id} />
          </Suspense>
        )}

        {activeTab === 'overview' && (
          <div className="flex flex-col gap-8">
        {m.lastError && (
          <InlineNotice variant="error" role="alert">
            <span className="font-medium">{t('missions.last_error_label')}: </span>
            {m.lastError}
          </InlineNotice>
        )}

        <Section title={t('missions.facts_title')}>
          <div className="mt-2 space-y-1">
            <KeyValue label={t('missions.facts_agent')} value={m.agentName} />
            <KeyValue
              label={t('missions.facts_envelope')}
              value={
                <Badge variant="outline" size="sm">
                  {m.hitlPolicyName}
                </Badge>
              }
            />
            <KeyValue
              label={t('missions.facts_heartbeat')}
              value={
                <span title={absoluteTime(m.lastHeartbeat)}>
                  {heartbeatLabel(m.lastHeartbeat, i18n.language, {
                    never: t('missions.heartbeat_never'),
                    justNow: t('common.just_now'),
                  })}
                </span>
              }
            />
            <KeyValue
              label={t('missions.facts_session')}
              value={<span className="font-mono text-xs break-all">{m.sessionId || '—'}</span>}
            />
            <KeyValue
              label={t('missions.facts_instance')}
              value={<span className="font-mono text-xs break-all">{m.instanceId || '—'}</span>}
            />
            <KeyValue label={t('missions.facts_created')} value={absoluteTime(m.createdAt)} />
            <KeyValue label={t('missions.facts_updated')} value={absoluteTime(m.updatedAt)} />
          </div>
        </Section>

        {/* The living plan — rendered only when the mission has one (an unplanned
            mission carries the zero Plan, which shows no panel). */}
        <MissionPlanPanel plan={m.plan} />

        {/* The plan-revision history — the "+2/−1 — why" feed, newest first.
            Renders nothing on a legacy/never-planned mission. */}
        <MissionRevisionFeed revisions={m.planRevisions} />

        <Section title={t('missions.reports_title')} description={t('missions.reports_description')}>
          <div className="mt-4 space-y-3">
            {reportsQuery.isLoading ? (
              <LoadingState message={t('missions.reports_loading')} />
            ) : reportsQuery.error ? (
              <ErrorState
                error={reportsQuery.error}
                onRetry={() => void reportsQuery.refetch()}
                title={t('missions.reports_load_error')}
              />
            ) : reports.length === 0 ? (
              <P variant="muted">{t('missions.reports_empty')}</P>
            ) : (
              reports.map(r => (
                <div
                  key={r.id}
                  className="border-surface-200 dark:border-dark-surface-600 rounded-lg border p-4">
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <Badge variant={REPORT_KIND_BADGE_VARIANT[r.kind]} size="sm">
                        {t(REPORT_KIND_LABEL_KEY[r.kind])}
                      </Badge>
                      <span className="font-medium">{r.summary}</span>
                    </div>
                    <span
                      className="text-text-muted dark:text-dark-text-muted text-xs"
                      title={absoluteTime(r.createdAt)}>
                      {relativeTime(r.createdAt, i18n.language, t('common.just_now'))}
                    </span>
                  </div>

                  {r.detail && (
                    <Collapsible title={t('missions.report_detail_toggle')} className="mt-3">
                      <P className="mt-2 whitespace-pre-wrap">{r.detail}</P>
                    </Collapsible>
                  )}

                  {r.refs && r.refs.length > 0 && (
                    <div className="mt-3">
                      <Span variant="muted" className="text-xs">
                        {t('missions.report_refs_label')}:
                      </Span>
                      <ul className="mt-1 space-y-0.5">
                        {r.refs.map((ref, i) => (
                          <li key={i} className="font-mono text-xs break-all">
                            {ref}
                          </li>
                        ))}
                      </ul>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </Section>
          </div>
        )}
      </div>
    </Page>
  );
}

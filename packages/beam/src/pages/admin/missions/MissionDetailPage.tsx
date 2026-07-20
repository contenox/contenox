import {
  Badge,
  Collapsible,
  ErrorState,
  GridLayout,
  H1,
  InlineNotice,
  KeyValue,
  LoadingState,
  P,
  Page,
  Section,
  Span,
  StatusIndicator,
} from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { Link, useParams } from 'react-router-dom';
import { useMission, useMissionReports } from '../../../hooks/useMissions';
import { relativeTime } from '../../../lib/relativeTime';
import {
  MISSION_STATUS_INDICATOR,
  MISSION_STATUS_LABEL_KEY,
  REPORT_KIND_BADGE_VARIANT,
  REPORT_KIND_LABEL_KEY,
  heartbeatLabel,
} from './missionPresentation';

/**
 * Mission mode's detail surface (docs/development/blueprints/acp/
 * fleet-consolidation.md, "Mission mode", slice M2): the mission's facts plus
 * its reports newest-first. Reports are the record of unattended work — this
 * is the page an operator reads to find out what happened, so a `blocker`
 * report is styled to be unmistakable next to a `progress` one, and Detail /
 * Refs are rendered plainly rather than as something clickable into a
 * document the board cannot actually resolve (see FleetPage's identical
 * "shown, not linked" treatment of session ids, for the same reason: a
 * ref may be a local path, not a fetchable URL).
 */
export default function MissionDetailPage() {
  const { t, i18n } = useTranslation();
  const { id } = useParams<{ id: string }>();
  const missionQuery = useMission(id ?? '');
  const reportsQuery = useMissionReports(id ?? '');

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
        <GridLayout variant="body" minWidth="minmax(0, 1fr)" className="gap-8 pb-8">
          <ErrorState
            error={missionQuery.error ?? undefined}
            onRetry={() => void missionQuery.refetch()}
            title={t('missions.not_found_title')}
            description={t('missions.not_found_description')}
          />
        </GridLayout>
      </Page>
    );
  }

  const m = missionQuery.data;
  const reports = reportsQuery.data ?? [];

  return (
    <Page bodyScroll="auto">
      <GridLayout variant="body" minWidth="minmax(0, 1fr)" className="gap-8 pb-8">
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
          <div className="mt-3">
            <StatusIndicator
              status={MISSION_STATUS_INDICATOR[m.status]}
              label={t(MISSION_STATUS_LABEL_KEY[m.status])}
              showIcon
              size="sm"
            />
          </div>
        </div>

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
              value={<span className="font-mono text-xs">{m.sessionId || '—'}</span>}
            />
            <KeyValue
              label={t('missions.facts_instance')}
              value={<span className="font-mono text-xs">{m.instanceId || '—'}</span>}
            />
            <KeyValue label={t('missions.facts_created')} value={absoluteTime(m.createdAt)} />
            <KeyValue label={t('missions.facts_updated')} value={absoluteTime(m.updatedAt)} />
          </div>
        </Section>

        <Section
          title={t('missions.reports_title')}
          description={t('missions.reports_description')}>
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
      </GridLayout>
    </Page>
  );
}

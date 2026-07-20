import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  GridLayout,
  H1,
  LoadingState,
  P,
  Page,
  Section,
  StatusIndicator,
  Table,
  TableCell,
  TableRow,
} from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { useMissions } from '../../../hooks/useMissions';
import {
  MISSION_STATUS_INDICATOR,
  MISSION_STATUS_LABEL_KEY,
  heartbeatLabel,
} from './missionPresentation';

/**
 * Mission mode's list surface (docs/development/blueprints/acp/
 * fleet-consolidation.md, "Mission mode", slice M2): every fired mission,
 * intent first — what a unit was sent to do matters more than its process
 * state, which is why intent is the one column rendered as the primary,
 * linked fact and status/envelope/heartbeat sit alongside it rather than
 * ahead of it.
 */
export default function MissionListPage() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { data, isLoading, error, refetch } = useMissions();

  if (isLoading) {
    return (
      <Page bodyScroll="auto">
        <LoadingState message={t('missions.loading')} />
      </Page>
    );
  }

  if (error) {
    return (
      <Page bodyScroll="auto">
        <GridLayout variant="body" minWidth="minmax(0, 1fr)" className="gap-8 pb-8">
          <ErrorState
            error={error}
            onRetry={() => void refetch()}
            title={t('missions.load_error')}
          />
        </GridLayout>
      </Page>
    );
  }

  const missions = data ?? [];

  const absoluteTime = (iso: string) => {
    const parsed = Date.parse(iso);
    return Number.isNaN(parsed) ? iso : new Date(parsed).toLocaleString(i18n.language);
  };

  return (
    <Page bodyScroll="auto">
      <GridLayout variant="body" minWidth="minmax(0, 1fr)" className="gap-8 pb-8">
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <H1 variant="page">{t('missions.title')}</H1>
            <P variant="muted" className="mt-2">
              {t('missions.description')}
            </P>
          </div>
          <Button variant="primary" size="sm" onClick={() => navigate('/missions/new')}>
            {t('missions.fire_action')}
          </Button>
        </div>

        {missions.length === 0 ? (
          <EmptyState
            title={t('missions.empty_title')}
            description={t('missions.empty_description')}
          />
        ) : (
          <Section>
            <Table
              columns={[
                t('missions.col_intent'),
                t('missions.col_agent'),
                t('missions.col_status'),
                t('missions.col_envelope'),
                t('missions.col_heartbeat'),
              ]}>
              {missions.map(m => (
                <TableRow key={m.id}>
                  <TableCell>
                    <Link
                      to={`/missions/${m.id}`}
                      className="text-primary dark:text-dark-primary font-medium hover:underline">
                      {m.intent}
                    </Link>
                    {m.lastError && (
                      <div
                        className="text-error dark:text-dark-error mt-1 max-w-sm truncate text-xs"
                        title={m.lastError}>
                        {t('missions.last_error_label')}: {m.lastError}
                      </div>
                    )}
                  </TableCell>
                  <TableCell>{m.agentName}</TableCell>
                  <TableCell>
                    <StatusIndicator
                      status={MISSION_STATUS_INDICATOR[m.status]}
                      label={t(MISSION_STATUS_LABEL_KEY[m.status])}
                      showIcon
                      size="sm"
                    />
                  </TableCell>
                  <TableCell>
                    <Badge variant="outline" size="sm">
                      {m.hitlPolicyName}
                    </Badge>
                  </TableCell>
                  <TableCell
                    title={m.lastHeartbeat ? absoluteTime(m.lastHeartbeat) : undefined}
                    className={
                      m.lastHeartbeat
                        ? undefined
                        : 'text-text-muted dark:text-dark-text-muted italic'
                    }>
                    {heartbeatLabel(m.lastHeartbeat, i18n.language, {
                      never: t('missions.heartbeat_never'),
                      justNow: t('common.just_now'),
                    })}
                  </TableCell>
                </TableRow>
              ))}
            </Table>
          </Section>
        )}
      </GridLayout>
    </Page>
  );
}

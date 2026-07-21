import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  H1,
  LoadingState,
  P,
  Page,
  Section,
} from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { PlanProgress } from '../../../components/PlanProgress';
import { UnitStatus } from '../../../components/UnitStatus';
import { useInboxReports } from '../../../hooks/useApprovals';
import { useFleet } from '../../../hooks/useFleet';
import { useMissions } from '../../../hooks/useMissions';
import type { InstanceStatus } from '../../../lib/types';
import { blockedMissionIds } from '../../../lib/unitStatus';

/**
 * Mission mode's list surface (docs/development/blueprints/acp/
 * fleet-consolidation.md, "Mission mode", slice M2): every fired mission,
 * intent first — what a unit was sent to do matters more than its process
 * state. Each mission renders as a generous single-column row rather than a
 * table crushed into a column, and its status is the ONE composed truth
 * (`UnitStatus`) — process state (joined from the fleet), verdict, liveness,
 * and blocker — so the same unit no longer reads one way here and another way
 * on the board.
 */
export default function MissionListPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { data, isLoading, error, refetch } = useMissions();
  // Joined only to show the unit's live process state next to its mission
  // verdict — the reconciliation the composed status exists for. A missing
  // fleet feed degrades to "no process chip", never breaks the list.
  const fleetQuery = useFleet();
  const inboxReports = useInboxReports();

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
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-8 p-4 md:p-6">
          <ErrorState
            error={error}
            onRetry={() => void refetch()}
            title={t('missions.load_error')}
          />
        </div>
      </Page>
    );
  }

  const missions = data ?? [];

  const instanceById = new Map<string, InstanceStatus>();
  for (const entry of fleetQuery.data ?? []) {
    for (const inst of entry.instances ?? []) instanceById.set(inst.id, inst);
  }
  const blocked = blockedMissionIds(inboxReports.items.map(i => i.report));

  return (
    <Page bodyScroll="auto">
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-8 p-4 md:p-6">
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
            <div className="space-y-3">
              {missions.map(m => {
                const inst = m.instanceId ? instanceById.get(m.instanceId) : undefined;
                return (
                  <div
                    key={m.id}
                    className="border-surface-200 dark:border-dark-surface-600 space-y-2 rounded-lg border p-4">
                    <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
                      <div className="min-w-0 space-y-1">
                        <Link
                          to={`/missions/${m.id}`}
                          className="text-primary dark:text-dark-primary block font-medium hover:underline"
                          title={m.intent}>
                          {m.intent}
                        </Link>
                        {m.lastError && (
                          <div
                            className="text-error dark:text-dark-error max-w-full truncate text-xs"
                            title={m.lastError}>
                            {t('missions.last_error_label')}: {m.lastError}
                          </div>
                        )}
                      </div>
                      <div className="flex flex-col items-start gap-1 sm:items-end">
                        <UnitStatus
                          facts={{
                            instanceState: inst?.state,
                            missionStatus: m.status,
                            lastHeartbeat: m.lastHeartbeat,
                            blocked: blocked.has(m.id),
                          }}
                          className="sm:justify-end"
                        />
                        <PlanProgress plan={m.plan} />
                      </div>
                    </div>
                    <div className="text-text-muted dark:text-dark-text-muted flex flex-wrap items-center gap-x-6 gap-y-1 text-sm">
                      <span>
                        {t('missions.col_agent')}: {m.agentName}
                      </span>
                      <span className="flex items-center gap-1.5">
                        {t('missions.col_envelope')}:
                        <Badge variant="outline" size="sm">
                          {m.hitlPolicyName}
                        </Badge>
                      </span>
                    </div>
                  </div>
                );
              })}
            </div>
          </Section>
        )}
      </div>
    </Page>
  );
}

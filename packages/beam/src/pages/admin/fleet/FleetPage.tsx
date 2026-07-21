import {
  Badge,
  Button,
  Dialog,
  EmptyState,
  ErrorState,
  H1,
  InlineNotice,
  KeyValue,
  LoadingState,
  P,
  Page,
  Section,
  StatusIndicator,
} from '@contenox/ui';
import type { UseMutationResult } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useNavigate } from 'react-router-dom';
import { PlanProgress } from '../../../components/PlanProgress';
import { UnitStatus } from '../../../components/UnitStatus';
import { startAdoptSession, useAdoptIntent } from '../../../lib/adoptIntent';
import { useInboxReports } from '../../../hooks/useApprovals';
import {
  useCancelInstance,
  useFleet,
  useStopInstance,
  type CancelInstanceInput,
} from '../../../hooks/useFleet';
import { useMissions } from '../../../hooks/useMissions';
import type { TranslationKey } from '../../../i18n';
import { relativeTime } from '../../../lib/relativeTime';
import type { FleetInstanceState, InstanceStatus, Mission } from '../../../lib/types';
import { blockedMissionIds, PROCESS_LABEL_KEY } from '../../../lib/unitStatus';

// Instances in these states are hoisted into the attention strip; everything
// else (running / starting / stopped) is a live-or-recently-live unit.
const ATTENTION_STATES: FleetInstanceState[] = ['error', 'warning'];
const attentionRank = (s: FleetInstanceState): number => (s === 'error' ? 0 : 1);

// States in which the instance is (or is becoming) a live agent process. Stop
// on one of these kills working software; stop on anything else is a reap.
const LIVE_STATES: FleetInstanceState[] = ['running', 'starting'];

/**
 * Which confirm copy the Stop dialog uses. There is exactly one Stop verb —
 * `Manager.Stop` also removes the instance from the registry, so the same call
 * is both "kill a working agent" and "clear a dead row from the board" — but
 * those read as two very different asks to an operator, and the board must be
 * truthful about which one it is about to do.
 */
export function stopConfirmCopy(state: FleetInstanceState): {
  title: TranslationKey;
  body: TranslationKey;
  confirm: TranslationKey;
} {
  return LIVE_STATES.includes(state)
    ? {
        title: 'fleet.stop_confirm_title',
        body: 'fleet.stop_confirm_body',
        confirm: 'fleet.stop_confirm_confirm',
      }
    : {
        title: 'fleet.stop_confirm_title_dead',
        body: 'fleet.stop_confirm_body_dead',
        confirm: 'fleet.stop_confirm_confirm_dead',
      };
}

/**
 * The Stop gate, as a pure trio of handlers.
 *
 * `request` NEVER stops anything — it only nominates an instance for
 * confirmation; the mutation is reachable exclusively through `confirm`. That
 * invariant is the whole point of the gate, so it lives here as plain functions
 * rather than inline arrows: `packages/beam` has no `@testing-library/react`
 * (see PermissionCard.test.tsx), and this way the gate is exercisable directly
 * instead of only through a DOM click that cannot be simulated.
 */
export function createStopFlow(
  stop: (instanceId: string) => void,
  setTarget: (instance: InstanceStatus | null) => void,
) {
  return {
    request: (instance: InstanceStatus) => setTarget(instance),
    confirm: (instance: InstanceStatus) => {
      stop(instance.id);
      setTarget(null);
    },
    dismiss: () => setTarget(null),
  };
}

/**
 * The Stop confirmation. Composed from `Dialog` + `Button` because the kit has
 * no prebuilt confirm, and kept out of `FleetPage` so the copy selection is a
 * plain prop-driven render rather than a branch buried in the board.
 */
export function StopConfirmDialog({
  instance,
  pending,
  onConfirm,
  onClose,
}: {
  instance: InstanceStatus;
  pending: boolean;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const copy = stopConfirmCopy(instance.state);

  return (
    <Dialog
      open
      onClose={onClose}
      title={t(copy.title)}
      closeLabel={t('fleet.stop_confirm_close')}
      className="w-[min(32rem,90vw)]">
      <P>{t(copy.body)}</P>
      <div className="mt-4 space-y-1">
        <KeyValue label={t('fleet.stop_confirm_agent')} value={instance.agentName} />
        <KeyValue
          label={t('fleet.stop_confirm_instance')}
          value={<span className="font-mono text-xs break-all">{instance.id}</span>}
        />
        <KeyValue
          label={t('fleet.stop_confirm_state')}
          value={t(PROCESS_LABEL_KEY[instance.state])}
        />
      </div>
      <div className="mt-6 flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
        <Button
          variant="outline"
          palette="neutral"
          size="sm"
          className="w-full sm:w-auto"
          onClick={onClose}>
          {t('fleet.stop_confirm_decline')}
        </Button>
        <Button
          variant="danger"
          size="sm"
          className="w-full sm:w-auto"
          isLoading={pending}
          disabled={pending}
          onClick={onConfirm}>
          {t(copy.confirm)}
        </Button>
      </div>
    </Dialog>
  );
}

type StopMutation = UseMutationResult<string, Error, string, unknown>;
type CancelMutation = UseMutationResult<string, Error, CancelInstanceInput, unknown>;

/**
 * One running (or recently-live) unit, given the room to breathe that the
 * board's whole point is to give it: the composed status truth up top, then its
 * instance id, the mission it is running, sessions, viewers, and age laid out as
 * plain facts, then one addressable row per attached session, then the lifecycle
 * actions. A generous single-column card rather than a crushed table row, so it
 * reads the same on a phone as on a wide screen.
 */
function InstanceRow({
  entry,
  inst,
  mission,
  blocked,
  tone,
  stopMutation,
  cancelMutation,
  onRequestStop,
}: {
  entry: { agentName: string; kind: string };
  inst: InstanceStatus;
  mission?: Mission;
  blocked: boolean;
  tone?: 'attention';
  stopMutation: StopMutation;
  cancelMutation: CancelMutation;
  onRequestStop: (inst: InstanceStatus) => void;
}) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const { setAdoptIntent } = useAdoptIntent();
  const sessionIds = inst.sessionIds ?? [];
  // Adoption needs a live downstream to attach a viewer to, so "Open session" is
  // offered only while the instance is running (the runtime rejects adopting a
  // stopped/errored one). See lib/adoptIntent.ts / adoptMeta.ts.
  const canAdopt = inst.state === 'running';
  const openSession = (sessionId: string) =>
    startAdoptSession({ instanceId: inst.id, sessionId }, { setAdoptIntent, navigate });

  const absoluteTime = (iso: string) => {
    const parsed = Date.parse(iso);
    return Number.isNaN(parsed) ? iso : new Date(parsed).toLocaleString(i18n.language);
  };

  const stopPending = stopMutation.isPending && stopMutation.variables === inst.id;
  const stopFailed = stopMutation.error !== null && stopMutation.variables === inst.id;
  const cancelPending = (sessionId?: string) =>
    cancelMutation.isPending &&
    cancelMutation.variables?.instanceId === inst.id &&
    cancelMutation.variables.sessionId === sessionId;
  const cancelFailed =
    cancelMutation.error !== null && cancelMutation.variables?.instanceId === inst.id;

  const border =
    tone === 'attention'
      ? inst.state === 'error'
        ? 'border-l-4 border-l-error dark:border-l-dark-error'
        : 'border-l-4 border-l-warning dark:border-l-dark-warning'
      : '';

  return (
    <div
      className={`border-surface-200 dark:border-dark-surface-600 space-y-3 rounded-lg border p-4 ${border}`}>
      <div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{entry.agentName}</span>
            <Badge variant="secondary" size="sm">
              {entry.kind}
            </Badge>
          </div>
          {mission && (
            <Link
              to={`/missions/${mission.id}`}
              className="text-text-muted dark:text-dark-text-muted block max-w-full truncate text-sm hover:underline"
              title={mission.intent}>
              {mission.intent}
            </Link>
          )}
        </div>
        <div className="flex flex-col items-start gap-1 sm:items-end">
          <UnitStatus
            facts={{
              instanceState: inst.state,
              missionStatus: mission?.status,
              lastHeartbeat: mission?.lastHeartbeat,
              blocked,
            }}
            className="sm:justify-end"
          />
          <PlanProgress plan={mission?.plan} />
        </div>
      </div>

      <div className="text-text-muted dark:text-dark-text-muted flex flex-wrap gap-x-6 gap-y-1 text-sm">
        <span className="min-w-0 break-all">
          {t('fleet.col_instance')}: <span className="font-mono text-xs">{inst.id}</span>
        </span>
        <span className="tabular-nums">
          {t('fleet.col_sessions')}: {inst.sessions}
        </span>
        <span className="tabular-nums">
          {t('fleet.col_viewers')}: {inst.viewers}
        </span>
        <span title={absoluteTime(inst.startedAt)}>
          {t('fleet.col_started')}:{' '}
          {relativeTime(inst.startedAt, i18n.language, t('common.just_now'))}
        </span>
      </div>

      {/* One addressable row per attached session. These are DOWNSTREAM ACP
          session ids; beam's /chat/:id routes are keyed by UPSTREAM contenox
          session ids, and acpsvc holds that mapping in one direction only
          (acpsvc/session.go). So the id is shown, not linked — a link here
          would be a dead link into the wrong id space. */}
      {sessionIds.length === 0 ? (
        <P variant="muted" className="text-xs">
          {t('fleet.no_sessions')}
        </P>
      ) : (
        <ul className="space-y-2">
          {sessionIds.map(sessionId => (
            <li
              key={sessionId}
              className="border-surface-100 dark:border-dark-surface-700 flex flex-col gap-2 rounded border p-2 sm:flex-row sm:items-center sm:justify-between">
              <span className="min-w-0 break-all">
                <span className="text-text-muted mr-2 text-xs">{t('fleet.session_label')}</span>
                <span className="font-mono text-xs">{sessionId}</span>
              </span>
              <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
                {/* Open the running session in the normal chat surface by ADOPTING
                    it (session/new + contenox.adopt) — watch its replay and, if
                    no one else controls it, answer its permission asks. */}
                {canAdopt && (
                  <Button
                    variant="outline"
                    palette="neutral"
                    size="xs"
                    className="w-full sm:w-auto"
                    aria-label={t('fleet.open_session_aria', { id: sessionId })}
                    onClick={() => openSession(sessionId)}>
                    {t('fleet.open_session')}
                  </Button>
                )}
                <Button
                  variant="outline"
                  palette="neutral"
                  size="xs"
                  className="w-full sm:w-auto"
                  aria-label={t('fleet.cancel_session_aria', { id: sessionId })}
                  isLoading={cancelPending(sessionId)}
                  disabled={cancelPending(sessionId)}
                  onClick={() => cancelMutation.mutate({ instanceId: inst.id, sessionId })}>
                  {t('fleet.cancel_session')}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      <div className="flex flex-wrap items-center justify-end gap-2">
        {/* Cancel-all only earns its place once there is more than one session;
            with a single session the per-session row above already is the whole
            fan-out. */}
        {sessionIds.length > 1 && (
          <Button
            variant="outline"
            palette="neutral"
            size="xs"
            aria-label={t('fleet.cancel_all_aria')}
            isLoading={cancelPending(undefined)}
            disabled={cancelPending(undefined)}
            onClick={() => cancelMutation.mutate({ instanceId: inst.id })}>
            {t('fleet.cancel_all')}
          </Button>
        )}
        {/* Stop stays offered in every state: the call is idempotent, and on a
            dead instance it IS the reap action — there is no other way to clear
            the row. */}
        <Button
          variant="danger"
          size="xs"
          aria-label={t('fleet.stop_aria', { id: inst.id })}
          isLoading={stopPending}
          disabled={stopPending}
          onClick={() => onRequestStop(inst)}>
          {t('fleet.stop')}
        </Button>
      </div>

      {(stopFailed || cancelFailed) && (
        <InlineNotice variant="error" role="alert">
          {stopFailed
            ? `${t('fleet.stop_error')} ${stopMutation.error?.message ?? ''}`
            : `${t('fleet.cancel_error')} ${cancelMutation.error?.message ?? ''}`}
        </InlineNotice>
      )}
    </div>
  );
}

export default function FleetPage() {
  const { t } = useTranslation();
  const { data, isLoading, error, refetch } = useFleet();
  // Missions carry InstanceID (missionservice.Mission), so the board joins to
  // them client-side rather than the server threading mission data through
  // FleetEntry. A failure here degrades silently — the row still renders,
  // just without its mission facts.
  const { data: missionsData } = useMissions();
  // Report feed, purely to know which missions are blocked (latest report is a
  // blocker) so the board can surface a "waiting on a human" chip first-class.
  // Cache-shared with the inbox and mission detail (same per-mission report
  // query keys), so mounting it here is close to free when navigating between
  // the fleet surfaces.
  const inboxReports = useInboxReports();
  const stopInstance = useStopInstance();
  const cancelInstance = useCancelInstance();
  const [stopTarget, setStopTarget] = useState<InstanceStatus | null>(null);
  const stopFlow = createStopFlow(stopInstance.mutate, setStopTarget);

  if (isLoading) {
    return (
      <Page bodyScroll="auto">
        <LoadingState message={t('fleet.loading')} />
      </Page>
    );
  }

  if (error) {
    return (
      <Page bodyScroll="auto">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-8 p-4 md:p-6">
          <ErrorState error={error} onRetry={() => void refetch()} title={t('fleet.load_error')} />
        </div>
      </Page>
    );
  }

  const fleet = data ?? [];

  const missionByInstanceId = new Map<string, Mission>(
    (missionsData ?? [])
      .filter((m): m is Mission & { instanceId: string } => !!m.instanceId)
      .map(m => [m.instanceId, m]),
  );
  const blocked = blockedMissionIds(inboxReports.items.map(i => i.report));

  // Every instance across every agent, split into the two piles the board is
  // organised around: what needs attention (error/warning) and what is
  // running-or-recently-live (everything else). Idle agents — declared but with
  // no live instance — collapse into one compact secondary list, not a card each.
  const attention: { entry: (typeof fleet)[number]; inst: InstanceStatus }[] = [];
  const live: { entry: (typeof fleet)[number]; inst: InstanceStatus }[] = [];
  for (const entry of fleet) {
    for (const inst of entry.instances ?? []) {
      if (ATTENTION_STATES.includes(inst.state)) attention.push({ entry, inst });
      else live.push({ entry, inst });
    }
  }
  attention.sort((a, b) => {
    const byState = attentionRank(a.inst.state) - attentionRank(b.inst.state);
    if (byState !== 0) return byState;
    return Date.parse(b.inst.startedAt) - Date.parse(a.inst.startedAt);
  });
  live.sort((a, b) => Date.parse(b.inst.startedAt) - Date.parse(a.inst.startedAt));

  const idleEntries = fleet.filter(entry => (entry.instances ?? []).length === 0);
  const runningCount = live.filter(x => LIVE_STATES.includes(x.inst.state)).length;

  const isBlocked = (mission?: Mission) => (mission ? blocked.has(mission.id) : false);

  const renderRow = (
    entry: (typeof fleet)[number],
    inst: InstanceStatus,
    rowTone?: 'attention',
  ) => {
    const mission = missionByInstanceId.get(inst.id);
    return (
      <InstanceRow
        key={inst.id}
        entry={entry}
        inst={inst}
        mission={mission}
        blocked={isBlocked(mission)}
        tone={rowTone}
        stopMutation={stopInstance}
        cancelMutation={cancelInstance}
        onRequestStop={stopFlow.request}
      />
    );
  };

  return (
    <Page bodyScroll="auto">
      <div className="mx-auto flex w-full max-w-5xl flex-col gap-8 p-4 md:p-6">
        <div>
          <H1 variant="page">{t('fleet.title')}</H1>
          <P variant="muted" className="mt-2">
            {t('fleet.description')}
          </P>
          {fleet.length > 0 && (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <Badge variant="secondary" size="sm">
                {t('fleet.count_running', { n: runningCount })}
              </Badge>
              <Badge variant={attention.length > 0 ? 'error' : 'secondary'} size="sm">
                {t('fleet.count_attention', { n: attention.length })}
              </Badge>
              <Badge variant="secondary" size="sm">
                {t('fleet.count_idle', { n: idleEntries.length })}
              </Badge>
            </div>
          )}
        </div>

        {fleet.length === 0 ? (
          <EmptyState title={t('fleet.empty_title')} description={t('fleet.empty_description')} />
        ) : (
          <>
            {attention.length > 0 && (
              <Section
                title={t('fleet.attention_title')}
                description={t('fleet.attention_description')}>
                <div className="mt-4 space-y-3">
                  {attention.map(({ entry, inst }) => renderRow(entry, inst, 'attention'))}
                </div>
              </Section>
            )}

            <Section title={t('fleet.running_title')}>
              <div className="mt-4 space-y-3">
                {live.length === 0 ? (
                  <P variant="muted">{t('fleet.running_none')}</P>
                ) : (
                  live.map(({ entry, inst }) => renderRow(entry, inst))
                )}
              </div>
            </Section>

            {idleEntries.length > 0 && (
              <Section
                title={t('fleet.idle_agents_title')}
                description={t('fleet.idle_agents_description')}>
                <ul className="border-surface-200 dark:border-dark-surface-600 divide-surface-200 dark:divide-dark-surface-600 mt-4 divide-y rounded-lg border">
                  {idleEntries.map(entry => (
                    <li key={entry.agentId} className="flex flex-wrap items-center gap-2 px-3 py-2">
                      <span className="font-medium">{entry.agentName}</span>
                      <Badge variant="secondary" size="sm">
                        {entry.kind}
                      </Badge>
                      <span className="ml-auto">
                        <StatusIndicator status="planned" label={t('fleet.idle')} size="sm" />
                      </span>
                    </li>
                  ))}
                </ul>
              </Section>
            )}
          </>
        )}

        {stopTarget && (
          <StopConfirmDialog
            instance={stopTarget}
            pending={stopInstance.isPending && stopInstance.variables === stopTarget.id}
            onClose={stopFlow.dismiss}
            onConfirm={() => stopFlow.confirm(stopTarget)}
          />
        )}
      </div>
    </Page>
  );
}

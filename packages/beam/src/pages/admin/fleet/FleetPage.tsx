import {
  Badge,
  Button,
  Card,
  Dialog,
  EmptyState,
  ErrorState,
  GridLayout,
  H1,
  H2,
  H3,
  InlineNotice,
  KeyValue,
  LoadingState,
  P,
  Page,
  Section,
  Span,
  StatusIndicator,
  Table,
  TableCell,
  TableRow,
  type Status,
} from '@contenox/ui';
import { Fragment, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link } from 'react-router-dom';
import { useCancelInstance, useFleet, useStopInstance } from '../../../hooks/useFleet';
import { useMissions } from '../../../hooks/useMissions';
import type { TranslationKey } from '../../../i18n';
import { relativeTime } from '../../../lib/relativeTime';
import type { FleetInstanceState, InstanceStatus, Mission } from '../../../lib/types';

// Instance state → StatusIndicator status. error/warning are fixed by the
// slice; the other three are chosen so the dot colour reads as fleet health:
//   running  → completed   (green ✓  — up and serving, the healthy steady state)
//   starting → in-progress (amber ⟳  — transitional, coming up)
//   stopped  → planned     (neutral ○ — declared/known but not currently running)
// error and warning keep the red/amber alert semantics one-to-one.
const STATE_STATUS: Record<FleetInstanceState, Status> = {
  running: 'completed',
  starting: 'in-progress',
  stopped: 'planned',
  warning: 'warning',
  error: 'error',
};

const STATE_LABEL_KEY: Record<FleetInstanceState, TranslationKey> = {
  starting: 'fleet.state.starting',
  running: 'fleet.state.running',
  stopped: 'fleet.state.stopped',
  warning: 'fleet.state.warning',
  error: 'fleet.state.error',
};

// Instances in these states are hoisted into the attention strip.
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
 *
 * `starting` counts as live: it is not dead, it is coming up, and stopping it
 * aborts a bring-up. Everything else (stopped/warning/error) gets the reap
 * copy, which still admits the instance *might* be serving a conversation the
 * board cannot see.
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
 *
 * The body states the real cost rather than hiding it in a tooltip: the fleet
 * blueprint's invariant is that the board is truthful about what Stop destroys.
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
          value={<span className="font-mono text-xs">{instance.id}</span>}
        />
        <KeyValue
          label={t('fleet.stop_confirm_state')}
          value={t(STATE_LABEL_KEY[instance.state])}
        />
      </div>
      <div className="mt-6 flex justify-end gap-2">
        <Button variant="outline" palette="neutral" size="sm" onClick={onClose}>
          {t('fleet.stop_confirm_decline')}
        </Button>
        <Button
          variant="danger"
          size="sm"
          isLoading={pending}
          disabled={pending}
          onClick={onConfirm}>
          {t(copy.confirm)}
        </Button>
      </div>
    </Dialog>
  );
}

export default function FleetPage() {
  const { t, i18n } = useTranslation();
  const { data, isLoading, error, refetch } = useFleet();
  // Missions carry InstanceID (missionservice.Mission), so the board joins to
  // them client-side rather than the server threading mission data through
  // FleetEntry. A failure here degrades silently — the row still renders,
  // just without its intent — because the fleet board's own load/error state
  // must not depend on a second, unrelated feed.
  const { data: missionsData } = useMissions();
  const stopInstance = useStopInstance();
  const cancelInstance = useCancelInstance();
  // The instance awaiting Stop confirmation, or null. Stop is the one verb here
  // that destroys state, so it never fires straight off the click.
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
        <GridLayout variant="body" minWidth="minmax(0, 1fr)" className="gap-8 pb-8">
          <ErrorState error={error} onRetry={() => void refetch()} title={t('fleet.load_error')} />
        </GridLayout>
      </Page>
    );
  }

  const fleet = data ?? [];

  // Client-side join, keyed by InstanceID — see the useMissions() call above
  // for why this is a join rather than a server-side field. A mission not yet
  // bound to an instance (still being created) or one whose instance already
  // moved on both simply find no row here, which is correct: this map only
  // ever answers "which mission is THIS instance currently running."
  const missionByInstanceId = new Map<string, Mission>(
    (missionsData ?? [])
      .filter((m): m is Mission & { instanceId: string } => !!m.instanceId)
      .map(m => [m.instanceId, m]),
  );

  // Attention-first: every error/warning instance (with its agent) hoisted to
  // the top, error before warning, then most-recently-started first.
  const attention = fleet
    .flatMap(entry =>
      (entry.instances ?? [])
        .filter(inst => ATTENTION_STATES.includes(inst.state))
        .map(inst => ({ entry, inst })),
    )
    .sort((a, b) => {
      const byState = attentionRank(a.inst.state) - attentionRank(b.inst.state);
      if (byState !== 0) return byState;
      return Date.parse(b.inst.startedAt) - Date.parse(a.inst.startedAt);
    });

  const absoluteTime = (iso: string) => {
    const parsed = Date.parse(iso);
    return Number.isNaN(parsed) ? iso : new Date(parsed).toLocaleString(i18n.language);
  };

  // One mutation hook serves the whole board, so pending/error state is scoped
  // back to a row by comparing against the in-flight `variables`. Without this,
  // clicking Cancel on one session would grey out every button on the page.
  const stopPending = (instanceId: string) =>
    stopInstance.isPending && stopInstance.variables === instanceId;
  const cancelPending = (instanceId: string, sessionId?: string) =>
    cancelInstance.isPending &&
    cancelInstance.variables?.instanceId === instanceId &&
    cancelInstance.variables.sessionId === sessionId;
  const stopFailed = (instanceId: string) =>
    stopInstance.error !== null && stopInstance.variables === instanceId;
  const cancelFailed = (instanceId: string) =>
    cancelInstance.error !== null && cancelInstance.variables?.instanceId === instanceId;

  return (
    <Page bodyScroll="auto">
      <GridLayout variant="body" minWidth="minmax(0, 1fr)" className="gap-8 pb-8">
        <div>
          <H1 variant="page">{t('fleet.title')}</H1>
          <P variant="muted" className="mt-2">
            {t('fleet.description')}
          </P>
        </div>

        {fleet.length === 0 ? (
          <EmptyState title={t('fleet.empty_title')} description={t('fleet.empty_description')} />
        ) : (
          <>
            {attention.length > 0 && (
              <Section
                title={t('fleet.attention_title')}
                description={t('fleet.attention_description')}>
                <div className="mt-4 grid gap-3 sm:grid-cols-2">
                  {attention.map(({ entry, inst }) => {
                    const mission = missionByInstanceId.get(inst.id);
                    return (
                      <Card
                        key={inst.id}
                        variant="surface"
                        statusBorder={inst.state === 'error' ? 'error' : 'warning'}>
                        <div className="flex items-center justify-between gap-3">
                          <StatusIndicator
                            status={STATE_STATUS[inst.state]}
                            label={t(STATE_LABEL_KEY[inst.state])}
                            showIcon
                            size="sm"
                          />
                          <Badge variant="secondary" size="sm">
                            {entry.kind}
                          </Badge>
                        </div>
                        <H3 className="mt-3">{entry.agentName}</H3>
                        <div className="mt-2 space-y-1">
                          {mission && (
                            <KeyValue
                              label={t('fleet.mission_intent_label')}
                              value={
                                <Link
                                  to={`/missions/${mission.id}`}
                                  className="hover:underline"
                                  title={mission.intent}>
                                  {mission.intent}
                                </Link>
                              }
                            />
                          )}
                          <KeyValue
                            label={t('fleet.col_instance')}
                            value={<span className="font-mono text-xs">{inst.id}</span>}
                          />
                          <KeyValue label={t('fleet.col_sessions')} value={inst.sessions} />
                          <KeyValue label={t('fleet.col_viewers')} value={inst.viewers} />
                          <KeyValue
                            label={t('fleet.col_started')}
                            value={
                              <span title={absoluteTime(inst.startedAt)}>
                                {relativeTime(inst.startedAt, i18n.language, t('common.just_now'))}
                              </span>
                            }
                          />
                        </div>
                      </Card>
                    );
                  })}
                </div>
              </Section>
            )}

            {fleet.map(entry => {
              const instances = entry.instances ?? [];
              const idle = instances.length === 0;
              const running = instances.filter(i => i.state === 'running').length;
              return (
                <Section key={entry.agentId}>
                  <div className="flex flex-wrap items-center gap-3">
                    <H2>{entry.agentName}</H2>
                    <Badge variant="secondary" size="sm">
                      {entry.kind}
                    </Badge>
                    {idle ? (
                      <Badge variant="outline" size="sm">
                        {t('fleet.idle')}
                      </Badge>
                    ) : (
                      <Span className="text-text-muted dark:text-dark-text-muted text-sm">
                        {t('fleet.instances_running', { n: running })}
                      </Span>
                    )}
                  </div>

                  {idle ? (
                    <P variant="muted" className="mt-3">
                      {t('fleet.idle_description')}
                    </P>
                  ) : (
                    <div className="mt-4">
                      <Table
                        columns={[
                          t('fleet.col_state'),
                          t('fleet.col_instance'),
                          t('fleet.col_sessions'),
                          t('fleet.col_viewers'),
                          t('fleet.col_started'),
                          t('fleet.col_actions'),
                        ]}>
                        {instances.map(inst => {
                          const sessionIds = inst.sessionIds ?? [];
                          const mission = missionByInstanceId.get(inst.id);
                          return (
                            <Fragment key={inst.id}>
                              <TableRow>
                                <TableCell>
                                  <StatusIndicator
                                    status={STATE_STATUS[inst.state]}
                                    label={t(STATE_LABEL_KEY[inst.state])}
                                    showIcon
                                    size="sm"
                                  />
                                </TableCell>
                                <TableCell className="font-mono text-xs">
                                  <div>{inst.id}</div>
                                  {/* Missions carry InstanceID; this is the client-side
                                      join (see missionByInstanceId above). Additive only —
                                      a row with no matching mission renders exactly as
                                      before. Linked to the mission's own detail page rather
                                      than duplicating its report feed here. */}
                                  {mission && (
                                    <Link
                                      to={`/missions/${mission.id}`}
                                      className="text-text-muted dark:text-dark-text-muted mt-1 block max-w-[16rem] truncate font-sans text-xs italic hover:underline"
                                      title={mission.intent}>
                                      {mission.intent}
                                    </Link>
                                  )}
                                </TableCell>
                                <TableCell className="tabular-nums">{inst.sessions}</TableCell>
                                <TableCell className="tabular-nums">{inst.viewers}</TableCell>
                                <TableCell title={absoluteTime(inst.startedAt)}>
                                  {relativeTime(inst.startedAt, i18n.language, t('common.just_now'))}
                                </TableCell>
                                <TableCell>
                                  <div className="flex flex-wrap items-center justify-end gap-2">
                                    {/* Cancel-all only earns its place once there is
                                        more than one session; with a single session the
                                        per-session row below already is the whole fan-out. */}
                                    {sessionIds.length > 1 && (
                                      <Button
                                        variant="outline"
                                        palette="neutral"
                                        size="xs"
                                        aria-label={t('fleet.cancel_all_aria')}
                                        isLoading={cancelPending(inst.id, undefined)}
                                        disabled={cancelPending(inst.id, undefined)}
                                        onClick={() =>
                                          cancelInstance.mutate({ instanceId: inst.id })
                                        }>
                                        {t('fleet.cancel_all')}
                                      </Button>
                                    )}
                                    {/* Stop stays offered in every state: the call is
                                        idempotent, and on a dead instance it IS the reap
                                        action — there is no other way to clear the row. */}
                                    <Button
                                      variant="danger"
                                      size="xs"
                                      aria-label={t('fleet.stop_aria', { id: inst.id })}
                                      isLoading={stopPending(inst.id)}
                                      disabled={stopPending(inst.id)}
                                      onClick={() => stopFlow.request(inst)}>
                                      {t('fleet.stop')}
                                    </Button>
                                  </div>
                                </TableCell>
                              </TableRow>

                              {/* One addressable row per attached session. These are
                                  DOWNSTREAM ACP session ids; beam's /chat/:id routes are
                                  keyed by UPSTREAM contenox session ids, and acpsvc holds
                                  that mapping in one direction only (acpsvc/session.go).
                                  So the id is shown, not linked — a link here would be a
                                  dead link into the wrong id space. */}
                              {sessionIds.length === 0 ? (
                                <TableRow>
                                  <TableCell />
                                  <TableCell colSpan={5} className="text-text-muted text-xs">
                                    {t('fleet.no_sessions')}
                                  </TableCell>
                                </TableRow>
                              ) : (
                                sessionIds.map(sessionId => (
                                  <TableRow key={`${inst.id}:${sessionId}`}>
                                    <TableCell />
                                    <TableCell colSpan={4}>
                                      <span className="text-text-muted mr-2 text-xs">
                                        {t('fleet.session_label')}
                                      </span>
                                      <span className="font-mono text-xs">{sessionId}</span>
                                    </TableCell>
                                    <TableCell>
                                      <div className="flex justify-end">
                                        <Button
                                          variant="outline"
                                          palette="neutral"
                                          size="xs"
                                          aria-label={t('fleet.cancel_session_aria', {
                                            id: sessionId,
                                          })}
                                          isLoading={cancelPending(inst.id, sessionId)}
                                          disabled={cancelPending(inst.id, sessionId)}
                                          onClick={() =>
                                            cancelInstance.mutate({
                                              instanceId: inst.id,
                                              sessionId,
                                            })
                                          }>
                                          {t('fleet.cancel_session')}
                                        </Button>
                                      </div>
                                    </TableCell>
                                  </TableRow>
                                ))
                              )}

                              {/* A failed lifecycle call never disappears quietly: it
                                  lands on the row it belongs to, with the server's own
                                  message. */}
                              {(stopFailed(inst.id) || cancelFailed(inst.id)) && (
                                <TableRow>
                                  <TableCell colSpan={6}>
                                    <InlineNotice variant="error" role="alert">
                                      {stopFailed(inst.id)
                                        ? `${t('fleet.stop_error')} ${stopInstance.error?.message ?? ''}`
                                        : `${t('fleet.cancel_error')} ${cancelInstance.error?.message ?? ''}`}
                                    </InlineNotice>
                                  </TableCell>
                                </TableRow>
                              )}
                            </Fragment>
                          );
                        })}
                      </Table>
                    </div>
                  )}
                </Section>
              );
            })}
          </>
        )}

        {stopTarget && (
          <StopConfirmDialog
            instance={stopTarget}
            pending={stopPending(stopTarget.id)}
            onClose={stopFlow.dismiss}
            onConfirm={() => stopFlow.confirm(stopTarget)}
          />
        )}
      </GridLayout>
    </Page>
  );
}

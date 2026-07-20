import {
  Badge,
  Card,
  EmptyState,
  ErrorState,
  GridLayout,
  H1,
  H2,
  H3,
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
import { useTranslation } from 'react-i18next';
import { useFleet } from '../../../hooks/useFleet';
import type { TranslationKey } from '../../../i18n';
import type { FleetInstanceState } from '../../../lib/types';

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

/**
 * Locale-aware "x ago" for an ISO timestamp, mirroring the sidebar's
 * relativeTimeLabel (components/sidebar/AcpSessionSidebar.tsx). Falls back to
 * the raw string if the timestamp is unparseable.
 */
function relativeTime(iso: string, locale: string, justNow: string): string {
  const then = Date.parse(iso);
  if (Number.isNaN(then)) return iso;
  const diffSec = Math.round((Date.now() - then) / 1000);
  if (diffSec < 45) return justNow;
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  const diffMin = Math.round(diffSec / 60);
  if (diffMin < 60) return rtf.format(-diffMin, 'minute');
  const diffHour = Math.round(diffMin / 60);
  if (diffHour < 24) return rtf.format(-diffHour, 'hour');
  const diffDay = Math.round(diffHour / 24);
  if (diffDay < 30) return rtf.format(-diffDay, 'day');
  const diffMonth = Math.round(diffDay / 30);
  if (diffMonth < 12) return rtf.format(-diffMonth, 'month');
  return rtf.format(-Math.round(diffMonth / 12), 'year');
}

export default function FleetPage() {
  const { t, i18n } = useTranslation();
  const { data, isLoading, error, refetch } = useFleet();

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
                  {attention.map(({ entry, inst }) => (
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
                              {relativeTime(inst.startedAt, i18n.language, t('fleet.just_now'))}
                            </span>
                          }
                        />
                      </div>
                    </Card>
                  ))}
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
                        ]}>
                        {instances.map(inst => (
                          <TableRow key={inst.id}>
                            <TableCell>
                              <StatusIndicator
                                status={STATE_STATUS[inst.state]}
                                label={t(STATE_LABEL_KEY[inst.state])}
                                showIcon
                                size="sm"
                              />
                            </TableCell>
                            <TableCell className="font-mono text-xs">{inst.id}</TableCell>
                            <TableCell className="tabular-nums">{inst.sessions}</TableCell>
                            <TableCell className="tabular-nums">{inst.viewers}</TableCell>
                            <TableCell title={absoluteTime(inst.startedAt)}>
                              {relativeTime(inst.startedAt, i18n.language, t('fleet.just_now'))}
                            </TableCell>
                          </TableRow>
                        ))}
                      </Table>
                    </div>
                  )}
                </Section>
              );
            })}
          </>
        )}
      </GridLayout>
    </Page>
  );
}

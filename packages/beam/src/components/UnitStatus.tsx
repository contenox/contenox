import { Badge, cn, StatusIndicator } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { relativeTime } from '../lib/relativeTime';
import { composeUnitStatus, type UnitStatusFacts } from '../lib/unitStatus';

/**
 * One composed status for a fleet unit — the single presentational truth used
 * by the fleet board, the mission list, the mission detail, and the inbox (see
 * `composeUnitStatus` for the reasoning). It renders the ordered atoms as a
 * wrapping row of chips: the process state as a StatusIndicator "signal light",
 * and the verdict / liveness / blocked facts as labelled badges, each carrying a
 * tooltip that says which truth it reports so they never read as a
 * contradiction. Renders nothing when there is nothing to say (no facts).
 */
export function UnitStatus({
  facts,
  className,
  size = 'sm',
}: {
  facts: UnitStatusFacts;
  className?: string;
  size?: 'sm' | 'md';
}) {
  const { t, i18n } = useTranslation();
  const atoms = composeUnitStatus(facts);
  if (atoms.length === 0) return null;

  return (
    <div className={cn('flex flex-wrap items-center gap-x-2 gap-y-1', className)}>
      {atoms.map((atom, i) => {
        const title = atom.titleKey ? t(atom.titleKey) : undefined;

        if (atom.kind === 'process' && atom.indicator) {
          return (
            <span key={i} className="shrink-0" title={title}>
              <StatusIndicator
                status={atom.indicator}
                label={atom.labelKey ? t(atom.labelKey) : undefined}
                showIcon
                size={size}
              />
            </span>
          );
        }

        const label =
          atom.kind === 'liveness' && atom.heartbeat
            ? `${t('unit.liveness_prefix')}: ${relativeTime(atom.heartbeat, i18n.language, t('common.just_now'))}`
            : atom.labelKey
              ? t(atom.labelKey)
              : '';

        return (
          <Badge
            key={i}
            variant={atom.variant ?? 'secondary'}
            size="sm"
            title={title}
            className={cn('shrink-0', atom.kind === 'blocked' && 'font-semibold')}>
            {atom.kind === 'blocked' ? `⚠ ${label}` : label}
          </Badge>
        );
      })}
    </div>
  );
}

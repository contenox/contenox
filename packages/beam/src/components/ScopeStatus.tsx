import { Badge } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { MissionChangeScope } from '../lib/types';

/**
 * The attention layer's scope surfacing (attention-layer.md Stage 2), placed
 * BESIDE the composed UnitStatus on the mission detail — the same loudest-first,
 * chip-adjacent idiom PlanProgress uses, never inside the status atoms. It is
 * ADVICE, not a verdict: the scope-anomaly chip flags that a unit's changes
 * reached outside its expected scope as an early-warning signal, but nothing
 * here gates anything and the operator's eyes stay the judge (the exoskeleton,
 * not autopilot). The quiet count badge is the plain "how much did it touch"
 * fact.
 */

/** Quiet "N files / M directories" badge — the plain scope fact, never a health colour. */
export function ScopeBadge({ scope }: { scope: MissionChangeScope }) {
  const { t } = useTranslation();
  return (
    <Badge variant="secondary" size="sm" title={t('scope.count_title')}>
      {t('scope.count', { files: scope.files, dirs: scope.dirs })}
    </Badge>
  );
}

/**
 * The loud scope-anomaly advice chip — rendered only when `scope.anomaly`. Its
 * tone matches the UnitStatus blocked atom (a leading ⚠, semibold) but its
 * tooltip states plainly that it is advice, not a verdict, reusing the
 * attention layer's "rank is advice" stance.
 */
export function ScopeAnomalyChip({ scope }: { scope: MissionChangeScope }) {
  const { t } = useTranslation();
  if (!scope.anomaly) return null;
  return (
    <Badge
      variant="warning"
      size="sm"
      className="shrink-0 font-semibold"
      title={t('scope.anomaly_chip_title')}>
      {`⚠ ${t('scope.anomaly_chip')}`}
    </Badge>
  );
}

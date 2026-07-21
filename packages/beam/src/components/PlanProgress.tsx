import { cn, StatusIndicator } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { planProgress } from '../lib/missionPlan';
import type { MissionPlan } from '../lib/types';

/**
 * The compact step-progress fragment for a fleet unit that has a plan —
 * "3/7 Schritte" with a quiet in-progress marker — rendered ADJACENT to the
 * `<UnitStatus>` atoms on every row that shows a mission (the board, the mission
 * list, the inbox groups). It is a distinct fact from the composed STATUS
 * (blocked / process / verdict / liveness), and reads as neutral progress, never
 * a health signal — so it lives beside `UnitStatus` rather than inside
 * `composeUnitStatus`, sharing the one `planProgress` helper with the mission
 * detail's full plan panel.
 *
 * Renders NOTHING when there is no plan (absent, or the zero Plan a never-planned
 * mission carries) — no "0/0" shell.
 */
export function PlanProgress({
  plan,
  className,
}: {
  plan?: MissionPlan | null;
  className?: string;
}) {
  const { t } = useTranslation();
  const progress = planProgress(plan);
  if (!progress) return null;

  const { completed, total, inProgress } = progress;

  return (
    <span
      className={cn(
        'text-text-muted dark:text-dark-text-muted inline-flex shrink-0 items-center gap-1.5 text-xs',
        className,
      )}
      title={
        inProgress > 0
          ? t('plan.progress_title_active', { completed, total, inProgress })
          : t('plan.progress_title', { completed, total })
      }>
      {inProgress > 0 && (
        <StatusIndicator status="in-progress" size="sm" aria-hidden />
      )}
      <span className="tabular-nums">{t('plan.steps', { completed, total })}</span>
    </span>
  );
}

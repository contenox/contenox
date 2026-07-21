import { Badge, P, Section, Span, StatusIndicator } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import {
  PLAN_ENTRY_PRIORITY_LABEL_KEY,
  PLAN_ENTRY_STATUS_INDICATOR,
  PLAN_ENTRY_STATUS_LABEL_KEY,
  planProgress,
} from '../lib/missionPlan';
import type { MissionPlan } from '../lib/types';

/**
 * A mission's living plan on the detail surface: the entries as a checklist
 * (each with a status chip and a quiet priority marker), the current revision,
 * and — when the agent recorded one — the latest revision's `explanation` as the
 * "why it changed" line.
 *
 * Renders NOTHING when there is no plan: an absent plan, or the zero Plan a
 * never-planned mission carries (revision 0, no entries). An unplanned mission
 * therefore shows no panel at all, not an empty shell — matching the report
 * section's "say so plainly, fabricate nothing" stance.
 */
export function MissionPlanPanel({ plan }: { plan?: MissionPlan | null }) {
  const { t } = useTranslation();
  const progress = planProgress(plan);
  if (!plan || !progress) return null;

  const entries = plan.entries ?? [];
  const { completed, total } = progress;

  return (
    <Section title={t('plan.panel_title')}>
      <div className="mt-2 flex flex-wrap items-center gap-2">
        <Badge variant="secondary" size="sm">
          {t('plan.revision', { n: plan.revision })}
        </Badge>
        <span className="text-text-muted dark:text-dark-text-muted text-xs tabular-nums">
          {t('plan.steps', { completed, total })}
        </span>
      </div>

      {/* The agent's own account of the latest revision — the "why it changed"
          line, shown plainly rather than as something clickable. */}
      {plan.explanation && (
        <P variant="muted" className="mt-3 whitespace-pre-wrap text-sm">
          <span className="font-medium">{t('plan.why_label')}: </span>
          {plan.explanation}
        </P>
      )}

      <ol className="mt-4 space-y-2">
        {entries.map(entry => (
          <li
            key={entry.id}
            className="border-surface-200 dark:border-dark-surface-600 flex items-start justify-between gap-3 rounded-lg border p-3">
            <div className="flex min-w-0 items-start gap-2">
              <span className="mt-0.5 shrink-0">
                <StatusIndicator
                  status={PLAN_ENTRY_STATUS_INDICATOR[entry.status]}
                  label={t(PLAN_ENTRY_STATUS_LABEL_KEY[entry.status])}
                  showIcon
                  size="sm"
                />
              </span>
              <span className="text-text dark:text-dark-text min-w-0 break-words">
                {entry.content}
              </span>
            </div>
            <Span
              variant="muted"
              className="shrink-0 text-xs"
              title={t('plan.priority_title')}>
              {t(PLAN_ENTRY_PRIORITY_LABEL_KEY[entry.priority])}
            </Span>
          </li>
        ))}
      </ol>
    </Section>
  );
}

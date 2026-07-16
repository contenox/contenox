import { Badge, Collapsible, Panel, Span } from '@contenox/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { PlanEntry, PlanEntryStatus } from '../../../lib/acp';

function statusVariant(status: PlanEntryStatus): 'success' | 'primary' | 'secondary' {
  if (status === 'completed') return 'success';
  if (status === 'in_progress') return 'primary';
  return 'secondary';
}

/**
 * The agent's plan, rendered as a collapsible section above the transcript —
 * on every breakpoint, not just mobile. A dedicated side rail would need its
 * own responsive scaffolding for little benefit at this scale (see Stage 3
 * blueprint: "a collapsible section is fine").
 */
export function PlanPanel({ entries }: { entries: PlanEntry[] }) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);

  if (entries.length === 0) return null;

  const done = entries.filter(e => e.status === 'completed').length;

  return (
    <Panel variant="surface" className="mx-3 mt-3 sm:mx-4">
      <Collapsible
        open={open}
        onOpenChange={setOpen}
        title={
          <span className="flex items-center gap-2">
            <Span variant="status">{t('acp_chat.plan_title')}</Span>
            <Badge variant="secondary" size="sm">
              {done}/{entries.length}
            </Badge>
          </span>
        }
      >
        <ol className="mt-2 space-y-1.5 text-sm">
          {entries.map((entry, i) => (
            // Keyed by content + arrival index (not just `i`): `plan` replaces
            // the whole entries array wholesale on every update, and content
            // is the one stable identity a step has (no server-issued id) —
            // keying by it alone would collide on duplicate step text, so the
            // index is a tiebreaker, not the primary identity.
            <li key={`${entry.content}-${i}`} className="flex items-start gap-2">
              <Badge size="sm" variant={statusVariant(entry.status)} className="mt-0.5 shrink-0">
                {t(`acp_chat.plan_status_${entry.status}`)}
              </Badge>
              <span className="text-text dark:text-dark-text">{entry.content}</span>
            </li>
          ))}
        </ol>
      </Collapsible>
    </Panel>
  );
}

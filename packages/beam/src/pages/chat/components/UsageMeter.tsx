import { ProgressBar, Span } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { AcpUsageState } from '../../../hooks/acpSessionState';

/** Compact used/size meter for the session's context window — hidden entirely until the agent reports usage. */
export function UsageMeter({ usage }: { usage: AcpUsageState | null }) {
  const { t } = useTranslation();
  if (!usage || usage.size <= 0) return null;

  const pct = Math.min(100, Math.max(0, Math.round((usage.used / usage.size) * 100)));
  const palette = pct >= 90 ? 'error' : pct >= 70 ? 'warning' : 'neutral';

  return (
    <div className="flex items-center gap-2" aria-label={t('acp_chat.usage_label')}>
      <ProgressBar value={pct} palette={palette} className="w-16 sm:w-28" />
      <Span variant="muted" className="text-xs tabular-nums">
        {usage.used.toLocaleString()} / {usage.size.toLocaleString()}
      </Span>
    </div>
  );
}

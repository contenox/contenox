import { ProgressBar, Span } from '@contenox/ui';
import { cn } from '../lib/utils';

function contextUsagePalette(pct: number): 'primary' | 'warning' | 'error' {
  if (pct > 90) return 'error';
  if (pct > 70) return 'warning';
  return 'primary';
}

function contextUsageTextClass(pct: number): string | undefined {
  if (pct > 90) return 'text-error dark:text-dark-error';
  if (pct > 70) return 'text-warning dark:text-dark-warning';
  return undefined;
}

/** Compact context-window usage meter shared by the chat toolbar and the console status line. */
export function ContextUsageMeter({ used, size }: { used: number; size: number }) {
  const safeUsed = Math.max(0, used);
  const pct = Math.round((safeUsed / size) * 100);
  const palette = contextUsagePalette(pct);
  const textClass = contextUsageTextClass(pct);
  const usedLabel = safeUsed > 0 ? `${safeUsed.toLocaleString()}/` : '';
  const title =
    safeUsed > 0
      ? `Context: ${safeUsed.toLocaleString()} / ${size.toLocaleString()} tokens (${pct}%)`
      : `Context window: ${size.toLocaleString()} tokens`;

  return (
    <div
      className="ml-3 flex shrink-0 items-center gap-2 text-xs font-medium tabular-nums"
      title={title}>
      <Span variant={textClass ? undefined : 'muted'} className={cn(textClass, 'tabular-nums')}>
        {usedLabel}
        {size.toLocaleString()}
      </Span>
      <ProgressBar
        value={Math.min(100, Math.max(0, pct))}
        palette={palette}
        className="h-2 w-24 bg-surface-200 shadow-inner dark:bg-dark-surface-300"
      />
      <Span variant={textClass ? undefined : 'muted'} className={cn(textClass, 'tabular-nums')}>
        {pct}%
      </Span>
    </div>
  );
}

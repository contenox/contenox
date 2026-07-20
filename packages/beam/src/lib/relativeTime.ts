/**
 * Locale-aware "x ago" for an ISO timestamp. Originated in FleetPage (instance
 * `startedAt`) mirroring the sidebar's relativeTimeLabel
 * (components/sidebar/AcpSessionSidebar.tsx), and shared here so the mission
 * pages (`lastHeartbeat`, report `createdAt`) reuse the exact same idiom
 * instead of a third copy of the same rounding rules. Falls back to the raw
 * string if the timestamp is unparseable.
 */
export function relativeTime(iso: string, locale: string, justNow: string): string {
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

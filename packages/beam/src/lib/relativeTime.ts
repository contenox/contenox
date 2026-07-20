/**
 * Locale-aware "x ago" for an ISO timestamp: "just now" below 45s, otherwise
 * `Intl.RelativeTimeFormat` rounded to the coarsest sensible unit (minutes,
 * hours, days, months, years). Falls back to the raw string if the timestamp
 * is unparseable. The single shared implementation for every relative-time
 * label in the app — FleetPage (`startedAt`), the mission pages
 * (`lastHeartbeat`, report `createdAt`), and the ACP session sidebar
 * (`updatedAt`) — so they all round the same way. Takes a non-optional ISO
 * string and always returns a string; a caller with an optional timestamp
 * (e.g. the sidebar, which renders nothing for a session with none) checks
 * for absence itself before calling in, rather than this function guessing
 * what "absent" should mean for every caller.
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

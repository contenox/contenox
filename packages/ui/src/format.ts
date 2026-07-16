/**
 * Formats a number using locale-aware compact notation (e.g. 12345 -> "12.3K",
 * 1500000 -> "1.5M"). Thin wrapper around Intl.NumberFormat so call sites do
 * not each re-specify the same options. `locale` defaults to the runtime's
 * default locale (the user's browser/OS locale) when omitted.
 *
 * Pair with the exact value (e.g. as a `title` attribute) when precision
 * matters — compaction is lossy by design.
 */
export function formatCompactNumber(value: number, locale?: string): string {
  return new Intl.NumberFormat(locale, {
    notation: "compact",
    maximumFractionDigits: 1,
  }).format(value);
}

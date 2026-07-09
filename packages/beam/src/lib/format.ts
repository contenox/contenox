export function formatBytes(value: number | undefined): string | undefined {
  if (!value || value <= 0) return undefined;
  const units = ['B', 'KiB', 'MiB', 'GiB', 'TiB'];
  let next = value;
  let unit = 0;
  while (next >= 1024 && unit < units.length - 1) {
    next /= 1024;
    unit += 1;
  }
  return `${next >= 10 || unit === 0 ? next.toFixed(0) : next.toFixed(1)} ${units[unit]}`;
}

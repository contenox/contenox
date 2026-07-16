/**
 * Small pure helpers backing the Settings page's config-key forms — split out
 * so the validation/defaulting rules (which mirror Go-side behavior in
 * runtime/stateservice and runtime/contenoxcli) are unit-testable without
 * mounting the form components.
 */

/** Mirrors stateservice.normalizeDefaultMaxTokens: empty is valid (clears the
 * override); otherwise must be a non-negative integer. */
export function isValidMaxTokens(value: string): boolean {
  const trimmed = value.trim();
  if (trimmed === '') return true;
  return /^\d+$/.test(trimmed);
}

/** telemetry-enabled is opt-in: off unless the stored value is literally
 * "true" (matches runtime/contenoxcli/cli.go's setupTelemetryLogging). */
export function resolveTelemetryEnabled(value: string | undefined): boolean {
  return value === 'true';
}

/** update-check is opt-out: on unless the stored value is literally "false"
 * (matches runtime/contenoxcli/update_cmd.go's isUpdateCheckDisabled). */
export function resolveUpdateCheckEnabled(value: string | undefined): boolean {
  return value !== 'false';
}

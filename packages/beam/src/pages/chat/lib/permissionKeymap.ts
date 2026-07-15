import type { PermissionOption, PermissionOptionKind } from '../../../lib/acp';

/**
 * Pure keyboard-mapping logic for the permission gate (PermissionGate.tsx).
 * Mapping is by option KIND, not by button position — the agent controls the
 * order/wording of `options`, so a fixed "first button = 'y'" mapping would
 * silently rebind if the agent ever reorders its offers. No React, no DOM.
 */

const KEY_TO_KIND: Record<string, PermissionOptionKind> = {
  y: 'allow_once',
  a: 'allow_always',
  n: 'reject_once',
};

/** Resolves a single keypress (already lower/upper-case agnostic) to the option it should trigger, or null if the key has no binding or no option of that kind was offered. */
export function optionForKey(options: PermissionOption[], key: string): PermissionOption | null {
  const kind = KEY_TO_KIND[key.toLowerCase()];
  if (!kind) return null;
  return options.find(o => o.kind === kind) ?? null;
}

/**
 * The option Escape/backdrop-dismiss should resolve to: `reject_once` if
 * offered, else any `reject_*` kind, else null — a permission gate must never
 * silently pick an "allow" on dismiss, so a request with no reject option at
 * all yields null (caller must not auto-dismiss; the user must pick explicitly).
 */
export function safestRejectOption(options: PermissionOption[]): PermissionOption | null {
  return options.find(o => o.kind === 'reject_once') ?? options.find(o => o.kind.startsWith('reject')) ?? null;
}

/** Render order: allow options before reject options; stable otherwise (preserves the agent's within-group ordering). */
export function orderedPermissionOptions(options: PermissionOption[]): PermissionOption[] {
  const rank = (o: PermissionOption) => (o.kind.startsWith('allow') ? 0 : o.kind.startsWith('reject') ? 1 : 2);
  return options
    .map((option, index) => ({ option, index }))
    .sort((a, b) => rank(a.option) - rank(b.option) || a.index - b.index)
    .map(({ option }) => option);
}

/** The key-hint label shown on a button (uppercase, matches the keydown mapping above); null for kinds with no binding (e.g. `reject_always`, which has no dedicated key to avoid a destructive action needing only one keypress). */
export function keyHintForOption(option: PermissionOption): string | null {
  switch (option.kind) {
    case 'allow_once':
      return 'Y';
    case 'allow_always':
      return 'A';
    case 'reject_once':
      return 'N';
    default:
      return null;
  }
}

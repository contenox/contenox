import type { SetupStatus } from './types';

const STORAGE_KEY = 'beam_setup_wizard_dismissal';

export type WizardDismissalRecord = { fingerprint: string };

export type WizardDismissalSnapshot = {
  /** The setup-state fingerprint the wizard was last dismissed under, or null if not dismissed. */
  record: WizardDismissalRecord | null;
  /** True while the wizard was force-opened (Settings' "Run setup wizard" reconfiguration entry),
   * bypassing the normal "only show when setup is incomplete" gate. */
  manualOpen: boolean;
};

// In-memory fallback so dismissal still works for the current tab session when
// localStorage is unavailable (private browsing) or absent (SSR/tests) —
// degrades to "forgotten on reload" instead of the wizard being unable to be
// dismissed at all.
const memoryFallback = new Map<string, string>();

function getStorageValue(key: string): string | null {
  try {
    if (typeof localStorage !== 'undefined') {
      return localStorage.getItem(key);
    }
  } catch {
    /* best-effort: localStorage may be unavailable (private mode) */
  }
  return memoryFallback.get(key) ?? null;
}

function setStorageValue(key: string, value: string | null): void {
  try {
    if (typeof localStorage !== 'undefined') {
      if (value === null) {
        localStorage.removeItem(key);
      } else {
        localStorage.setItem(key, value);
      }
      return;
    }
  } catch {
    /* best-effort: localStorage may be unavailable (private mode) */
  }
  if (value === null) {
    memoryFallback.delete(key);
  } else {
    memoryFallback.set(key, value);
  }
}

/**
 * Sorted, stable fingerprint of the current "incomplete setup" state — the
 * same inputs Layout uses to compute `setupComplete` (error-severity issues +
 * whether any backend is reachable). Comparing fingerprints lets the wizard
 * tell "still the same broken state the user dismissed" apart from "setup
 * regressed differently since then" (e.g. a new error, or backends going
 * unreachable again), so a regression re-arms the wizard instead of it
 * staying hidden forever.
 */
export function computeSetupFingerprint(status: SetupStatus | undefined | null): string {
  if (!status) return 'unknown';
  const errorCodes = (status.issues ?? [])
    .filter(issue => issue.severity === 'error')
    .map(issue => issue.code)
    .sort();
  const reachable = status.reachableBackendCount > 0 ? '1' : '0';
  return `${reachable}|${errorCodes.join(',')}`;
}

type Listener = () => void;
const listeners = new Set<Listener>();

function readRecord(): WizardDismissalRecord | null {
  const raw = getStorageValue(STORAGE_KEY);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw) as { fingerprint?: unknown };
    return typeof parsed.fingerprint === 'string' ? { fingerprint: parsed.fingerprint } : null;
  } catch {
    return null;
  }
}

let snapshot: WizardDismissalSnapshot = { record: readRecord(), manualOpen: false };

function setSnapshot(next: WizardDismissalSnapshot): void {
  snapshot = next;
  listeners.forEach(listener => listener());
}

/** Stable-reference getter for useSyncExternalStore — only changes identity via setSnapshot. */
export function getWizardDismissalSnapshot(): WizardDismissalSnapshot {
  return snapshot;
}

export function subscribeWizardDismissal(listener: Listener): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

/**
 * Records that the wizard was dismissed under `status`'s current state and
 * clears any pending manual-open override. Call when the user dismisses the
 * wizard or finishes a step (OnboardingWizard's onDismiss/onFinish).
 */
export function dismissWizard(status: SetupStatus | undefined | null): void {
  const fingerprint = computeSetupFingerprint(status);
  setStorageValue(STORAGE_KEY, JSON.stringify({ fingerprint } satisfies WizardDismissalRecord));
  setSnapshot({ record: { fingerprint }, manualOpen: false });
}

/**
 * Clears the dismissal record so the wizard re-arms for the current state.
 * Used by the persistent "Setup incomplete" escape-hatch banner.
 */
export function clearWizardDismissal(): void {
  setStorageValue(STORAGE_KEY, null);
  setSnapshot({ record: null, manualOpen: snapshot.manualOpen });
}

/**
 * Forces the wizard open regardless of setup completeness — the Settings
 * page's "Run setup wizard" reconfiguration entry.
 */
export function openWizardManually(): void {
  setStorageValue(STORAGE_KEY, null);
  setSnapshot({ record: null, manualOpen: true });
}

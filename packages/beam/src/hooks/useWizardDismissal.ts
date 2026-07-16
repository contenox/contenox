import { useSyncExternalStore } from 'react';
import {
  getWizardDismissalSnapshot,
  subscribeWizardDismissal,
  type WizardDismissalSnapshot,
} from '../lib/wizardDismissal';

const SERVER_SNAPSHOT: WizardDismissalSnapshot = { record: null, manualOpen: false };

/**
 * Reactive view of the wizard's dismissal/manual-open state. Backed by a
 * module-level store (see lib/wizardDismissal.ts) rather than component
 * state, so Layout (renders the wizard) and the Settings page (offers a
 * "Run setup wizard" reconfiguration entry) observe the same state without
 * prop drilling — both are descendants of Layout in the route tree, but
 * Settings is rendered as Layout's `mainContent`, not a parent.
 */
export function useWizardDismissal(): WizardDismissalSnapshot {
  return useSyncExternalStore(
    subscribeWizardDismissal,
    getWizardDismissalSnapshot,
    () => SERVER_SNAPSHOT,
  );
}

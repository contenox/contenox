import { beforeEach, describe, expect, it } from 'vitest';
import type { SetupStatus } from './types';
import {
  clearWizardDismissal,
  computeSetupFingerprint,
  dismissWizard,
  getWizardDismissalSnapshot,
  openWizardManually,
  subscribeWizardDismissal,
} from './wizardDismissal';

function status(partial: Partial<SetupStatus>): SetupStatus {
  return {
    defaultModel: '',
    defaultProvider: '',
    defaultChain: '',
    hitlPolicyName: '',
    backendCount: 0,
    reachableBackendCount: 0,
    issues: [],
    backendChecks: [],
    ...partial,
  };
}

describe('computeSetupFingerprint', () => {
  it('returns a stable placeholder for undefined/null status', () => {
    expect(computeSetupFingerprint(undefined)).toBe('unknown');
    expect(computeSetupFingerprint(null)).toBe('unknown');
  });

  it('is order-independent in the error issue codes', () => {
    const a = status({
      issues: [
        { code: 'zeta', severity: 'error', message: 'z' },
        { code: 'alpha', severity: 'error', message: 'a' },
      ],
    });
    const b = status({
      issues: [
        { code: 'alpha', severity: 'error', message: 'a' },
        { code: 'zeta', severity: 'error', message: 'z' },
      ],
    });
    expect(computeSetupFingerprint(a)).toBe(computeSetupFingerprint(b));
  });

  it('ignores non-error issues', () => {
    const withWarning = status({
      issues: [{ code: 'no_backends', severity: 'warning', message: 'n' }],
    });
    const withoutIssues = status({ issues: [] });
    expect(computeSetupFingerprint(withWarning)).toBe(computeSetupFingerprint(withoutIssues));
  });

  it('distinguishes reachable vs unreachable backend counts', () => {
    const reachable = status({ reachableBackendCount: 1 });
    const unreachable = status({ reachableBackendCount: 0 });
    expect(computeSetupFingerprint(reachable)).not.toBe(computeSetupFingerprint(unreachable));
  });

  it('changes when the set of error codes changes (regression detection)', () => {
    const before = status({
      issues: [{ code: 'missing_default_model', severity: 'error', message: 'm' }],
    });
    const after = status({
      issues: [
        { code: 'missing_default_model', severity: 'error', message: 'm' },
        { code: 'all_backends_unreachable', severity: 'error', message: 'u' },
      ],
    });
    expect(computeSetupFingerprint(before)).not.toBe(computeSetupFingerprint(after));
  });
});

describe('wizard dismissal store', () => {
  beforeEach(() => {
    // Reset to a clean, undismissed state before each test.
    clearWizardDismissal();
  });

  it('starts undismissed with no manual override', () => {
    const snap = getWizardDismissalSnapshot();
    expect(snap.record).toBeNull();
    expect(snap.manualOpen).toBe(false);
  });

  it('dismissWizard records the fingerprint of the given status and clears manual-open', () => {
    openWizardManually();
    expect(getWizardDismissalSnapshot().manualOpen).toBe(true);

    const broken = status({
      issues: [{ code: 'missing_default_model', severity: 'error', message: 'm' }],
    });
    dismissWizard(broken);

    const snap = getWizardDismissalSnapshot();
    expect(snap.record).toEqual({ fingerprint: computeSetupFingerprint(broken) });
    expect(snap.manualOpen).toBe(false);
  });

  it('clearWizardDismissal re-arms without touching manualOpen', () => {
    dismissWizard(status({}));
    expect(getWizardDismissalSnapshot().record).not.toBeNull();

    clearWizardDismissal();
    expect(getWizardDismissalSnapshot().record).toBeNull();
  });

  it('openWizardManually clears any dismissal and sets manualOpen', () => {
    dismissWizard(status({}));
    openWizardManually();

    const snap = getWizardDismissalSnapshot();
    expect(snap.record).toBeNull();
    expect(snap.manualOpen).toBe(true);
  });

  it('notifies subscribers on every mutation', () => {
    let calls = 0;
    const unsubscribe = subscribeWizardDismissal(() => {
      calls += 1;
    });

    dismissWizard(status({}));
    clearWizardDismissal();
    openWizardManually();

    expect(calls).toBe(3);
    unsubscribe();

    dismissWizard(status({}));
    expect(calls).toBe(3); // no further notifications once unsubscribed
  });
});

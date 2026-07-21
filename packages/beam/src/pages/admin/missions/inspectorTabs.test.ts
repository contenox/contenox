import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  MISSION_INSPECTOR_TABS,
  readStoredTab,
  resolveActiveTab,
  writeStoredTab,
} from './inspectorTabs';

describe('MISSION_INSPECTOR_TABS', () => {
  it('ships overview first (the default), then changes, search, and terminal', () => {
    expect(MISSION_INSPECTOR_TABS.map(t => t.id)).toEqual([
      'overview',
      'changes',
      'search',
      'terminal',
    ]);
  });
});

describe('resolveActiveTab', () => {
  it('keeps a stored id that still exists', () => {
    expect(resolveActiveTab('terminal')).toBe('terminal');
  });

  it('falls back to the first tab when nothing is stored', () => {
    expect(resolveActiveTab(null)).toBe('overview');
  });

  it('falls back to the first tab when the stored id no longer exists', () => {
    expect(resolveActiveTab('a-tab-that-was-removed')).toBe('overview');
  });

  it('respects a custom tab list (subset registration)', () => {
    const custom = [
      { id: 'changes', icon: MISSION_INSPECTOR_TABS[0].icon, labelKey: 'inspector.tab_overview' as const },
    ];
    expect(resolveActiveTab('terminal', custom)).toBe('changes');
    expect(resolveActiveTab('changes', custom)).toBe('changes');
  });
});

describe('active-tab persistence', () => {
  let store: Record<string, string>;

  beforeEach(() => {
    store = {};
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (k in store ? store[k] : null),
      setItem: (k: string, v: string) => {
        store[k] = v;
      },
      removeItem: (k: string) => {
        delete store[k];
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('round-trips the selected tab through localStorage', () => {
    expect(readStoredTab()).toBeNull();
    writeStoredTab('terminal');
    expect(readStoredTab()).toBe('terminal');
  });

  it('degrades to no persistence when localStorage throws', () => {
    vi.stubGlobal('localStorage', {
      getItem: () => {
        throw new Error('blocked');
      },
      setItem: () => {
        throw new Error('blocked');
      },
    });
    expect(() => writeStoredTab('terminal')).not.toThrow();
    expect(readStoredTab()).toBeNull();
  });
});

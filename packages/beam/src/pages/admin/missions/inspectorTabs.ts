import { FileDiff, Info, Search, Terminal, type LucideProps } from 'lucide-react';
import { useCallback, useEffect, useState, type ComponentType } from 'react';
import type { TranslationKey } from '../../../i18n';

/**
 * The mission inspector's tab-registration shape (Arc 4 of ide-workflows.md).
 * This is the extension point the roadmap calls out: the Changes and Search
 * slices land by ADDING an entry here — id + icon + label key — with no change
 * to the tab bar itself. Nothing about a tab's CONTENT lives in this shape; the
 * page maps an id to its panel, so registering a tab and building its body stay
 * decoupled. No placeholders are shipped for unbuilt tabs — the list simply
 * gains entries when their slices are real.
 */
export interface MissionInspectorTab {
  /** Stable id, persisted as the active-tab selection and used to key content. */
  id: string;
  /** Lucide icon component rendered beside the label. */
  icon: ComponentType<LucideProps>;
  /** i18n key for the tab label (en + de required). */
  labelKey: TranslationKey;
}

/**
 * The tabs shipped today: the mission facts/reports/plan under Overview, and the
 * host Terminal. Changes/Search append here when built. Exported as data (not
 * hard-coded in the component) precisely so a new slice is a one-line addition.
 */
export const MISSION_INSPECTOR_TABS: readonly MissionInspectorTab[] = [
  { id: 'overview', icon: Info, labelKey: 'inspector.tab_overview' },
  { id: 'changes', icon: FileDiff, labelKey: 'inspector.tab_changes' },
  { id: 'search', icon: Search, labelKey: 'inspector.tab_search' },
  { id: 'terminal', icon: Terminal, labelKey: 'inspector.tab_terminal' },
];

const STORAGE_KEY = 'beam.missionInspector.activeTab';

/**
 * The active tab to show given a persisted id and the currently-registered
 * tabs: the stored id when it still exists, else the first tab. Pure, so a
 * stored id from a tab that has since been removed (or a first run with nothing
 * stored) resolves deterministically without touching storage.
 */
export function resolveActiveTab(
  stored: string | null,
  tabs: readonly MissionInspectorTab[] = MISSION_INSPECTOR_TABS,
): string {
  if (stored && tabs.some(t => t.id === stored)) return stored;
  return tabs[0]?.id ?? 'overview';
}

export function readStoredTab(): string | null {
  try {
    if (typeof localStorage !== 'undefined') return localStorage.getItem(STORAGE_KEY);
  } catch {
    /* best-effort */
  }
  return null;
}

export function writeStoredTab(id: string): void {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(STORAGE_KEY, id);
  } catch {
    /* best-effort */
  }
}

/**
 * Active-tab state for the mission inspector, localStorage-persisted per the
 * sibling toggle idiom (usePersistentToggle). Re-resolves if the active tab
 * disappears from `tabs` (e.g. a tab is unregistered), so a stale selection can
 * never strand the panel on an id that no longer renders.
 */
export function useInspectorTab(
  tabs: readonly MissionInspectorTab[] = MISSION_INSPECTOR_TABS,
): [string, (id: string) => void] {
  const [active, setActive] = useState(() => resolveActiveTab(readStoredTab(), tabs));

  useEffect(() => {
    const resolved = resolveActiveTab(active, tabs);
    if (resolved !== active) setActive(resolved);
  }, [active, tabs]);

  const select = useCallback((id: string) => {
    setActive(id);
    writeStoredTab(id);
  }, []);

  return [active, select];
}

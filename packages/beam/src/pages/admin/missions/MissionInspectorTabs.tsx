import { Tabs } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { MissionInspectorTab } from './inspectorTabs';

export interface MissionInspectorTabsProps {
  tabs: readonly MissionInspectorTab[];
  activeTab: string;
  onTabChange: (id: string) => void;
}

/**
 * The flat tab bar on the mission detail — a thin, accessible wrapper over the
 * house `Tabs` (keyboard nav, `role=tab`), mapping each registration entry to a
 * trigger whose label is `icon + text`. It owns no state: the page owns the
 * persisted active-tab hook and the per-id content, so this stays purely
 * presentational and future Changes/Search tabs render here for free.
 */
export function MissionInspectorTabs({ tabs, activeTab, onTabChange }: MissionInspectorTabsProps) {
  const { t } = useTranslation();

  const uiTabs = tabs.map(tab => {
    const Icon = tab.icon;
    return {
      id: tab.id,
      label: (
        <span className="inline-flex items-center gap-1.5">
          <Icon aria-hidden="true" className="h-4 w-4" />
          {t(tab.labelKey)}
        </span>
      ),
    };
  });

  return (
    <div className="border-surface-200 dark:border-dark-surface-600 border-b">
      <Tabs tabs={uiTabs} activeTab={activeTab} onTabChange={onTabChange} />
    </div>
  );
}

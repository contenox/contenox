/**
 * i18n keys referenced in this file (namespace `canvas`/`terminal`; see i18n.ts):
 *   terminal.panel_title  = "Terminal"          (canvas terminal-tab label)
 *   canvas.close_tab      = "Close {{name}}"     (canvas tab ✕ aria-label)
 */
import { cn, ResizablePanel, ResizablePanelGroup, ResizablePanelHandle, Tabs, TabPanel, TabPanels, type Tab } from '@contenox/ui';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import type { CanvasTab } from '../lib/canvasTabs';
import type { UseCanvasTabsResult } from '../../../hooks/useCanvasTabs';
import { TerminalTab } from './TerminalTab';

/** The canvas becomes a side-by-side split at/above this width; below it, a full-width takeover. */
const SIDE_BY_SIDE_QUERY = '(min-width: 1024px)';

export interface CanvasRegionProps {
  /** The focused session whose surfaces the canvas reflects (`null` = empty/new-chat). */
  sessionId: string | null;
  /** The canvas tab-model (open list, active id, open/close/focus). */
  canvas: UseCanvasTabsResult;
  /** The primary (chat) body — rendered beside the canvas when open, full-width when the canvas is empty. */
  children: ReactNode;
  className?: string;
}

/**
 * The chat's secondary CANVAS region (workspace-canvas Slice B1): a resizable,
 * tabbed pane to the RIGHT of the chat body holding the terminal (B1) and, later,
 * file/diff surfaces (B2+). It wraps the chat `children` and:
 *
 *  - **Collapses** entirely when no canvas tab is open — the chat takes the full
 *    width and no split/handle renders.
 *  - **Side-by-side** at ≥1024px: an `@contenox/ui` `ResizablePanelGroup` splits
 *    chat | canvas with a draggable handle; the canvas defaults to 480px and is
 *    resizable within bounds.
 *  - **Full-width takeover** below 1024px: the canvas fills the region so it is
 *    never a cramped side rail on a narrow viewport. The chat body stays MOUNTED
 *    (hidden) so its stream/draft survive; closing the canvas tab returns to it.
 *
 * The breakpoint is resolved in JS (`useMediaQuery`) rather than pure CSS because
 * the split's sizing lives in `ResizablePanel`'s inline flex styles, which a
 * responsive class can't override — so the two layouts are distinct subtrees, and
 * `children` renders in exactly one of them at a time (single mount).
 */
export function CanvasRegion({ sessionId, canvas, children, className }: CanvasRegionProps) {
  const { t } = useTranslation();
  const sideBySide = useMediaQuery(SIDE_BY_SIDE_QUERY);

  // Collapsed: no canvas tab open — the chat body takes the full width.
  if (canvas.tabs.length === 0) {
    return <div className={cn('flex min-h-0 min-w-0 flex-1 flex-col', className)}>{children}</div>;
  }

  const tabLabel = (tab: CanvasTab): string => {
    switch (tab.kind) {
      case 'terminal':
        return t('terminal.panel_title');
      default:
        return tab.id;
    }
  };

  const stripTabs: Tab[] = canvas.tabs.map(tab => {
    const label = tabLabel(tab);
    return { id: tab.id, label, closable: true, closeLabel: t('canvas.close_tab', { name: label }) };
  });
  const activeId = canvas.activeId ?? '';

  const surface = (
    <div className="bg-surface dark:bg-dark-surface flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center border-b px-2">
        <Tabs
          tabs={stripTabs}
          activeTab={activeId}
          onTabChange={canvas.focus}
          onClose={canvas.close}
          className="min-w-0 flex-1 flex-nowrap"
        />
      </div>
      <TabPanels>
        {canvas.tabs.map(tab => (
          <TabPanel key={tab.id} tabId={tab.id} activeTab={activeId} className="p-2">
            {tab.kind === 'terminal' && <TerminalTab sessionId={sessionId} />}
          </TabPanel>
        ))}
      </TabPanels>
    </div>
  );

  // Narrow: full-width takeover — the canvas replaces the (still-mounted) chat.
  if (!sideBySide) {
    return (
      <div className={cn('flex min-h-0 min-w-0 flex-1 flex-col', className)}>
        <div className="hidden">{children}</div>
        {surface}
      </div>
    );
  }

  // Wide: side-by-side resizable chat | canvas split.
  return (
    <ResizablePanelGroup className={cn('min-w-0 flex-1', className)}>
      <ResizablePanel className="flex min-h-0 flex-col">{children}</ResizablePanel>
      <ResizablePanelHandle orientation="horizontal" />
      <ResizablePanel
        defaultSize="480px"
        minSize={320}
        maxSize={960}
        className="border-surface-200 dark:border-dark-surface-600 flex min-h-0 flex-col border-l">
        {surface}
      </ResizablePanel>
    </ResizablePanelGroup>
  );
}

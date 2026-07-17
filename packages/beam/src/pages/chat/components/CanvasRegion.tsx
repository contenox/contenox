/**
 * i18n keys referenced in this file (namespace `canvas`/`terminal`; see i18n.ts):
 *   terminal.panel_title    = "Terminal"          (canvas terminal-tab label)
 *   terminal.open_in_canvas = "Open the terminal…" (open-terminal affordance)
 *   canvas.close_tab        = "Close {{name}}"     (canvas tab ✕ aria-label)
 */
import { Button, cn, ResizablePanel, ResizablePanelGroup, ResizablePanelHandle, Tabs, TabPanel, TabPanels, type Tab } from '@contenox/ui';
import { Terminal } from 'lucide-react';
import { useCallback, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import type { WorkspaceFilePeek } from '../../../hooks/useWorkspaceFiles';
import { TERMINAL_CANVAS_TAB, type CanvasTab } from '../lib/canvasTabs';
import type { UseCanvasTabsResult } from '../../../hooks/useCanvasTabs';
import type { RequestPermissionRequest } from '../../../lib/acp';
import { TerminalTab } from './TerminalTab';
import { FileViewTab } from './FileViewTab';
import { ApprovalViewTab } from './ApprovalViewTab';

/** The canvas becomes a side-by-side split at/above this width; below it, a full-width takeover. */
const SIDE_BY_SIDE_QUERY = '(min-width: 1024px)';

export interface CanvasRegionProps {
  /** The focused session whose surfaces the canvas reflects (`null` = empty/new-chat). */
  sessionId: string | null;
  /** The canvas tab-model (open list, active id, open/close/focus). */
  canvas: UseCanvasTabsResult;
  /** Reads a file's content — threaded to the read-only `file` canvas tabs. */
  readFile: (path: string) => Promise<WorkspaceFilePeek>;
  /** The session's live pending permission (or null) — threaded to `approval` canvas tabs so they know when they've gone stale. */
  pendingPermission: RequestPermissionRequest | null;
  /** The same responder the permission gate uses — threaded to `approval` canvas tabs. */
  onRespondPermission: (optionId: string) => void;
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
export function CanvasRegion({
  sessionId,
  canvas,
  readFile,
  pendingPermission,
  onRespondPermission,
  children,
  className,
}: CanvasRegionProps) {
  const { t } = useTranslation();
  const sideBySide = useMediaQuery(SIDE_BY_SIDE_QUERY);
  const { open } = canvas;

  // The single affordance for opening the terminal as a canvas tab (dedups to a
  // focus if already open) — replaces the old chat-toolbar toggle.
  const openTerminal = useCallback(() => open(TERMINAL_CANVAS_TAB), [open]);

  const openTerminalButton = (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      aria-label={t('terminal.open_in_canvas')}
      title={t('terminal.open_in_canvas')}
      onClick={openTerminal}>
      <Terminal className="h-4 w-4" />
    </Button>
  );

  // Collapsed: no canvas tab open — the chat body takes the full width, with a
  // slim rail carrying the always-available "open terminal" affordance.
  if (canvas.tabs.length === 0) {
    return (
      <div className={cn('flex min-h-0 min-w-0 flex-1', className)}>
        <div className="flex min-h-0 min-w-0 flex-1 flex-col">{children}</div>
        <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-col items-center gap-1 border-l px-1 py-2">
          {openTerminalButton}
        </div>
      </div>
    );
  }

  const tabLabel = (tab: CanvasTab): string => {
    switch (tab.kind) {
      case 'terminal':
        return t('terminal.panel_title');
      case 'file':
        return tab.title ?? tab.path ?? tab.id;
      case 'approval':
        return tab.title ?? t('acp_chat.permission_title');
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
      <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center gap-1 border-b px-2">
        <Tabs
          tabs={stripTabs}
          activeTab={activeId}
          onTabChange={canvas.focus}
          onClose={canvas.close}
          className="min-w-0 flex-1 flex-nowrap"
        />
        <span className="shrink-0">{openTerminalButton}</span>
      </div>
      <TabPanels>
        {canvas.tabs.map(tab => (
          <TabPanel key={tab.id} tabId={tab.id} activeTab={activeId} className="flex min-h-0 flex-col p-2">
            {tab.kind === 'terminal' && <TerminalTab sessionId={sessionId} />}
            {tab.kind === 'file' && tab.path && <FileViewTab path={tab.path} readFile={readFile} />}
            {tab.kind === 'approval' && tab.approval && (
              <ApprovalViewTab
                permission={tab.approval}
                pendingPermission={pendingPermission}
                active={tab.id === activeId}
                onRespond={onRespondPermission}
                onClose={() => canvas.close(tab.id)}
              />
            )}
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

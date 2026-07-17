/**
 * i18n keys referenced in this file (namespace `workspace`; add to i18n.ts):
 *   workspace.panel_title          = "Workspace"
 *   workspace.refresh              = "Refresh"
 *   workspace.loading              = "Loading…"
 *   workspace.empty                = "This workspace is empty"
 *   workspace.agent_view           = "Agent view"
 *   workspace.legend_allowed       = "allowed"
 *   workspace.legend_approval      = "needs approval"
 *   workspace.legend_blocked       = "blocked"
 *   workspace.legend_unreachable   = "unreachable"
 *   workspace.access_unreachable   = "Outside the workspace boundary"
 *   workspace.access_read          = "Read"
 *   workspace.access_write         = "Write"
 */
import { Button, FileTree, InlineNotice, type FileTreeNode } from '@contenox/ui';
import { RefreshCw, ShieldCheck } from 'lucide-react';
import { useCallback, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { type UseWorkspaceFilesResult } from '../../../hooks/useWorkspaceFiles';
import { toFileTreeNodes, type AccessLabels } from '../lib/workspaceTree';

export interface WorkspacePanelProps {
  /** The session workspace root; `null` when there is nothing to show yet. */
  root: string | null;
  /** Shared `useWorkspaceFiles(root)` result — owned by the page, also fed to the mention menu. */
  files: UseWorkspaceFilesResult;
  /** Opens a file as a read-only canvas tab (files no longer preview inline in this sidebar). */
  onOpenFile: (path: string) => void;
  /** The path of the file whose canvas tab is currently active, for row highlight. */
  selectedFilePath?: string | null;
}

/**
 * IDE-style file explorer for the session workspace: a lazily-loaded directory
 * tree backed by `useWorkspaceFiles`. Clicking a file opens it as a read-only
 * canvas tab (no inline preview lives here anymore). An optional "agent view"
 * overlays the active HITL policy's per-entry verdict (a colored status dot +
 * tooltip; unreachable rows are dimmed). Pure presentation — all
 * fetching/caching lives in the shared hook and the pure `workspaceTree` helpers.
 *
 * The panel's visibility is governed SOLELY by the chat toolbar's "Files"
 * toggle (a shared persistent toggle); it carries no collapse affordance of its
 * own, so there is exactly one open/close mechanism.
 */
export function WorkspacePanel({ root, files, onOpenFile, selectedFilePath }: WorkspacePanelProps) {
  const { t } = useTranslation();
  const { agentView, setAgentView } = files;

  const handleNodeSelect = useCallback(
    (node: FileTreeNode) => {
      const path = node.path ?? node.id;
      if (node.isDirectory) {
        files.ensureLoaded(path);
        return;
      }
      onOpenFile(path);
    },
    [files, onOpenFile],
  );

  const accessLabels = useMemo<AccessLabels>(
    () => ({
      unreachable: t('workspace.access_unreachable'),
      read: t('workspace.access_read'),
      write: t('workspace.access_write'),
    }),
    [t],
  );

  const nodes = useMemo(
    () => toFileTreeNodes(files.cache, undefined, agentView ? accessLabels : undefined),
    [files.cache, agentView, accessLabels],
  );

  if (!root) return null;

  const isEmptyRoot = !files.rootLoading && !files.error && nodes.length === 0;

  return (
    <div className="border-surface-200 bg-surface-50 dark:border-dark-surface-600 dark:bg-dark-surface-100 flex h-full w-64 min-w-0 shrink-0 flex-col border-r sm:w-72">
      <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2">
        <span className="text-text dark:text-dark-text truncate text-sm font-medium">{t('workspace.panel_title')}</span>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            type="button"
            variant={agentView ? 'primary' : 'ghost'}
            palette="neutral"
            size="icon"
            aria-pressed={agentView}
            aria-label={t('workspace.agent_view')}
            title={t('workspace.agent_view')}
            onClick={() => setAgentView(!agentView)}>
            <ShieldCheck className="h-3.5 w-3.5" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('workspace.refresh')}
            onClick={() => files.refresh()}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      {agentView && (
        <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b px-3 py-1.5 text-[11px] text-text-muted dark:text-dark-text-muted">
          <LegendItem dotClass="ring-1 ring-inset ring-success-500/60" label={t('workspace.legend_allowed')} />
          <LegendItem dotClass="bg-warning-500 dark:bg-dark-warning-500" label={t('workspace.legend_approval')} />
          <LegendItem dotClass="bg-error-500 dark:bg-dark-error-500" label={t('workspace.legend_blocked')} />
          <LegendItem dotClass="bg-text-muted dark:bg-dark-text-muted opacity-50" label={t('workspace.legend_unreachable')} />
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {files.error ? (
          <InlineNotice variant="error" className="mb-2">
            <div className="flex items-center justify-between gap-2">
              <span className="min-w-0 truncate">{files.error}</span>
              <Button type="button" variant="outline" size="xs" onClick={() => files.refresh()}>
                {t('workspace.refresh')}
              </Button>
            </div>
          </InlineNotice>
        ) : null}

        {files.rootLoading && nodes.length === 0 ? (
          <span className="text-text-muted dark:text-dark-text-muted block px-1 py-2 text-xs">{t('workspace.loading')}</span>
        ) : isEmptyRoot ? (
          <span className="text-text-muted dark:text-dark-text-muted block px-1 py-2 text-xs">{t('workspace.empty')}</span>
        ) : (
          <FileTree
            nodes={nodes}
            directoryClickMode="expand"
            defaultExpanded={new Set<string>()}
            selectedId={selectedFilePath ?? undefined}
            onNodeSelect={handleNodeSelect}
          />
        )}
      </div>
    </div>
  );
}

function LegendItem({ dotClass, label }: { dotClass: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span aria-hidden="true" className={`h-2 w-2 shrink-0 rounded-full ${dotClass}`} />
      {label}
    </span>
  );
}

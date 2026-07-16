/**
 * i18n keys referenced in this file (namespace `workspace`; add to i18n.ts):
 *   workspace.panel_title    = "Workspace"
 *   workspace.refresh        = "Refresh"
 *   workspace.collapse       = "Collapse workspace"
 *   workspace.loading        = "Loading…"
 *   workspace.binary_file    = "Binary file — cannot preview"
 *   workspace.line           = "line"
 *   workspace.lines          = "lines"
 *   workspace.empty          = "This workspace is empty"
 *   workspace.peek_error     = "Could not open file"
 */
import { Button, cn, FileTree, InlineAttachmentRenderer, InlineNotice, type FileTreeNode } from '@contenox/ui';
import { PanelLeftClose, RefreshCw } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { type UseWorkspaceFilesResult, type WorkspaceFilePeek } from '../../../hooks/useWorkspaceFiles';
import { toFileTreeNodes } from '../lib/workspaceTree';

export interface WorkspacePanelProps {
  /** The session workspace root; `null` when there is nothing to show yet. */
  root: string | null;
  /** Shared `useWorkspaceFiles(root)` result — owned by the page, also fed to the mention menu. */
  files: UseWorkspaceFilesResult;
  /** Collapses the panel. */
  onClose: () => void;
}

type PeekState = { status: 'loading'; path: string } | { status: 'loaded'; peek: WorkspaceFilePeek } | { status: 'error'; path: string };

/**
 * IDE-style file explorer for the session workspace: a lazily-loaded
 * directory tree backed by `useWorkspaceFiles`, with a preview pane for the
 * currently selected file. Pure presentation — all fetching/caching lives in
 * the shared hook and the pure `workspaceTree` helpers.
 */
export function WorkspacePanel({ root, files, onClose }: WorkspacePanelProps) {
  const { t } = useTranslation();
  const [peek, setPeek] = useState<PeekState | null>(null);

  const handleNodeSelect = useCallback(
    (node: FileTreeNode) => {
      const path = node.path ?? node.id;
      if (node.isDirectory) {
        files.ensureLoaded(path);
        return;
      }
      setPeek({ status: 'loading', path });
      void files
        .readFile(path)
        .then(result => setPeek({ status: 'loaded', peek: result }))
        .catch(() => setPeek({ status: 'error', path }));
    },
    [files],
  );

  if (!root) return null;

  const nodes = toFileTreeNodes(files.cache);
  const isEmptyRoot = !files.rootLoading && !files.error && nodes.length === 0;
  const selectedId = peek?.status === 'loaded' ? peek.peek.path : peek?.path;

  return (
    <div className="border-surface-200 bg-surface-50 dark:border-dark-surface-600 dark:bg-dark-surface-100 flex h-full w-64 min-w-0 shrink-0 flex-col border-r sm:w-72">
      <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2">
        <span className="text-text dark:text-dark-text truncate text-sm font-medium">{t('workspace.panel_title')}</span>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('workspace.refresh')}
            onClick={() => files.refresh()}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
          <Button type="button" variant="ghost" size="icon" aria-label={t('workspace.collapse')} onClick={onClose}>
            <PanelLeftClose className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

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
            selectedId={selectedId}
            onNodeSelect={handleNodeSelect}
          />
        )}
      </div>

      {peek ? (
        <div className={cn('border-surface-200 dark:border-dark-surface-600 shrink-0 border-t p-2', 'max-h-64 overflow-y-auto')}>
          {peek.status === 'loading' ? (
            <span className="text-text-muted dark:text-dark-text-muted block px-1 py-1 text-xs">{t('workspace.loading')}</span>
          ) : peek.status === 'error' ? (
            <InlineNotice variant="error">{t('workspace.peek_error')}</InlineNotice>
          ) : peek.peek.isBinary ? (
            <InlineNotice variant="info">{t('workspace.binary_file')}</InlineNotice>
          ) : (
            <InlineAttachmentRenderer
              attachment={{ kind: 'file_view', path: peek.peek.path, text: peek.peek.text }}
              labels={{ line: t('workspace.line'), lines: t('workspace.lines') }}
            />
          )}
        </div>
      ) : null}
    </div>
  );
}

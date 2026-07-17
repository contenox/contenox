/**
 * i18n keys referenced in this file (namespace `workspace`; see i18n.ts):
 *   workspace.loading      = "Loading…"
 *   workspace.binary_file  = "Binary file — cannot preview"
 *   workspace.peek_error   = "Could not open file"
 *   workspace.line         = "line"
 *   workspace.lines        = "lines"
 */
import { CodeBlock, InlineNotice } from '@contenox/ui';
import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { WorkspaceFilePeek } from '../../../hooks/useWorkspaceFiles';

export interface FileViewTabProps {
  /** The workspace-relative path this read-only view renders. */
  path: string;
  /** Reads a file's content — the shared `useWorkspaceFiles().readFile`. */
  readFile: (path: string) => Promise<WorkspaceFilePeek>;
}

type ViewState = { status: 'loading' } | { status: 'loaded'; peek: WorkspaceFilePeek } | { status: 'error' };

/**
 * A read-only file view hosted as a CANVAS tab (successor to the old workspace
 * sidebar preview): fetches `path`'s content once via the shared `readFile` and
 * renders it directly, expanded, in a scrollable code block. Because a file tab
 * holds exactly one file and the tab strip already names it, there is NO
 * collapsible attachment-card chrome here — just a small line-count header line
 * over the content. The canvas tab strip supplies the label and ✕.
 */
export function FileViewTab({ path, readFile }: FileViewTabProps) {
  const { t } = useTranslation();
  const [state, setState] = useState<ViewState>({ status: 'loading' });

  useEffect(() => {
    let cancelled = false;
    setState({ status: 'loading' });
    void readFile(path)
      .then(peek => {
        if (!cancelled) setState({ status: 'loaded', peek });
      })
      .catch(() => {
        if (!cancelled) setState({ status: 'error' });
      });
    return () => {
      cancelled = true;
    };
  }, [path, readFile]);

  if (state.status === 'loading') {
    return <span className="text-text-muted dark:text-dark-text-muted block px-1 py-1 text-xs">{t('workspace.loading')}</span>;
  }
  if (state.status === 'error') {
    return <InlineNotice variant="error">{t('workspace.peek_error')}</InlineNotice>;
  }
  if (state.peek.isBinary) {
    return <InlineNotice variant="info">{t('workspace.binary_file')}</InlineNotice>;
  }
  const lineCount = state.peek.text.split('\n').length;
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="text-text-muted dark:text-dark-text-muted shrink-0 px-1 pb-1 text-[11px]">
        {lineCount} {lineCount === 1 ? t('workspace.line') : t('workspace.lines')}
      </div>
      <CodeBlock className="min-h-0 flex-1 px-1">{state.peek.text}</CodeBlock>
    </div>
  );
}

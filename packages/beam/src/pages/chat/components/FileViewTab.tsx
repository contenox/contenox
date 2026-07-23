/**
 * i18n keys referenced in this file (namespace `workspace`; see i18n.ts):
 *   workspace.loading      = "Loading…"
 *   workspace.binary_file  = "Binary file — cannot preview"
 *   workspace.peek_error   = "Could not open file"
 *   workspace.line         = "line"
 *   workspace.lines        = "lines"
 */
import { InlineNotice } from '@contenox/ui';
import { lazy, Suspense, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { WorkspaceFilePeek } from '../../../hooks/useWorkspaceFiles';

// The shared read-only Monaco viewer, kept ONLY behind this lazy boundary so
// Monaco never enters the main bundle and only loads once a file tab is actually
// opened (the instant-feel law); until it arrives, the "loading" span stands in
// as the fallback. Same component the mission diff's old/new panes use.
const MonacoView = lazy(() => import('../../../components/editors/MonacoView'));

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
 * renders it in a read-only Monaco editor (lazily loaded, so Monaco stays out of
 * the main bundle). Because a file tab holds exactly one file and the tab strip
 * already names it, there is NO collapsible attachment-card chrome here — just a
 * small line-count header line over the content. The canvas tab strip supplies
 * the label and ✕.
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
      <Suspense
        fallback={
          <span className="text-text-muted dark:text-dark-text-muted block px-1 py-1 text-xs">{t('workspace.loading')}</span>
        }>
        <MonacoView path={path} value={state.peek.text} />
      </Suspense>
    </div>
  );
}

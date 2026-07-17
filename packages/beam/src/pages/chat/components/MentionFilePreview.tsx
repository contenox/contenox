/**
 * i18n keys referenced in this file (namespace `workspace`):
 *   workspace.mention_preview_label = "File preview"
 *   workspace.mention_loading, workspace.binary_file, workspace.peek_error
 *   workspace.line, workspace.lines
 */
import { CodeBlock, InlineNotice } from '@contenox/ui';
import { FileText } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { FilePreviewState } from '../lib/filePreview';

export interface MentionFilePreviewProps {
  state: FilePreviewState;
}

/**
 * Live content preview shown as the TOP section INSIDE the `@`-mention browser
 * popover while it is open and a FILE is highlighted (see `useFilePreview` /
 * `mentionPreviewPath`). Rendered inside the absolutely-positioned popover
 * overlay (`MentionMenu`), NOT in the composer's normal flow — so navigating
 * files changes its content without ever reflowing the transcript/composer.
 * Reuses the shared `CodeBlock` file-view renderer for a real monospace,
 * scrollable body; the box is bounded (`max-h`) with its own `overflow-auto`
 * and a bottom-border separator to the file list below. Pure presentation.
 */
export function MentionFilePreview({ state }: MentionFilePreviewProps) {
  const { t } = useTranslation();
  if (state.status === 'hidden') return null;

  const path = state.path;

  return (
    <div
      role="region"
      aria-label={t('workspace.mention_preview_label')}
      className="border-surface-200 dark:border-dark-surface-600 flex max-h-[40vh] min-h-0 shrink-0 flex-col overflow-hidden border-b">
      <div className="border-surface-200 dark:border-dark-surface-600 text-text-muted dark:text-dark-text-muted flex shrink-0 items-center gap-1.5 border-b px-3 py-1.5 text-xs">
        <FileText className="h-3.5 w-3.5 shrink-0" aria-hidden />
        <span className="min-w-0 flex-1 truncate font-mono">{path}</span>
        {state.status === 'text' && (
          <span className="shrink-0 text-[10px]">
            {lineCount(state.text)} {lineCount(state.text) === 1 ? t('workspace.line') : t('workspace.lines')}
          </span>
        )}
      </div>

      {state.status === 'loading' ? (
        <div className="text-text-muted dark:text-dark-text-muted px-3 py-2 text-xs">{t('workspace.mention_loading')}</div>
      ) : state.status === 'binary' ? (
        <div className="p-2">
          <InlineNotice variant="info">{t('workspace.binary_file')}</InlineNotice>
        </div>
      ) : state.status === 'error' ? (
        <div className="p-2">
          <InlineNotice variant="error">{t('workspace.peek_error')}</InlineNotice>
        </div>
      ) : (
        <div className="min-h-0 flex-1 overflow-auto">
          <CodeBlock className="px-3 py-2">{state.text}</CodeBlock>
        </div>
      )}
    </div>
  );
}

function lineCount(text: string): number {
  return text.split('\n').length;
}

import { DiffEditor, Editor } from '@monaco-editor/react';
import { Button, InlineNotice, Span } from '@contenox/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useMediaQuery } from '../../../hooks/useMediaQuery';
import type { TranslationKey } from '../../../i18n';
import { defineBeamMonacoThemes, useMonacoAppTheme } from '../../../lib/monacoAppTheme';
import { languageForPath } from '../../../lib/missionChanges';
import type { MissionChangeStatus, MissionFileDiff } from '../../../lib/types';

/**
 * The per-file diff, rendered with Monaco (`@monaco-editor/react`) — the proven
 * choice for exactly this (OpenHands), never a hand-rolled differ. This is the
 * ONLY module that imports Monaco in the mission inspector, and it is lazily
 * loaded from MissionChangesTab (itself lazily loaded from the page), so Monaco
 * never enters the mission-detail bundle and does not even load until a file row
 * is expanded — the diff is asynchronous enhancement over the instant list (the
 * Sublime-nature law). It is read-only: Beam reviews, it never edits (the
 * meaningful-filter).
 */

type DiffView = 'old' | 'diff' | 'new';

const READONLY_OPTIONS = {
  readOnly: true,
  automaticLayout: true,
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
  fontFamily: '"Geist Mono", ui-monospace, SFMono-Regular, Menlo, monospace',
  renderWhitespace: 'selection',
  padding: { top: 8, bottom: 8 },
} as const;

export default function MonacoDiffView({
  path,
  status,
  diff,
}: {
  path: string;
  status: MissionChangeStatus;
  diff: MissionFileDiff;
}) {
  const { t } = useTranslation();
  const [view, setView] = useState<DiffView>('diff');
  const theme = useMonacoAppTheme();
  const language = languageForPath(path);
  // Side-by-side reads on a wide viewport; inline is the honest fit for the
  // narrow mission-inspector column and mobile.
  const sideBySide = useMediaQuery('(min-width: 768px)');

  const views: { id: DiffView; labelKey: TranslationKey; titleKey: TranslationKey }[] = [
    { id: 'old', labelKey: 'changes.view_old', titleKey: 'changes.view_old_title' },
    { id: 'diff', labelKey: 'changes.view_diff', titleKey: 'changes.view_diff_title' },
    { id: 'new', labelKey: 'changes.view_new', titleKey: 'changes.view_new_title' },
  ];

  // An added file has no original; a deleted file has no resulting version —
  // say so plainly instead of showing a blank editor.
  const emptyNote =
    view === 'old' && status === 'added'
      ? t('changes.added_no_old')
      : view === 'new' && status === 'deleted'
        ? t('changes.deleted_no_new')
        : undefined;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="inline-flex gap-1" role="group">
          {views.map(v => (
            <Button
              key={v.id}
              type="button"
              size="xs"
              variant={view === v.id ? 'primary' : 'outline'}
              title={t(v.titleKey)}
              onClick={() => setView(v.id)}>
              {t(v.labelKey)}
            </Button>
          ))}
        </div>
        {diff.truncated && (
          <Span variant="muted" className="text-xs" title={t('changes.diff_truncated')}>
            {t('changes.diff_truncated')}
          </Span>
        )}
      </div>

      {emptyNote ? (
        <InlineNotice variant="info">{emptyNote}</InlineNotice>
      ) : (
        <div className="border-surface-200 dark:border-dark-surface-600 h-[50vh] min-h-[18rem] overflow-hidden rounded-lg border">
          {view === 'diff' ? (
            <DiffEditor
              height="100%"
              original={diff.original}
              modified={diff.modified}
              language={language}
              theme={theme}
              beforeMount={defineBeamMonacoThemes}
              options={{ ...READONLY_OPTIONS, renderSideBySide: sideBySide }}
            />
          ) : (
            <Editor
              height="100%"
              value={view === 'old' ? diff.original : diff.modified}
              language={language}
              theme={theme}
              beforeMount={defineBeamMonacoThemes}
              options={READONLY_OPTIONS}
            />
          )}
        </div>
      )}
    </div>
  );
}

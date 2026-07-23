import { Editor } from '@monaco-editor/react';
import type { editor } from 'monaco-editor';
import { defineBeamMonacoThemes, useMonacoAppTheme } from '../../lib/monacoAppTheme';
import { languageForPath } from '../../lib/missionChanges';

/**
 * The ONE read-only Monaco viewer for Beam — the shared component behind the chat
 * canvas's file tab (FileViewTab) and the mission inspector's old/new diff panes
 * (MonacoDiffView). Both previously carried their own byte-identical
 * `READONLY_OPTIONS` and theme wiring; this collapses that into a single seam so
 * a viewer tweak (font, padding, whitespace) lands everywhere at once.
 *
 * Read-only by contract: Beam reviews, it never edits (the meaningful-filter).
 * Editing surfaces (ChainJsonEditor, PolicyEditor) are a separate concern and do
 * NOT use this. Monaco stays out of the main bundle: this module is only ever
 * reached through a lazy boundary (FileViewTab's `lazy(() => import(...))` and the
 * lazily-loaded MonacoDiffView), so it and `monaco-editor` load on demand.
 */

/** The shared read-only editor options — the single definition every read-only
 * Monaco surface uses (also spread by MonacoDiffView's DiffEditor). */
export const MONACO_READONLY_OPTIONS: editor.IStandaloneEditorConstructionOptions = {
  readOnly: true,
  automaticLayout: true,
  minimap: { enabled: false },
  scrollBeyondLastLine: false,
  fontFamily: '"Geist Mono", ui-monospace, SFMono-Regular, Menlo, monospace',
  renderWhitespace: 'selection',
  padding: { top: 8, bottom: 8 },
};

export interface MonacoViewProps {
  /** The text to render. */
  value: string;
  /** Explicit Monaco language id; when omitted it is derived from `path`. */
  language?: string;
  /** Source path — used to derive the language when `language` is not given, and
   * so the same file re-renders with stable syntax colouring. */
  path?: string;
  /** Wrapper classes. The parent owns sizing; the editor fills it (height 100%).
   * Defaults to filling a flex column (`min-h-0 flex-1 overflow-hidden`). */
  className?: string;
  /** Option overrides merged onto {@link MONACO_READONLY_OPTIONS} (e.g. a diff
   * pane that wants a different padding). */
  options?: editor.IStandaloneEditorConstructionOptions;
}

export default function MonacoView({ value, language, path, className, options }: MonacoViewProps) {
  const theme = useMonacoAppTheme();
  const resolvedLanguage = language ?? (path ? languageForPath(path) : undefined);
  return (
    <div className={className ?? 'min-h-0 flex-1 overflow-hidden'}>
      <Editor
        height="100%"
        value={value}
        language={resolvedLanguage}
        theme={theme}
        beforeMount={defineBeamMonacoThemes}
        options={options ? { ...MONACO_READONLY_OPTIONS, ...options } : MONACO_READONLY_OPTIONS}
      />
    </div>
  );
}

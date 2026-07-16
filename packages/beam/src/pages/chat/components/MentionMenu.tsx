/**
 * i18n keys referenced in this file (namespace `workspace`; add to i18n.ts):
 *   workspace.mention_menu_label = "Workspace files"
 */
import { cn } from '@contenox/ui';
import { ChevronRight, File as FileIcon, Folder } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import {
  applyMention,
  applyMentionAscend,
  applyMentionFolder,
  deriveMentionQuery,
  mentionCandidatesForScope,
  mentionMenuKeyFromEvent,
  splitMentionQuery,
  type MentionCandidate,
  type WorkspaceFileRef,
} from '../lib/mentions';
import type { DirCache } from '../lib/workspaceTree';
import { clampIndex } from '../lib/slashMenuState';

/** Candidates shown per scope are capped for render/perf; the leaf filter narrows this in practice. */
const MAX_ENTRIES = 50;

export interface UseMentionMenuArgs {
  draft: string;
  /** Current caret index in the composer textarea. */
  caret: number;
  /** Per-directory listing cache from `useWorkspaceFiles` (root keyed by `''`). */
  cache: DirCache;
  /** Lazily loads a directory's children when the menu browses into it. */
  ensureLoaded: (dir: string) => void;
  /** Whether a directory's listing is still loading. */
  isLoading: (dir: string) => boolean;
  /** Inserts a picked FILE as a mention: new draft, caret after the token, the file. */
  onInsert: (nextDraft: string, nextCaret: number, file: WorkspaceFileRef) => void;
  /** Drills into a picked FOLDER: new draft (`@dir/`) and caret; the menu keeps browsing. */
  onDrill: (nextDraft: string, nextCaret: number) => void;
}

export interface UseMentionMenuResult {
  open: boolean;
  entries: MentionCandidate[];
  /** The directory currently being browsed (root is `''`), shown as a breadcrumb. */
  scope: string;
  /** True when the browsed directory's listing has not arrived yet. */
  loading: boolean;
  activeIndex: number;
  pick: (entry: MentionCandidate) => void;
  setActiveIndex: (i: number) => void;
  handleKeyDown: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
}

const EMPTY_ENTRIES: MentionCandidate[] = [];

/**
 * Drives the composer's `@`-mention popover as a scoped file BROWSER: the
 * `@query` is read as a path, split into a directory scope and a leaf filter
 * (`lib/mentions.ts`). Files in scope insert as `resource_link` mentions;
 * directories are navigable — picking one rewrites the draft to `@dir/` and
 * the menu re-scopes into it, lazily loading its children. This is how a deep
 * file is reached without pre-expanding the side panel. Keydown mapping (arrow
 * nav/accept/close) is shared with the slash menu (`lib/slashMenuState.ts`).
 */
export function useMentionMenu({ draft, caret, cache, ensureLoaded, isLoading, onInsert, onDrill }: UseMentionMenuArgs): UseMentionMenuResult {
  const query = useMemo(() => deriveMentionQuery(draft, caret), [draft, caret]);
  const { dir: scope, leaf } = useMemo(() => splitMentionQuery(query.query), [query.query]);

  // Ask the data hook for the browsed directory's children as soon as the
  // scope points at one we haven't listed yet (idempotent in the hook).
  useEffect(() => {
    if (query.active) ensureLoaded(scope);
  }, [query.active, scope, ensureLoaded]);

  const scopeEntries = useMemo<MentionCandidate[]>(() => {
    const entries = cache[scope];
    if (!entries) return EMPTY_ENTRIES;
    return entries.map(e => ({ path: e.path, name: e.name, isDirectory: e.isDirectory }));
  }, [cache, scope]);

  const filtered = useMemo(() => mentionCandidatesForScope(scopeEntries, leaf).slice(0, MAX_ENTRIES), [scopeEntries, leaf]);
  const loading = query.active && cache[scope] === undefined && isLoading(scope);

  // Escape closes the popover for the CURRENT draft text without touching the
  // draft itself; any draft change re-derives the query fresh, so the
  // dismissal doesn't stick past the keystroke that caused it.
  const [dismissedFor, setDismissedFor] = useState<string | null>(null);
  const open = query.active && dismissedFor !== draft && (filtered.length > 0 || loading);

  const [activeIndex, setActiveIndexState] = useState(0);
  useEffect(() => {
    setActiveIndexState(i => clampIndex(i, filtered.length));
  }, [filtered.length]);

  const setActiveIndex = useCallback((index: number) => setActiveIndexState(clampIndex(index, filtered.length)), [filtered.length]);

  const pick = useCallback(
    (entry: MentionCandidate) => {
      const q = deriveMentionQuery(draft, caret);
      if (entry.isDirectory) {
        const { text, caret: nextCaret } = applyMentionFolder(draft, q, entry);
        onDrill(text, nextCaret);
        setActiveIndexState(0);
        setDismissedFor(null);
        return;
      }
      const { text, caret: nextCaret } = applyMention(draft, q, entry);
      onInsert(text, nextCaret, { path: entry.path, name: entry.name });
      setDismissedFor(null);
    },
    [draft, caret, onInsert, onDrill],
  );

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (!open) return;
      const action = mentionMenuKeyFromEvent(e.key);
      if (!action) return;
      // Consume (preventDefault) only when the key drives a menu action, so
      // →/← still move the text caret when there is no folder to drill into
      // or no parent to ascend to.
      switch (action.type) {
        case 'move':
          e.preventDefault();
          setActiveIndexState(i => clampIndex(i + action.delta, filtered.length));
          break;
        case 'accept': {
          e.preventDefault();
          const entry = filtered[activeIndex];
          if (entry) pick(entry);
          break;
        }
        case 'drill': {
          const entry = filtered[activeIndex];
          if (entry?.isDirectory) {
            e.preventDefault();
            pick(entry); // folders drill via pick()
          }
          break;
        }
        case 'ascend':
          if (scope !== '') {
            e.preventDefault();
            const { text, caret: nextCaret } = applyMentionAscend(draft, query);
            onDrill(text, nextCaret);
            setActiveIndexState(0);
          }
          break;
        case 'close':
          e.preventDefault();
          setDismissedFor(draft);
          break;
      }
    },
    [open, filtered, activeIndex, pick, draft, query, scope, onDrill],
  );

  return { open, entries: filtered, scope, loading, activeIndex, pick, setActiveIndex, handleKeyDown };
}

export interface MentionMenuProps {
  entries: MentionCandidate[];
  /** The directory being browsed (root is `''`) — shown as a breadcrumb header. */
  scope: string;
  /** Listing not yet loaded for the browsed directory. */
  loading: boolean;
  activeIndex: number;
  onPick: (entry: MentionCandidate) => void;
  onHoverIndex?: (index: number) => void;
}

/**
 * The `@`-mention browser popover — pure presentation, anchored by the caller
 * (`position: relative` wrapper) above the composer. Folders carry a folder
 * glyph and a drill chevron; files carry a file glyph. A breadcrumb header
 * shows the current directory when browsing below the root.
 */
export function MentionMenu({ entries, scope, loading, activeIndex, onPick, onHoverIndex }: MentionMenuProps) {
  const { t } = useTranslation();

  return (
    <div
      role="listbox"
      aria-label={t('workspace.mention_menu_label')}
      className="border-surface-200 bg-surface-50 dark:border-dark-surface-600 dark:bg-dark-surface-100 absolute bottom-full left-0 z-20 mb-2 max-h-64 w-full overflow-auto rounded-lg border shadow-lg sm:w-96"
    >
      {scope !== '' && (
        <div className="border-surface-200 dark:border-dark-surface-600 text-text-muted dark:text-dark-text-muted sticky top-0 border-b bg-inherit px-3 py-1.5 font-mono text-xs">
          {scope}/
        </div>
      )}
      {loading && entries.length === 0 ? (
        <div className="text-text-muted dark:text-dark-text-muted px-3 py-2 text-xs">{t('workspace.mention_loading')}</div>
      ) : (
        entries.map((entry, i) => (
          <button
            key={entry.path}
            type="button"
            role="option"
            aria-selected={i === activeIndex}
            onMouseEnter={() => onHoverIndex?.(i)}
            // mousedown (not click) + preventDefault so the textarea never
            // blurs before the pick registers.
            onMouseDown={e => {
              e.preventDefault();
              onPick(entry);
            }}
            className={cn(
              'flex w-full items-center gap-2 px-3 py-2 text-left text-sm',
              i === activeIndex ? 'bg-primary-50 dark:bg-dark-primary-900' : 'hover:bg-surface-100 dark:hover:bg-dark-surface-300',
            )}
          >
            {entry.isDirectory ? (
              <Folder className="text-primary-500 dark:text-dark-primary h-3.5 w-3.5 shrink-0" aria-hidden />
            ) : (
              <FileIcon className="text-text-muted dark:text-dark-text-muted h-3.5 w-3.5 shrink-0" aria-hidden />
            )}
            <span className="text-text dark:text-dark-text min-w-0 flex-1 truncate font-mono text-xs font-medium">
              {entry.name}
              {entry.isDirectory && '/'}
            </span>
            {entry.isDirectory && <ChevronRight className="text-text-muted dark:text-dark-text-muted h-3.5 w-3.5 shrink-0" aria-hidden />}
          </button>
        ))
      )}
    </div>
  );
}

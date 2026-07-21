import { Badge } from '@contenox/ui';
import { Search } from 'lucide-react';
import { Fragment, useEffect, useRef, type KeyboardEvent, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { TranslationKey } from '../../i18n';
import type { RankedItem } from '../../lib/palette/paletteState';
import type { PaletteItem, PaletteItemType } from '../../lib/palette/types';
import { cn } from '../../lib/utils';

/**
 * The palette's presentation: a top-anchored modal on desktop, a full-screen
 * sheet on mobile. Deliberately prop-driven and free of the open/query/frecency
 * state (that lives in {@link CommandPalette} and the pure reducer), so the
 * layout, highlighting, type badges, and empty state can be pinned with a static
 * render. A flat, ranked, badge-tagged list is used rather than type-grouped
 * sections: grouping would fight the cross-type fuzzy+frecency ranking, and the
 * house already reads flat rows with trailing badges everywhere.
 */

const TYPE_LABEL_KEY: Record<PaletteItemType, TranslationKey> = {
  action: 'palette.type_action',
  mission: 'palette.type_mission',
  agent: 'palette.type_agent',
  session: 'palette.type_session',
  fleet: 'palette.type_fleet',
  workspace: 'palette.type_workspace',
  inbox: 'palette.type_inbox',
};

const TYPE_BADGE_VARIANT: Record<PaletteItemType, 'primary' | 'accent' | 'warning' | 'outline'> = {
  action: 'outline',
  mission: 'primary',
  agent: 'accent',
  session: 'outline',
  fleet: 'primary',
  workspace: 'outline',
  inbox: 'warning',
};

export interface PaletteOverlayProps {
  query: string;
  results: RankedItem[];
  selected: number;
  onQueryChange: (query: string) => void;
  onArrow: (delta: number) => void;
  onEnter: () => void;
  onEscape: () => void;
  onHover: (index: number) => void;
  onExecute: (item: PaletteItem) => void;
  onClose: () => void;
}

export function PaletteOverlay(props: PaletteOverlayProps) {
  const { query, results, selected, onQueryChange, onArrow, onEnter, onEscape, onHover, onExecute, onClose } =
    props;
  const { t } = useTranslation();
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, []);

  // Keep the highlighted row visible as the selection moves with the keyboard.
  useEffect(() => {
    const el = listRef.current?.querySelector<HTMLElement>(`#palette-opt-${selected}`);
    el?.scrollIntoView({ block: 'nearest' });
  }, [selected]);

  const onInputKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault();
        onArrow(1);
        break;
      case 'ArrowUp':
        e.preventDefault();
        onArrow(-1);
        break;
      case 'Enter':
        e.preventDefault();
        onEnter();
        break;
      case 'Escape':
        e.preventDefault();
        onEscape();
        break;
    }
  };

  return (
    <div
      className="fixed inset-0 z-[60] flex justify-center bg-black/50 backdrop-blur-sm sm:items-start sm:pt-[10vh]"
      role="presentation"
      onClick={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label={t('palette.aria_label')}
        onClick={e => e.stopPropagation()}
        className={cn(
          'border-surface-300 dark:border-dark-surface-700 bg-surface-50 dark:bg-dark-surface-100 flex h-full w-full flex-col overflow-hidden text-inherit shadow-2xl',
          'sm:h-auto sm:max-h-[70vh] sm:w-[36rem] sm:max-w-[90vw] sm:rounded-xl sm:border',
        )}>
        <div className="border-surface-300 dark:border-dark-surface-700 flex shrink-0 items-center gap-2 border-b px-4 py-3">
          <Search className="text-text-muted dark:text-dark-text-muted h-4 w-4 shrink-0" aria-hidden />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={e => onQueryChange(e.target.value)}
            onKeyDown={onInputKeyDown}
            placeholder={t('palette.placeholder')}
            aria-label={t('palette.placeholder')}
            role="combobox"
            aria-expanded="true"
            aria-controls="palette-listbox"
            aria-activedescendant={results.length > 0 ? `palette-opt-${selected}` : undefined}
            autoComplete="off"
            spellCheck={false}
            className="text-text dark:text-dark-text placeholder:text-text-muted min-w-0 flex-1 bg-transparent text-sm outline-none"
          />
          <kbd className="border-surface-300 text-text-muted dark:border-dark-surface-600 hidden rounded border px-1.5 py-0.5 text-[10px] sm:inline-block">
            Esc
          </kbd>
        </div>

        <ul
          ref={listRef}
          id="palette-listbox"
          role="listbox"
          aria-label={t('palette.aria_label')}
          className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2">
          {results.length === 0 ? (
            <li className="text-text-muted dark:text-dark-text-muted px-3 py-8 text-center text-sm">
              {t('palette.empty')}
            </li>
          ) : (
            results.map((row, index) => (
              <li
                key={row.item.id}
                id={`palette-opt-${index}`}
                role="option"
                aria-selected={index === selected}
                onMouseMove={() => onHover(index)}
                onClick={() => onExecute(row.item)}
                className={cn(
                  'flex cursor-pointer items-center gap-3 rounded-lg px-3 py-2',
                  index === selected
                    ? 'bg-surface-200 dark:bg-dark-surface-200'
                    : 'hover:bg-surface-100 dark:hover:bg-dark-surface-200',
                )}>
                <span className="text-text-muted dark:text-dark-text-muted shrink-0">{row.item.icon}</span>
                <div className="min-w-0 flex-1">
                  <div className="text-text dark:text-dark-text truncate text-sm">
                    <Highlighted text={row.item.title} indexes={row.matchIndexes} />
                  </div>
                  {row.item.subtitle && (
                    <div className="text-text-muted dark:text-dark-text-muted truncate text-xs">
                      {row.item.subtitle}
                    </div>
                  )}
                </div>
                <Badge size="sm" variant={TYPE_BADGE_VARIANT[row.item.type]}>
                  {t(TYPE_LABEL_KEY[row.item.type])}
                </Badge>
              </li>
            ))
          )}
        </ul>

        <div className="border-surface-300 text-text-muted dark:border-dark-surface-700 hidden shrink-0 items-center gap-4 border-t px-4 py-2 text-[11px] sm:flex">
          <span>{t('palette.hint_navigate')}</span>
          <span>{t('palette.hint_open')}</span>
          <span>{t('palette.hint_close')}</span>
        </div>
      </div>
    </div>
  );
}

/** Renders `text` with the matched character positions emphasized. */
export function Highlighted({ text, indexes }: { text: string; indexes: number[] }): ReactNode {
  if (indexes.length === 0) return text;
  const set = new Set(indexes);
  const segments: { text: string; hi: boolean }[] = [];
  for (let i = 0; i < text.length; i++) {
    const hi = set.has(i);
    const last = segments[segments.length - 1];
    if (last && last.hi === hi) last.text += text[i];
    else segments.push({ text: text[i], hi });
  }
  return (
    <>
      {segments.map((seg, i) =>
        seg.hi ? (
          <mark
            key={i}
            className="text-primary-600 dark:text-dark-primary-400 bg-transparent font-semibold">
            {seg.text}
          </mark>
        ) : (
          <Fragment key={i}>{seg.text}</Fragment>
        ),
      )}
    </>
  );
}

import { cn } from '@contenox/ui';
import { useCallback, useEffect, useMemo, useState, type KeyboardEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { AvailableCommand } from '../../../lib/acp';
import { useSlashCommandRegistry, useSlashCommands } from '../../../lib/slashCommands';
import { clampIndex, deriveSlashMenuQuery, filterSlashCommands, slashMenuKeyFromEvent, type SlashMenuEntry } from '../lib/slashMenuState';

export interface UseSlashCommandMenuArgs {
  draft: string;
  onDraftChange: (value: string) => void;
  availableCommands: AvailableCommand[];
}

export interface UseSlashCommandMenuResult {
  open: boolean;
  entries: SlashMenuEntry[];
  activeIndex: number;
  pick: (name: string) => void;
  setActiveIndex: (index: number) => void;
  handleKeyDown: (e: KeyboardEvent<HTMLTextAreaElement>) => void;
}

/**
 * Registers the agent's `session.availableCommands` into the shared
 * `SlashCommandRegistry` (must be called under `SlashCommandRegistryProvider`)
 * — agent commands are authoritative; this page adds no client-side entries
 * of its own — then derives the popover's open/filter/selection state from
 * the composer draft via the pure helpers in `lib/slashMenuState.ts`.
 */
export function useSlashCommandMenu({
  draft,
  onDraftChange,
  availableCommands,
}: UseSlashCommandMenuArgs): UseSlashCommandMenuResult {
  const registry = useSlashCommandRegistry();

  useEffect(() => {
    const unregisters = availableCommands.map(cmd =>
      registry.register({
        trigger: '/',
        name: cmd.name,
        description: cmd.description,
        usage: cmd.input?.hint ? `/${cmd.name} ${cmd.input.hint}` : `/${cmd.name}`,
        // The composer submits slash-command text through sendPrompt verbatim
        // (acpsvc/commandrunner.go parses it server-side) — accepting a menu
        // entry only fills the composer; it never executes anything here.
        execute: () => undefined,
      }),
    );
    return () => unregisters.forEach(un => un());
  }, [registry, availableCommands]);

  const registered = useSlashCommands();
  const entries = useMemo<SlashMenuEntry[]>(
    () =>
      registered
        .filter(c => (c.trigger ?? '/') === '/')
        .map(c => ({ name: c.name, description: c.description, usage: c.usage })),
    [registered],
  );

  const { open: derivedOpen, query } = deriveSlashMenuQuery(draft);
  const filtered = useMemo(() => filterSlashCommands(entries, query), [entries, query]);

  // Escape closes the popover for the CURRENT draft text without touching the
  // draft itself; typing anything (draft changes) re-derives `open` fresh, so
  // the dismissal doesn't stick past the keystroke that caused it.
  const [dismissedFor, setDismissedFor] = useState<string | null>(null);
  const open = derivedOpen && filtered.length > 0 && dismissedFor !== draft;

  const [activeIndex, setActiveIndexState] = useState(0);
  useEffect(() => {
    setActiveIndexState(i => clampIndex(i, filtered.length));
  }, [filtered.length]);

  const setActiveIndex = useCallback((index: number) => setActiveIndexState(clampIndex(index, filtered.length)), [filtered.length]);

  const pick = useCallback(
    (name: string) => {
      onDraftChange(`/${name} `);
      setDismissedFor(null);
    },
    [onDraftChange],
  );

  const handleKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (!open) return;
      const action = slashMenuKeyFromEvent(e.key);
      if (!action) return;
      e.preventDefault();
      if (action.type === 'move') {
        setActiveIndexState(i => clampIndex(i + action.delta, filtered.length));
      } else if (action.type === 'accept') {
        const entry = filtered[activeIndex];
        if (entry) pick(entry.name);
      } else {
        setDismissedFor(draft);
      }
    },
    [open, filtered, activeIndex, pick, draft],
  );

  return { open, entries: filtered, activeIndex, pick, setActiveIndex, handleKeyDown };
}

export interface SlashCommandMenuProps {
  entries: SlashMenuEntry[];
  activeIndex: number;
  onPick: (name: string) => void;
  onHoverIndex?: (index: number) => void;
}

/** The completion popover itself — pure presentation, anchored by the caller (`position: relative` wrapper) above the composer. */
export function SlashCommandMenu({ entries, activeIndex, onPick, onHoverIndex }: SlashCommandMenuProps) {
  const { t } = useTranslation();
  if (entries.length === 0) return null;

  return (
    <div
      role="listbox"
      aria-label={t('acp_chat.slash_menu_label')}
      className="border-surface-200 bg-surface-50 dark:border-dark-surface-600 dark:bg-dark-surface-100 absolute bottom-full left-0 z-20 mb-2 max-h-64 w-full overflow-auto rounded-lg border shadow-lg sm:w-96"
    >
      {entries.map((entry, i) => (
        <button
          key={entry.name}
          type="button"
          role="option"
          aria-selected={i === activeIndex}
          onMouseEnter={() => onHoverIndex?.(i)}
          // mousedown (not click) + preventDefault so the textarea never
          // blurs before the pick registers.
          onMouseDown={e => {
            e.preventDefault();
            onPick(entry.name);
          }}
          className={cn(
            'flex w-full flex-col items-start gap-0.5 px-3 py-2 text-left text-sm',
            i === activeIndex
              ? 'bg-primary-50 dark:bg-dark-primary-900'
              : 'hover:bg-surface-100 dark:hover:bg-dark-surface-300',
          )}
        >
          <span className="text-text dark:text-dark-text font-mono text-xs font-medium">/{entry.name}</span>
          <span className="text-text-muted dark:text-dark-text-muted text-xs">{entry.description}</span>
        </button>
      ))}
    </div>
  );
}

import { useCallback, useEffect, useMemo, useReducer, useState } from 'react';
import { createPortal } from 'react-dom';
import { loadFrecency, recordUsage, saveFrecency, type FrecencyStore } from '../../lib/palette/frecency';
import {
  computeResults,
  createPaletteSearcher,
  INITIAL_PALETTE_STATE,
  paletteReducer,
} from '../../lib/palette/paletteState';
import type { PaletteItem } from '../../lib/palette/types';
import { PaletteOverlay } from './PaletteOverlay';
import { usePaletteSources } from './usePaletteSources';

/**
 * The goto-anything command palette (Tier 1½): Cmd/Ctrl+K from anywhere in the
 * app opens a fuzzy-select over the runtime's live objects and actions —
 * missions, fleet instances, agents, sessions, workspace roots, pending asks,
 * and page navigation — keyboard-first and instant. Filtering is synchronous per
 * keystroke over the full in-memory set (no async search, no index warmup, no
 * spinner); frecency silently lifts familiar results.
 *
 * Mounted once, app-wide, for authenticated users (see App.tsx). While closed it
 * is inert: the source set is only assembled when open, and the global key
 * listener does nothing but watch for the toggle chord.
 */
export function CommandPalette() {
  const [state, dispatch] = useReducer(paletteReducer, INITIAL_PALETTE_STATE);
  const [frecency, setFrecency] = useState<FrecencyStore>(() => loadFrecency());

  const items = usePaletteSources(state.open);
  const searcher = useMemo(() => createPaletteSearcher(items), [items]);
  const results = useMemo(
    () => computeResults(searcher, items, state.query, frecency, Date.now()),
    [searcher, items, state.query, frecency],
  );

  // Global toggle. Cmd+K (mac) / Ctrl+K (win/linux) — verified free of any
  // existing binding; only Ctrl/Cmd+S is bound elsewhere (chain/policy save).
  // preventDefault stops the browser's own Cmd/Ctrl+K (address-bar search).
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === 'k' && !e.isComposing) {
        e.preventDefault();
        dispatch({ type: 'toggle' });
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  // Lock body scroll behind the modal while open.
  useEffect(() => {
    if (!state.open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previous;
    };
  }, [state.open]);

  const execute = useCallback(
    (item: PaletteItem) => {
      const next = recordUsage(frecency, item.id, Date.now());
      setFrecency(next);
      saveFrecency(next);
      dispatch({ type: 'close' });
      item.action();
    },
    [frecency],
  );

  const onEnter = useCallback(() => {
    const item = results[state.selected]?.item;
    if (item) execute(item);
  }, [results, state.selected, execute]);

  if (!state.open) return null;

  return createPortal(
    <PaletteOverlay
      query={state.query}
      results={results}
      selected={state.selected}
      onQueryChange={query => dispatch({ type: 'setQuery', query })}
      onArrow={delta => dispatch({ type: 'move', delta, count: results.length })}
      onEnter={onEnter}
      onEscape={() => dispatch({ type: 'close' })}
      onHover={index => dispatch({ type: 'setSelected', index, count: results.length })}
      onExecute={execute}
      onClose={() => dispatch({ type: 'close' })}
    />,
    document.body,
  );
}

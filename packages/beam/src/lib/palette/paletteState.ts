import { blend, frecencyBoost, type FrecencyStore } from './frecency';
import { createSearcher, type FuzzySearcher, type WeightedField } from './fuzzy';
import type { PaletteItem, PaletteItemType } from './types';

/**
 * The palette's pure core: how items become a matched field set, how a query +
 * frecency become a ranked result list, and the keyboard state machine. All of
 * it is synchronous and side-effect-free so the Sublime-nature latency budget
 * (filter on every keystroke over the full in-memory set, no async, no spinner)
 * is met by construction and is unit-testable without a DOM.
 */

/** Field weights: the title leads, then hidden keywords (names/aliases), then subtitle, then the id. */
export const FIELD_WEIGHTS = {
  title: 4,
  keyword: 3,
  subtitle: 2,
  id: 1,
} as const;

/** Maps a PaletteItem to its weighted searchable fields; only the title feeds highlighting. */
export function paletteItemFields(item: PaletteItem): WeightedField[] {
  const fields: WeightedField[] = [{ text: item.title, weight: FIELD_WEIGHTS.title, highlight: true }];
  for (const kw of item.keywords ?? []) fields.push({ text: kw, weight: FIELD_WEIGHTS.keyword });
  if (item.subtitle) fields.push({ text: item.subtitle, weight: FIELD_WEIGHTS.subtitle });
  fields.push({ text: item.id, weight: FIELD_WEIGHTS.id });
  return fields;
}

/** Builds the searcher for a set of items (index build happens once here). */
export function createPaletteSearcher(items: PaletteItem[]): FuzzySearcher<PaletteItem> {
  return createSearcher(items, paletteItemFields);
}

/** A result row: the item, its final blended rank, and highlight indexes into the title. */
export interface RankedItem {
  item: PaletteItem;
  score: number;
  matchIndexes: number[];
}

/** How many results the overlay renders at most — a hard cap so the list stays instant. */
export const RESULT_CAP = 50;

// Empty-query recents ordering: when the operator has typed nothing, pending
// asks are the loudest, then the live work objects, then declared/static ones.
const TYPE_PRIORITY: Record<PaletteItemType, number> = {
  inbox: 0,
  mission: 1,
  session: 2,
  fleet: 3,
  agent: 4,
  workspace: 5,
  action: 6,
};

/**
 * The ranked results for the current query. On a blank query it is the recents
 * view — most-frecent first, then by type priority (asks loudest), preserving
 * provider order within a tier. On a real query it is `matchScore × frecencyBoost`,
 * highest first, capped. Frecency only ever lifts a match; it never hides a
 * freshly-typed, well-matched item behind history (a zero match stays out).
 */
export function computeResults(
  searcher: FuzzySearcher<PaletteItem>,
  allItems: PaletteItem[],
  query: string,
  store: FrecencyStore,
  now: number,
  cap: number = RESULT_CAP,
): RankedItem[] {
  if (!query.trim()) return recents(allItems, store, now, cap);

  const matches = searcher.search(query);
  const ranked: RankedItem[] = matches.map(m => ({
    item: m.item,
    score: blend(m.score, frecencyBoost(store[m.item.id], now)),
    matchIndexes: m.matchIndexes,
  }));
  ranked.sort((a, b) => b.score - a.score);
  return ranked.slice(0, cap);
}

function recents(
  allItems: PaletteItem[],
  store: FrecencyStore,
  now: number,
  cap: number,
): RankedItem[] {
  const withOrder = allItems.map((item, order) => ({
    item,
    order,
    boost: frecencyBoost(store[item.id], now),
  }));
  withOrder.sort((a, b) => {
    // Used items first, hottest at the top.
    if (b.boost !== a.boost) return b.boost - a.boost;
    // Then loudest type (asks), then original provider order.
    const typeDelta = TYPE_PRIORITY[a.item.type] - TYPE_PRIORITY[b.item.type];
    if (typeDelta !== 0) return typeDelta;
    return a.order - b.order;
  });
  return withOrder.slice(0, cap).map(({ item, boost }) => ({ item, score: boost, matchIndexes: [] }));
}

// ── Keyboard / open state machine ───────────────────────────────────────────

export interface PaletteUIState {
  open: boolean;
  query: string;
  /** Index into the current result list of the highlighted row. */
  selected: number;
}

export const INITIAL_PALETTE_STATE: PaletteUIState = { open: false, query: '', selected: 0 };

export type PaletteAction =
  | { type: 'open' }
  | { type: 'close' }
  | { type: 'toggle' }
  | { type: 'setQuery'; query: string }
  | { type: 'move'; delta: number; count: number }
  | { type: 'setSelected'; index: number; count: number };

/** Pure reducer for the overlay's open/query/selection state. */
export function paletteReducer(state: PaletteUIState, action: PaletteAction): PaletteUIState {
  switch (action.type) {
    case 'open':
      return { open: true, query: '', selected: 0 };
    case 'close':
      return { open: false, query: '', selected: 0 };
    case 'toggle':
      return state.open ? { open: false, query: '', selected: 0 } : { open: true, query: '', selected: 0 };
    case 'setQuery':
      // Any edit re-ranks from scratch, so selection returns to the top match.
      return { ...state, query: action.query, selected: 0 };
    case 'move':
      return { ...state, selected: wrapIndex(state.selected + action.delta, action.count) };
    case 'setSelected':
      return { ...state, selected: clampIndex(action.index, action.count) };
    default:
      return state;
  }
}

/** Wraps a selection index around a list of `count` items (empty list → 0). */
export function wrapIndex(index: number, count: number): number {
  if (count <= 0) return 0;
  return ((index % count) + count) % count;
}

function clampIndex(index: number, count: number): number {
  if (count <= 0) return 0;
  return Math.max(0, Math.min(index, count - 1));
}

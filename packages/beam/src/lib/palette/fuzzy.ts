import fuzzysort from 'fuzzysort';

/**
 * The goto-anything palette's matching layer. `fuzzysort` is adopted only as the
 * inner char-level scorer, hidden entirely BEHIND this module's own interface —
 * everything the palette's ranking actually depends on is owned here, not by the
 * library:
 *
 *  - per-field scoring with weights (title > keywords > subtitle > id),
 *  - multi-token AND (the query is split on whitespace; EVERY token must match
 *    somewhere, and the per-item score is the sum of each token's best match),
 *  - smart-case (a query with any uppercase letter matches case-sensitively;
 *    an all-lowercase query is case-insensitive),
 *  - NFKD normalization + diacritic stripping done ONCE at index-build time and
 *    cached on each prepared field, so 'München' is reachable by typing
 *    'munchen',
 *  - match-index passthrough for highlighting, remapped back onto the ORIGINAL
 *    (un-normalized) title so a stripped diacritic never shifts the highlight.
 *
 * The inner scorer is swappable: {@link InnerScorer} is the whole seam. The
 * default is {@link fuzzysortScorer}; a future owned DP scorer implements the
 * same two methods and drops in via `createSearcher`'s `scorer` argument with no
 * change to any of the ownership above. Nothing outside this file imports
 * `fuzzysort`.
 */

/** One fuzzy match against a single field: a 0..1 score and the matched indexes into the normalized field text. */
export interface ScoreResult {
  /** 1 is a perfect match, 0 no match. Higher is better (matches fuzzysort v3). */
  score: number;
  /** Matched character positions in the normalized field text, ascending. */
  indexes: number[];
}

/**
 * The pluggable inner scorer — the single seam that lets the char-level matcher
 * be replaced (fuzzysort today, an owned DP scorer later) without touching the
 * weighting / multi-token / smart-case / normalization logic above it.
 *
 * `prepare` is called once per field at index-build time; its opaque result is
 * cached on the field and handed back to `score` on every keystroke. `score`
 * returns `null` for no match.
 */
export interface InnerScorer {
  prepare(normalized: string): unknown;
  score(query: string, prepared: unknown): ScoreResult | null;
}

/** The default inner scorer: fuzzysort v3, wrapped so its API never leaks past this module. */
export const fuzzysortScorer: InnerScorer = {
  prepare: (normalized: string) => fuzzysort.prepare(normalized),
  score: (query: string, prepared: unknown): ScoreResult | null => {
    // `prepared` is opaque to callers but this impl knows it produced a
    // fuzzysort Prepared, so the cast is local and honest.
    const result = fuzzysort.single(query, prepared as ReturnType<typeof fuzzysort.prepare>);
    if (!result) return null;
    return { score: result.score, indexes: Array.from(result.indexes) };
  },
};

/** A single searchable field of an item, with the weight its match contributes. */
export interface WeightedField {
  text: string;
  weight: number;
  /** When true, this field's matched indexes feed the highlight (typically the title). */
  highlight?: boolean;
}

/** A ranked search hit: the item, its summed weighted score, and highlight indexes into the ORIGINAL title. */
export interface SearchMatch<T> {
  item: T;
  score: number;
  /** Positions in the original (un-normalized) highlight field to emphasize. */
  matchIndexes: number[];
}

export interface FuzzySearcher<T> {
  /** Synchronous, in-memory search. Empty/blank query returns no matches (the caller renders recents instead). */
  search(query: string): SearchMatch<T>[];
}

/**
 * NFKD-normalizes and strips diacritics, preserving case, while building a map
 * from each normalized UTF-16 index back to its originating index in the source
 * string. The map is what keeps highlight indexes valid after stripping: e.g.
 * 'ü' (1 unit) becomes 'u' (1 unit) but a ligature could change length, and the
 * map absorbs that so a highlight never drifts.
 */
export function buildNormalized(text: string): { norm: string; map: number[] } {
  let norm = '';
  const map: number[] = [];
  for (let i = 0; i < text.length; i++) {
    const decomposed = text[i].normalize('NFKD').replace(/\p{M}/gu, '');
    for (let j = 0; j < decomposed.length; j++) {
      norm += decomposed[j];
      map.push(i);
    }
  }
  return { norm, map };
}

/** Whether a query should be matched case-sensitively (smart-case: any uppercase letter opts in). */
export function isCaseSensitive(query: string): boolean {
  return /\p{Lu}/u.test(query);
}

interface PreparedField {
  weight: number;
  highlight: boolean;
  /** Diacritic-stripped, case-preserved field text — what the smart-case check reads. */
  norm: string;
  /** norm index -> original index, for remapping highlight positions. */
  map: number[];
  prepared: unknown;
}

interface IndexedItem<T> {
  item: T;
  fields: PreparedField[];
}

/**
 * Builds an in-memory searcher over `items`. Normalization and scorer-prepare
 * run once here (the index build); `search` is then a pure synchronous scan with
 * no async, no warmup, and no allocation of new prepared targets — the palette's
 * per-keystroke latency budget depends on that.
 */
export function createSearcher<T>(
  items: T[],
  toFields: (item: T) => WeightedField[],
  scorer: InnerScorer = fuzzysortScorer,
): FuzzySearcher<T> {
  const index: IndexedItem<T>[] = items.map(item => ({
    item,
    fields: toFields(item)
      .filter(f => f.text.length > 0)
      .map(f => {
        const { norm, map } = buildNormalized(f.text);
        return {
          weight: f.weight,
          highlight: f.highlight ?? false,
          norm,
          map,
          prepared: scorer.prepare(norm),
        };
      }),
  }));

  const search = (query: string): SearchMatch<T>[] => {
    const trimmed = query.trim();
    if (!trimmed) return [];
    const caseSensitive = isCaseSensitive(trimmed);
    const tokens = trimmed
      .split(/\s+/)
      .map(tok => buildNormalized(tok).norm)
      .filter(Boolean);
    if (tokens.length === 0) return [];

    const matches: SearchMatch<T>[] = [];

    for (const entry of index) {
      let total = 0;
      const highlightIndexes = new Set<number>();
      let allTokensMatched = true;

      for (const token of tokens) {
        let bestWeighted = -Infinity;

        for (const field of entry.fields) {
          const result = scorer.score(token, field.prepared);
          if (!result) continue;
          // Smart-case: fuzzysort is always case-insensitive, so enforce case
          // ourselves against the case-preserved normalized text.
          if (caseSensitive && !caseMatches(field.norm, token, result.indexes)) continue;

          const weighted = result.score * field.weight;
          if (weighted > bestWeighted) bestWeighted = weighted;

          // Collect highlight positions whenever the token hits the highlight
          // field — independent of which field ranked best for this token.
          if (field.highlight) {
            for (const idx of result.indexes) highlightIndexes.add(field.map[idx]);
          }
        }

        if (bestWeighted === -Infinity) {
          allTokensMatched = false;
          break;
        }
        total += bestWeighted;
      }

      if (!allTokensMatched) continue;
      matches.push({
        item: entry.item,
        score: total,
        matchIndexes: [...highlightIndexes].sort((a, b) => a - b),
      });
    }

    // Highest score first; V8's Array.sort is stable, so equal scores keep index order.
    matches.sort((a, b) => b.score - a.score);
    return matches;
  };

  return { search };
}

/** Verifies the matched characters equal the query characters case-sensitively (smart-case gate). */
function caseMatches(norm: string, token: string, indexes: number[]): boolean {
  if (indexes.length !== token.length) return false;
  for (let k = 0; k < indexes.length; k++) {
    if (norm[indexes[k]] !== token[k]) return false;
  }
  return true;
}

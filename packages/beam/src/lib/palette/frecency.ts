/**
 * Frecency for the goto-anything palette: a per-item usage record (how often,
 * how recently) that BOOSTS a match's rank without ever hiding one. It is
 * automatic-only by the blind-spot doctrine — there is no pin/favorite UI — so
 * the store is written silently on every execute and read on every open.
 *
 * The blend is deliberately a pure multiplier on the match score:
 *
 *     rank = matchScore × frecencyBoost(entry, now)
 *
 * `frecencyBoost` is 1.0 for a never-used item and only ever ≥ 1, so history can
 * lift a familiar result above an equally-matched stranger but can NEVER push a
 * freshly-typed, well-matched item below a stale favorite (a zero match score
 * stays zero whatever its history). Boost grows with use count and decays toward
 * 1.0 as the last use recedes (a 3-day half-life), so yesterday's hot item cools
 * off on its own rather than fossilizing at the top.
 *
 * The math is pure and `now`-injected so it is testable without a clock; the
 * localStorage layer is a thin, guarded adapter (node/test and private-mode
 * safe) that degrades to an in-memory map.
 */

export interface FrecencyEntry {
  count: number;
  /** Epoch milliseconds of the last use. */
  lastUsed: number;
}

export type FrecencyStore = Record<string, FrecencyEntry>;

const STORAGE_KEY = 'beam_palette_frecency';
/** Recency half-life: a boost contribution halves every 3 days since last use. */
const HALF_LIFE_MS = 3 * 24 * 60 * 60 * 1000;
/** How hard frecency can push — the maximum extra multiplier from a very hot item. */
const BOOST_FACTOR = 0.6;
/** Cap stored entries so the store cannot grow without bound; least-recently-used are dropped. */
const MAX_ENTRIES = 250;

/** Records one use of `id` at `now`, returning a new store (pure). */
export function recordUsage(store: FrecencyStore, id: string, now: number): FrecencyStore {
  const prev = store[id];
  return {
    ...store,
    [id]: { count: (prev?.count ?? 0) + 1, lastUsed: now },
  };
}

/**
 * The multiplier an item's history earns. 1.0 for never-used (no entry or a
 * zeroed count); otherwise `1 + BOOST_FACTOR × log2(1+count) × recency`, where
 * recency decays from 1 (used just now) toward 0 (long ago) on the half-life.
 * The log keeps a heavily-used item from dominating linearly.
 */
export function frecencyBoost(entry: FrecencyEntry | undefined, now: number): number {
  if (!entry || entry.count <= 0) return 1;
  const ageMs = Math.max(0, now - entry.lastUsed);
  const recency = Math.pow(0.5, ageMs / HALF_LIFE_MS);
  const strength = Math.log2(1 + entry.count);
  return 1 + BOOST_FACTOR * strength * recency;
}

/** The settled blend: match score scaled by the frecency boost. */
export function blend(matchScore: number, boost: number): number {
  return matchScore * boost;
}

// ── localStorage adapter (guarded; degrades to memory) ──────────────────────

const memoryFallback = new Map<string, string>();

function getRaw(): string | null {
  try {
    if (typeof localStorage !== 'undefined') return localStorage.getItem(STORAGE_KEY);
  } catch {
    /* private mode / unavailable — fall through to memory */
  }
  return memoryFallback.get(STORAGE_KEY) ?? null;
}

function setRaw(value: string): void {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.setItem(STORAGE_KEY, value);
      return;
    }
  } catch {
    /* private mode / unavailable — fall through to memory */
  }
  memoryFallback.set(STORAGE_KEY, value);
}

/** Reads the stored usage map, tolerating absent/corrupt data as an empty store. */
export function loadFrecency(): FrecencyStore {
  const raw = getRaw();
  if (!raw) return {};
  try {
    const parsed = JSON.parse(raw) as unknown;
    if (!parsed || typeof parsed !== 'object') return {};
    const out: FrecencyStore = {};
    for (const [id, v] of Object.entries(parsed as Record<string, unknown>)) {
      if (
        v &&
        typeof v === 'object' &&
        typeof (v as FrecencyEntry).count === 'number' &&
        typeof (v as FrecencyEntry).lastUsed === 'number'
      ) {
        out[id] = { count: (v as FrecencyEntry).count, lastUsed: (v as FrecencyEntry).lastUsed };
      }
    }
    return out;
  } catch {
    return {};
  }
}

/** Persists the store, pruning to the most-recently-used {@link MAX_ENTRIES} first. */
export function saveFrecency(store: FrecencyStore): void {
  setRaw(JSON.stringify(pruneStore(store)));
}

/** Keeps the {@link MAX_ENTRIES} most-recently-used entries (pure; also unit-testable). */
export function pruneStore(store: FrecencyStore): FrecencyStore {
  const ids = Object.keys(store);
  if (ids.length <= MAX_ENTRIES) return store;
  const kept = ids
    .sort((a, b) => store[b].lastUsed - store[a].lastUsed)
    .slice(0, MAX_ENTRIES);
  const out: FrecencyStore = {};
  for (const id of kept) out[id] = store[id];
  return out;
}

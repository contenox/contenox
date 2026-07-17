import { useCallback, useSyncExternalStore } from 'react';

/**
 * A boolean UI preference that is SHARED across every consumer and persisted to
 * localStorage — e.g. "is the workspace file panel expanded". Backed by a
 * per-key module store so all mounted consumers stay in lockstep when any one
 * toggles it.
 *
 * This exists because the chat workspace now keeps several `ChatSessionTab`s
 * mounted at once (one per open tab). The panel-visibility toggles are a
 * workspace-wide preference, not a per-session one, so a private `useState` per
 * tab would drift: toggling "show terminal" in the active tab would leave the
 * other already-mounted tabs out of sync. A single shared store keeps every tab
 * consistent AND persists the choice. Best-effort persistence: it degrades to
 * in-memory when localStorage is unavailable (private mode / SSR / tests).
 */
export interface ToggleStore {
  /** Current value (in-memory, seeded from localStorage on first access). */
  getSnapshot: () => boolean;
  /** Subscribe to changes; returns an unsubscribe. */
  subscribe: (listener: () => void) => () => void;
  /** Set the value, persist it, and notify subscribers (no-op if unchanged). */
  set: (value: boolean) => void;
  /** Flip the value. */
  toggle: () => void;
}

const stores = new Map<string, ToggleStore>();

function readPersisted(key: string): boolean {
  try {
    if (typeof localStorage !== 'undefined') return localStorage.getItem(key) === '1';
  } catch {
    /* best-effort */
  }
  return false;
}

function writePersisted(key: string, value: boolean): void {
  try {
    if (typeof localStorage !== 'undefined') localStorage.setItem(key, value ? '1' : '0');
  } catch {
    /* best-effort */
  }
}

/**
 * The shared {@link ToggleStore} for `key`, created once per key and cached. The
 * value is read from localStorage on first access, then kept in memory (the
 * source of truth for all subscribers) and written back on every change.
 */
export function toggleStore(key: string): ToggleStore {
  const existing = stores.get(key);
  if (existing) return existing;

  const listeners = new Set<() => void>();
  let value = readPersisted(key);

  const store: ToggleStore = {
    getSnapshot: () => value,
    subscribe: listener => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    set: next => {
      if (next === value) return;
      value = next;
      writePersisted(key, next);
      listeners.forEach(listener => listener());
    },
    toggle: () => store.set(!value),
  };
  stores.set(key, store);
  return store;
}

export interface PersistentToggle {
  /** Current shared value. */
  open: boolean;
  /** Flip the shared value. */
  toggle: () => void;
  /** Set the shared value explicitly. */
  set: (value: boolean) => void;
}

/**
 * React binding over {@link toggleStore}: subscribes this component to a shared,
 * persisted boolean preference identified by `key`.
 */
export function usePersistentToggle(key: string): PersistentToggle {
  const store = toggleStore(key);
  const open = useSyncExternalStore(store.subscribe, store.getSnapshot, store.getSnapshot);
  const toggle = useCallback(() => store.toggle(), [store]);
  const set = useCallback((value: boolean) => store.set(value), [store]);
  return { open, toggle, set };
}

import { acpSessionReducer, initialAcpSessionState, type AcpSessionAction, type AcpSessionState } from './acpSessionState';
import type { SessionConfigOption, SessionInfo } from '../lib/acp';

/**
 * Pure, framework-free workspace-level state: reducer + types only, no React,
 * no WebSocket. This is the multi-session layer sitting above
 * `acpSessionState.ts` (which holds one session's live timeline) — it tracks
 * connection lifecycle and the `session/list` roster. `useAcpWorkspace.ts`
 * wires this reducer into `useReducer`; `acpWorkspaceController.ts` dispatches
 * actions in response to `AcpClient` events. Kept separate so both can be
 * unit-tested without mounting a component (see `acpWorkspaceState.test.ts`).
 *
 * This file ALSO owns the multiplexing layer (`acpSessionsReducer`, below):
 * one `AcpSessionState` slice PER open session, keyed by sessionId, so several
 * sessions can be subscribed and accumulating their `session/update` traffic
 * concurrently (see the workspace-tabs blueprint's "Multiplexing finding").
 * The single-view UI renders whichever slice is focused (`selectFocusedSession`).
 */

export type AcpWorkspaceStatus =
  | 'connecting'
  | 'ready'
  | 'reconnecting'
  | 'disconnected'
  | 'setup_required'
  | 'error';

/**
 * Outcome of the most recent explicit `openSession()` call (deep link / rail
 * switch) — kept independent of `AcpWorkspaceStatus`, which is reserved for
 * the connection lifecycle. Previously the page inferred "session not found"
 * from a combination of `status === 'error'`, `activeSessionId`, and empty
 * session state (see acpWorkspaceController.ts's pre-Stage-4 history); that
 * heuristic is gone — this field is the one authoritative signal.
 */
export type AcpSessionLoadState = 'loading' | 'ready' | 'not_found' | 'error';

export interface AcpWorkspaceState {
  status: AcpWorkspaceStatus;
  /** Set on `setup_required`/`error`; cleared on `connecting`/`ready`. */
  error: string | null;
  agentName: string | null;
  /** The `session/list` roster, sorted freshest-first (see `compareByFreshness`). */
  sessions: SessionInfo[];
  activeSessionId: string | null;
  /** Outcome of the in-flight/most-recent `openSession()` call — see `AcpSessionLoadState`. */
  sessionLoadState: AcpSessionLoadState;
  /** Set on `session_load_failed`; cleared on `session_load_start`/`session_load_succeeded`. */
  sessionLoadError: string | null;
  /**
   * The workspace-level (session-less) config options advertised in the
   * agent's `initialize` `_meta` (see `workspaceConfigOptionsFromInit`).
   * Re-read on every (re)connect. Empty for agents that don't advertise the
   * extension. The empty-chat surface renders its model/think/HITL/token-limit
   * controls from this, before any session exists — see `AcpChatPage`.
   */
  workspaceConfigOptions: SessionConfigOption[];
}

export const initialAcpWorkspaceState: AcpWorkspaceState = {
  status: 'connecting',
  error: null,
  agentName: null,
  sessions: [],
  activeSessionId: null,
  sessionLoadState: 'ready',
  sessionLoadError: null,
  workspaceConfigOptions: [],
};

export type AcpWorkspaceAction =
  | { type: 'connecting' }
  | { type: 'ready'; agentName: string | null; workspaceConfigOptions: SessionConfigOption[] }
  | { type: 'reconnecting' }
  | { type: 'disconnected' }
  /** JSON-RPC `-32000 auth_required` — terminal, the controller never auto-retries past this (see acpWorkspaceController.ts). */
  | { type: 'setup_required'; message: string }
  | { type: 'error'; message: string }
  /** A full, authoritative `session/list` result (paginated to completion) replaces the roster. */
  | { type: 'sessions_replaced'; sessions: SessionInfo[] }
  /** Insert-or-merge one session (new session created/opened, or a live `session_info_update`) and re-sort by freshness. */
  | { type: 'session_upserted'; session: SessionInfo }
  | { type: 'session_removed'; sessionId: string }
  | { type: 'active_session_changed'; sessionId: string | null }
  /** An explicit `openSession()` call started — the page can show a loading affordance instead of stale content. */
  | { type: 'session_load_start' }
  /** The `openSession()` call resolved: whichever session was requested is now open. */
  | { type: 'session_load_succeeded' }
  /** `openSession()`'s `session/load` failed with an unknown-session error — see acpWorkspaceController.ts's `classifySessionOpenFailure`. */
  | { type: 'session_load_not_found' }
  /** `openSession()`'s `session/load` failed for a reason other than "unknown session". */
  | { type: 'session_load_failed'; message: string };

/**
 * Freshest-first: sessions with a parseable `updatedAt` sort by it
 * descending; sessions with none sort after all sessions that have one
 * (stable amongst themselves and amongst equal timestamps — `Array.sort` is
 * stable per spec).
 */
function compareByFreshness(a: SessionInfo, b: SessionInfo): number {
  const at = a.updatedAt ? Date.parse(a.updatedAt) : NaN;
  const bt = b.updatedAt ? Date.parse(b.updatedAt) : NaN;
  const aValid = !Number.isNaN(at);
  const bValid = !Number.isNaN(bt);
  if (aValid && bValid) return bt - at;
  if (aValid) return -1;
  if (bValid) return 1;
  return 0;
}

/** Merges `incoming` onto `existing` (if any), treating an absent/undefined field on `incoming` as "keep existing" rather than clearing it. */
function mergeSessionInfo(existing: SessionInfo | undefined, incoming: SessionInfo): SessionInfo {
  if (!existing) return incoming;
  return {
    sessionId: incoming.sessionId,
    cwd: incoming.cwd ?? existing.cwd,
    title: incoming.title ?? existing.title,
    updatedAt: incoming.updatedAt ?? existing.updatedAt,
  };
}

export function acpWorkspaceReducer(state: AcpWorkspaceState, action: AcpWorkspaceAction): AcpWorkspaceState {
  switch (action.type) {
    case 'connecting':
      return { ...state, status: 'connecting', error: null };

    case 'ready':
      return {
        ...state,
        status: 'ready',
        error: null,
        agentName: action.agentName,
        workspaceConfigOptions: action.workspaceConfigOptions,
      };

    case 'reconnecting':
      return { ...state, status: 'reconnecting', error: null };

    case 'disconnected':
      return { ...state, status: 'disconnected' };

    case 'setup_required':
      return { ...state, status: 'setup_required', error: action.message };

    case 'error':
      return { ...state, status: 'error', error: action.message };

    case 'sessions_replaced': {
      // `session/list` is authoritative for MEMBERSHIP (a session absent from
      // this snapshot really is gone), but NOT for freshness of individual
      // fields: it pages a possibly-lagging server-side index, so a session
      // already open and live-pushing its derived title (`session_upserted`,
      // below) can resolve a `session/list` page — issued moments earlier,
      // e.g. the one every reconnect triggers via `refreshSessions()` — whose
      // row for that same session still has no title. A bare replace would
      // regress that session back to titleless in the roster the instant the
      // page lands, undoing the push with no live event to fix it again. Merge
      // onto whatever we already knew (same `incoming ?? existing` rule as
      // `session_upserted`) so a field already known fresher survives a
      // same-membership refresh.
      const known = new Map(state.sessions.map(s => [s.sessionId, s] as const));
      const sessions = action.sessions.map(incoming => mergeSessionInfo(known.get(incoming.sessionId), incoming));
      return { ...state, sessions: sessions.sort(compareByFreshness) };
    }

    case 'session_upserted': {
      const idx = state.sessions.findIndex(s => s.sessionId === action.session.sessionId);
      const sessions = state.sessions.slice();
      if (idx === -1) {
        sessions.push(action.session);
      } else {
        sessions[idx] = mergeSessionInfo(sessions[idx], action.session);
      }
      return { ...state, sessions: sessions.sort(compareByFreshness) };
    }

    case 'session_removed':
      return { ...state, sessions: state.sessions.filter(s => s.sessionId !== action.sessionId) };

    case 'active_session_changed':
      return { ...state, activeSessionId: action.sessionId };

    case 'session_load_start':
      return { ...state, sessionLoadState: 'loading', sessionLoadError: null };

    case 'session_load_succeeded':
      return { ...state, sessionLoadState: 'ready', sessionLoadError: null };

    case 'session_load_not_found':
      return { ...state, sessionLoadState: 'not_found', sessionLoadError: null };

    case 'session_load_failed':
      return { ...state, sessionLoadState: 'error', sessionLoadError: action.message };

    default:
      return state;
  }
}

// ---------------------------------------------------------------------------
// Multiplexing layer: one live-state slice per open session
// ---------------------------------------------------------------------------

/**
 * Reserved slice key for the "no session open" / empty-chat view. It cannot
 * collide with a real sessionId (the server never mints an empty-string id),
 * and it is where a failed lazy `newSession()` surfaces its error before any
 * session exists — preserving the pre-multiplexing behavior where the single
 * session reducer showed such errors even with `activeSessionId === null`. It
 * is NOT an open session; `selectOpenSessionIds` filters it out.
 */
export const EMPTY_SESSION_KEY = '';

/**
 * The multiplexed session state: an `AcpSessionState` slice per open session
 * (keyed by sessionId, plus the reserved `EMPTY_SESSION_KEY` empty-chat
 * slice), and the key of the slice the single-view UI renders. Each slice
 * accumulates ITS OWN session's `session/update` traffic independently, so a
 * background (non-focused) session keeps streaming while another is shown —
 * the whole point of holding several `client.subscribe()` subscriptions at
 * once (see the controller's per-session `buildSessionHandlers`).
 */
export interface AcpSessionsState {
  /** Live-timeline slice per open session, keyed by sessionId (`EMPTY_SESSION_KEY` for the empty-chat slice). */
  slices: Record<string, AcpSessionState>;
  /** Storage key of the slice the single-view UI renders — a sessionId, or `EMPTY_SESSION_KEY`. */
  focusedKey: string;
}

export const initialAcpSessionsState: AcpSessionsState = {
  slices: {},
  focusedKey: EMPTY_SESSION_KEY,
};

export type AcpSessionsAction =
  /** Apply a single-session action to `key`'s slice, creating it from `initialAcpSessionState` if absent. */
  | { type: 'session_dispatch'; key: string; action: AcpSessionAction }
  /** Drop `key`'s slice entirely (a closed/deleted session). Never changes `focusedKey` — the controller re-points focus explicitly. */
  | { type: 'session_closed'; key: string }
  /** Re-point which slice the single-view UI renders. */
  | { type: 'session_focused'; key: string };

export function acpSessionsReducer(state: AcpSessionsState, action: AcpSessionsAction): AcpSessionsState {
  switch (action.type) {
    case 'session_dispatch': {
      const prev = state.slices[action.key] ?? initialAcpSessionState;
      const next = acpSessionReducer(prev, action.action);
      if (next === prev) return state;
      return { ...state, slices: { ...state.slices, [action.key]: next } };
    }

    case 'session_closed': {
      if (!(action.key in state.slices)) return state;
      const slices = { ...state.slices };
      delete slices[action.key];
      return { ...state, slices };
    }

    case 'session_focused':
      return state.focusedKey === action.key ? state : { ...state, focusedKey: action.key };

    default:
      return state;
  }
}

/**
 * The focused session's slice — what the single-view UI renders. Returns the
 * shared `initialAcpSessionState` when the focused key has no slice yet (e.g.
 * the empty-chat view before anything has been dispatched to it), so the
 * reference is stable across unrelated background-session updates.
 */
export function selectFocusedSession(state: AcpSessionsState): AcpSessionState {
  return state.slices[state.focusedKey] ?? initialAcpSessionState;
}

/** The ids of all open sessions (every slice key except the reserved empty-chat one). */
export function selectOpenSessionIds(state: AcpSessionsState): string[] {
  return Object.keys(state.slices).filter(key => key !== EMPTY_SESSION_KEY);
}

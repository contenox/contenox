import type { SessionInfo } from '../lib/acp';

/**
 * Pure, framework-free workspace-level state: reducer + types only, no React,
 * no WebSocket. This is the multi-session layer sitting above
 * `acpSessionState.ts` (which holds only the one currently-open session's
 * live timeline) — it tracks connection lifecycle and the `session/list`
 * roster. `useAcpWorkspace.ts` wires this reducer into `useReducer`;
 * `acpWorkspaceController.ts` dispatches actions in response to `AcpClient`
 * events. Kept separate so both can be unit-tested without mounting a
 * component (see `acpWorkspaceState.test.ts`).
 */

export type AcpWorkspaceStatus =
  | 'connecting'
  | 'ready'
  | 'reconnecting'
  | 'disconnected'
  | 'setup_required'
  | 'error';

export interface AcpWorkspaceState {
  status: AcpWorkspaceStatus;
  /** Set on `setup_required`/`error`; cleared on `connecting`/`ready`. */
  error: string | null;
  agentName: string | null;
  /** The `session/list` roster, sorted freshest-first (see `compareByFreshness`). */
  sessions: SessionInfo[];
  activeSessionId: string | null;
}

export const initialAcpWorkspaceState: AcpWorkspaceState = {
  status: 'connecting',
  error: null,
  agentName: null,
  sessions: [],
  activeSessionId: null,
};

export type AcpWorkspaceAction =
  | { type: 'connecting' }
  | { type: 'ready'; agentName: string | null }
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
  | { type: 'active_session_changed'; sessionId: string | null };

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
      return { ...state, status: 'ready', error: null, agentName: action.agentName };

    case 'reconnecting':
      return { ...state, status: 'reconnecting', error: null };

    case 'disconnected':
      return { ...state, status: 'disconnected' };

    case 'setup_required':
      return { ...state, status: 'setup_required', error: action.message };

    case 'error':
      return { ...state, status: 'error', error: action.message };

    case 'sessions_replaced':
      return { ...state, sessions: [...action.sessions].sort(compareByFreshness) };

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

    default:
      return state;
  }
}

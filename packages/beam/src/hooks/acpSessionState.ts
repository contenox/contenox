import type {
  PlanEntry,
  RequestPermissionRequest,
  ToolCallContent,
  ToolCallLocation,
  ToolCallStatus,
  ToolKind,
} from '../lib/acp';
import type { ToolCallEvent, UsageEvent } from '../lib/acp';

/**
 * Pure, framework-free state for one ACP chat session: reducer + types only,
 * no React, no WebSocket. `useAcpSession.ts` wires this reducer into
 * `useReducer`; `acpSessionController.ts` dispatches actions in response to
 * `AcpClient` events. Kept separate so both can be unit-tested without
 * mounting a component (see `useAcpSession.test.tsx`).
 */

export type AcpConnectionStatus = 'connecting' | 'ready' | 'error';

export interface AcpChatMessage {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  /** True while more `agent_message_chunk`s are still expected for this message. */
  streaming?: boolean;
}

export interface AcpToolCallState {
  toolCallId: string;
  title?: string;
  kind?: ToolKind;
  status?: ToolCallStatus;
  content?: ToolCallContent[];
  locations?: ToolCallLocation[];
  rawInput?: unknown;
  rawOutput?: unknown;
}

export interface AcpUsageState {
  used: number;
  size: number;
}

export interface AcpSessionState {
  status: AcpConnectionStatus;
  /** Set on a `connection_error`/`prompt_error`; cleared on the next successful transition. */
  error: string | null;
  agentName: string | null;
  sessionId: string | null;
  messages: AcpChatMessage[];
  toolCalls: Record<string, AcpToolCallState>;
  /** Insertion order of `toolCalls` keys, so the UI can render cards in arrival order. */
  toolCallOrder: string[];
  plan: PlanEntry[];
  usage: AcpUsageState | null;
  pendingPermission: RequestPermissionRequest | null;
  /** True for the duration of one `session/prompt` call. */
  isPrompting: boolean;
}

export const initialAcpSessionState: AcpSessionState = {
  status: 'connecting',
  error: null,
  agentName: null,
  sessionId: null,
  messages: [],
  toolCalls: {},
  toolCallOrder: [],
  plan: [],
  usage: null,
  pendingPermission: null,
  isPrompting: false,
};

export type AcpSessionAction =
  | { type: 'connecting' }
  | { type: 'ready'; sessionId: string; agentName: string | null }
  | { type: 'connection_error'; message: string }
  | { type: 'user_message'; id: string; text: string }
  | { type: 'prompt_start' }
  | { type: 'message_chunk'; id: string; text: string }
  | { type: 'tool_call'; event: ToolCallEvent }
  | { type: 'plan'; entries: PlanEntry[] }
  | { type: 'usage'; usage: UsageEvent }
  | { type: 'permission_request'; request: RequestPermissionRequest }
  | { type: 'permission_resolved' }
  | { type: 'prompt_end' }
  | { type: 'prompt_error'; message: string };

function endStreaming(messages: AcpChatMessage[]): AcpChatMessage[] {
  if (!messages.some(m => m.streaming)) return messages;
  return messages.map(m => (m.streaming ? { ...m, streaming: false } : m));
}

export function acpSessionReducer(state: AcpSessionState, action: AcpSessionAction): AcpSessionState {
  switch (action.type) {
    case 'connecting':
      return { ...state, status: 'connecting', error: null };

    case 'ready':
      return {
        ...state,
        status: 'ready',
        error: null,
        sessionId: action.sessionId,
        agentName: action.agentName,
      };

    case 'connection_error':
      return { ...state, status: 'error', error: action.message };

    case 'user_message':
      return {
        ...state,
        messages: [...state.messages, { id: action.id, role: 'user', text: action.text }],
      };

    case 'prompt_start':
      return { ...state, isPrompting: true, error: null };

    case 'message_chunk': {
      const idx = state.messages.findIndex(m => m.id === action.id);
      if (idx === -1) {
        return {
          ...state,
          messages: [...state.messages, { id: action.id, role: 'assistant', text: action.text, streaming: true }],
        };
      }
      const messages = state.messages.slice();
      const existing = messages[idx];
      messages[idx] = { ...existing, text: existing.text + action.text, streaming: true };
      return { ...state, messages };
    }

    case 'tool_call': {
      const { event } = action;
      const existing = state.toolCalls[event.toolCallId];
      const merged: AcpToolCallState = {
        toolCallId: event.toolCallId,
        title: event.title ?? existing?.title,
        kind: event.kind ?? existing?.kind,
        status: event.status ?? existing?.status,
        content: event.content ?? existing?.content,
        locations: event.locations ?? existing?.locations,
        rawInput: event.rawInput ?? existing?.rawInput,
        rawOutput: event.rawOutput ?? existing?.rawOutput,
      };
      return {
        ...state,
        toolCalls: { ...state.toolCalls, [event.toolCallId]: merged },
        toolCallOrder: existing ? state.toolCallOrder : [...state.toolCallOrder, event.toolCallId],
      };
    }

    case 'plan':
      return { ...state, plan: action.entries };

    case 'usage':
      return { ...state, usage: { used: action.usage.used, size: action.usage.size } };

    case 'permission_request':
      return { ...state, pendingPermission: action.request };

    case 'permission_resolved':
      return { ...state, pendingPermission: null };

    case 'prompt_end':
      return { ...state, isPrompting: false, messages: endStreaming(state.messages) };

    case 'prompt_error':
      return {
        ...state,
        isPrompting: false,
        error: action.message,
        messages: endStreaming(state.messages),
      };

    default:
      return state;
  }
}

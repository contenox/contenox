import type {
  AvailableCommand,
  PlanEntry,
  RequestPermissionRequest,
  SessionConfigOption,
  StopReason,
  ToolCallContent,
  ToolCallLocation,
  ToolCallStatus,
  ToolKind,
} from '../lib/acp';
import type { ToolCallEvent, UsageEvent } from '../lib/acp';

/**
 * Pure, framework-free state for the ACP workspace's currently-open session:
 * reducer + types only, no React, no WebSocket. Exactly one instance of this
 * state is live at a time — the session `acpWorkspaceController.ts` currently
 * has subscribed — and it is reset (`session_reset`) whenever the workspace
 * switches which session is open. `useAcpWorkspace.ts` wires this reducer into
 * `useReducer`; `acpWorkspaceController.ts` dispatches actions in response to
 * `AcpClient` events. Kept separate so both can be unit-tested without
 * mounting a component (see `acpSessionState.test.ts`).
 *
 * Unified timeline (D4): `items` records arrival order across BOTH messages
 * and tool calls — this is what lets the UI render one interleaved thread
 * instead of two separately-ordered lists. Thought chunks are NOT separate
 * timeline items; they attach to their parent message (by `messageId`) as
 * collapsible block data, exactly like the text itself.
 */

export type AcpTimelineItemKind = 'message' | 'tool_call';

/** One entry in `AcpSessionState.items`, in arrival order. Look the full record up in `messages`/`toolCalls` by `id`. */
export interface AcpTimelineItem {
  kind: AcpTimelineItemKind;
  id: string;
}

export interface AcpChatMessage {
  id: string;
  role: 'user' | 'assistant';
  text: string;
  /** True while more `agent_message_chunk`/`user_message_chunk`s are still expected for this message. */
  streaming?: boolean;
  /** Collapsible "thinking" block (`agent_thought_chunk`s sharing this message's id). Assistant-only. */
  thinking?: string;
  /** True while more `agent_thought_chunk`s are still expected for this message. */
  thinkingStreaming?: boolean;
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

/**
 * Transient banner state for the currently-open session, driven by the
 * workspace controller's reconnect supervisor: `'disconnected'` while the
 * transport is down and this session's live updates may be stale,
 * `'resumed'` once the session has been successfully re-bound (via
 * `session/resume` or a `session/load` replay fallback) after a drop.
 */
export type AcpSessionConnectionBanner = 'disconnected' | 'resumed' | null;

export interface AcpSessionState {
  sessionId: string | null;
  /** Arrival-ordered index into `messages`/`toolCalls` — the single source of truth for render order. */
  items: AcpTimelineItem[];
  messages: Record<string, AcpChatMessage>;
  toolCalls: Record<string, AcpToolCallState>;
  plan: PlanEntry[];
  usage: AcpUsageState | null;
  configOptions: SessionConfigOption[];
  availableCommands: AvailableCommand[];
  pendingPermission: RequestPermissionRequest | null;
  /** True for the duration of one `session/prompt` call. */
  isPrompting: boolean;
  /** The most recently completed turn's stop reason, if any. */
  stopReason: StopReason | null;
  /** Set on a `prompt_error`; cleared on the next `session_reset`/`prompt_start`. */
  error: string | null;
  connectionBanner: AcpSessionConnectionBanner;
}

export const initialAcpSessionState: AcpSessionState = {
  sessionId: null,
  items: [],
  messages: {},
  toolCalls: {},
  plan: [],
  usage: null,
  configOptions: [],
  availableCommands: [],
  pendingPermission: null,
  isPrompting: false,
  stopReason: null,
  error: null,
  connectionBanner: null,
};

export type AcpSessionAction =
  /** Clears all timeline/turn state and points it at a (possibly null) session — dispatched whenever the workspace opens/creates/deletes-the-active session. */
  | { type: 'session_reset'; sessionId: string | null }
  | { type: 'user_message_chunk'; id: string; text: string }
  | { type: 'message_chunk'; id: string; text: string }
  | { type: 'thought_chunk'; id: string; text: string }
  | { type: 'tool_call'; event: ToolCallEvent }
  | { type: 'plan'; entries: PlanEntry[] }
  | { type: 'usage'; usage: UsageEvent }
  | { type: 'available_commands'; commands: AvailableCommand[] }
  | { type: 'config_options'; configOptions: SessionConfigOption[] }
  | { type: 'permission_request'; request: RequestPermissionRequest }
  | { type: 'permission_resolved' }
  | { type: 'prompt_start' }
  | { type: 'prompt_end'; stopReason: StopReason }
  | { type: 'prompt_error'; message: string }
  /** Transport dropped: this session's live updates are stale until a resume/reload lands. */
  | { type: 'connection_lost' }
  /** The workspace controller re-bound this session after a drop (resume or reload fallback). */
  | { type: 'connection_resumed' };

function ensureItem(items: AcpTimelineItem[], kind: AcpTimelineItemKind, id: string): AcpTimelineItem[] {
  if (items.some(it => it.kind === kind && it.id === id)) return items;
  return [...items, { kind, id }];
}

function upsertMessage(
  messages: Record<string, AcpChatMessage>,
  id: string,
  role: 'user' | 'assistant',
  patch: { text?: string; thinking?: string },
): Record<string, AcpChatMessage> {
  const existing = messages[id];
  const next: AcpChatMessage = existing ? { ...existing } : { id, role, text: '' };
  if (patch.text !== undefined) {
    next.text = next.text + patch.text;
    next.streaming = true;
  }
  if (patch.thinking !== undefined) {
    next.thinking = (next.thinking ?? '') + patch.thinking;
    next.thinkingStreaming = true;
  }
  return { ...messages, [id]: next };
}

/** Clears `streaming`/`thinkingStreaming` on every message once a turn ends (success or error). */
function endStreaming(messages: Record<string, AcpChatMessage>): Record<string, AcpChatMessage> {
  let changed = false;
  const next: Record<string, AcpChatMessage> = {};
  for (const [id, m] of Object.entries(messages)) {
    if (m.streaming || m.thinkingStreaming) {
      changed = true;
      next[id] = { ...m, streaming: false, thinkingStreaming: false };
    } else {
      next[id] = m;
    }
  }
  return changed ? next : messages;
}

export function acpSessionReducer(state: AcpSessionState, action: AcpSessionAction): AcpSessionState {
  switch (action.type) {
    case 'session_reset':
      return { ...initialAcpSessionState, sessionId: action.sessionId };

    case 'user_message_chunk':
      return {
        ...state,
        items: ensureItem(state.items, 'message', action.id),
        messages: upsertMessage(state.messages, action.id, 'user', { text: action.text }),
      };

    case 'message_chunk':
      return {
        ...state,
        items: ensureItem(state.items, 'message', action.id),
        messages: upsertMessage(state.messages, action.id, 'assistant', { text: action.text }),
      };

    case 'thought_chunk':
      return {
        ...state,
        items: ensureItem(state.items, 'message', action.id),
        messages: upsertMessage(state.messages, action.id, 'assistant', { thinking: action.text }),
      };

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
        items: ensureItem(state.items, 'tool_call', event.toolCallId),
        toolCalls: { ...state.toolCalls, [event.toolCallId]: merged },
      };
    }

    case 'plan':
      return { ...state, plan: action.entries };

    case 'usage':
      return { ...state, usage: { used: action.usage.used, size: action.usage.size } };

    case 'available_commands':
      return { ...state, availableCommands: action.commands };

    case 'config_options':
      return { ...state, configOptions: action.configOptions };

    case 'permission_request':
      return { ...state, pendingPermission: action.request };

    case 'permission_resolved':
      return { ...state, pendingPermission: null };

    case 'prompt_start':
      return { ...state, isPrompting: true, error: null };

    case 'prompt_end':
      return { ...state, isPrompting: false, stopReason: action.stopReason, messages: endStreaming(state.messages) };

    case 'prompt_error':
      return {
        ...state,
        isPrompting: false,
        error: action.message,
        messages: endStreaming(state.messages),
      };

    case 'connection_lost':
      return { ...state, connectionBanner: 'disconnected', pendingPermission: null, isPrompting: false };

    case 'connection_resumed':
      return { ...state, connectionBanner: 'resumed' };

    default:
      return state;
  }
}

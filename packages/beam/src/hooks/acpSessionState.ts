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
import type { ToolCallEvent, TerminalOutputPayload, UsageEvent } from '../lib/acp';

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

export type AcpTimelineItemKind = 'message' | 'tool_call' | 'terminal';

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
 * Live scrollback of the session's shell, accumulated from
 * `_contenox.terminalOutput` stream events for the terminal panel. `offset` is
 * the last seen absolute scrollback offset; a `reset` event (the initial
 * snapshot on subscribe/reconnect) replaces `text` rather than appending.
 */
export interface AcpTerminalState {
  text: string;
  offset: number;
}

/** A compact record of one `!` passthrough line, rendered in the transcript. */
export interface AcpTerminalCard {
  id: string;
  command: string;
  output: string;
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
  /** Compact `!` passthrough records, keyed by timeline item id. */
  terminals: Record<string, AcpTerminalCard>;
  /** Live shell scrollback for the terminal panel (null until first output). */
  terminal: AcpTerminalState | null;
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
  /**
   * The canonical `messages` key for the CURRENTLY ACTIVE turn's assistant
   * message, or null before its first chunk has arrived. Exists to fix a
   * stable-identity bug: `acpWorkspaceController.ts`'s `buildSessionHandlers`
   * groups `message_chunk`/`thought_chunk`s by `id ?? currentTurnId`, but
   * `currentTurnId` is only used for chunks that arrive with NO
   * server-assigned `messageId` — if the agent's chunks switch between
   * carrying a real id and not (or a thought chunk arrives under one id and
   * the text chunk under another) mid-turn, naively keying by `action.id`
   * would create a SECOND timeline item, orphaning that item's open/closed
   * toggle state and visually splitting one message in two. Per the
   * documented contract there is exactly one assistant message per turn
   * (see `sendPrompt`'s doc comment), so instead: the FIRST message/thought
   * chunk of an active turn establishes this field as that turn's canonical
   * id, and every subsequent message/thought chunk in the SAME turn is
   * resolved onto it regardless of what id it individually carries. Reset to
   * null on every `prompt_start` (a fresh turn starts with no canonical yet)
   * and implicitly by `session_reset` (full state reset). Deliberately NOT
   * consulted while no turn is active (`isPrompting` false) — replayed
   * history (`session/load`) legitimately contains many distinct messages
   * that must stay distinct, see the reducer's `session/load replay
   * ordering` tests.
   */
  activeTurnMessageId: string | null;
}

export const initialAcpSessionState: AcpSessionState = {
  sessionId: null,
  items: [],
  messages: {},
  toolCalls: {},
  terminals: {},
  terminal: null,
  plan: [],
  usage: null,
  configOptions: [],
  availableCommands: [],
  pendingPermission: null,
  isPrompting: false,
  stopReason: null,
  error: null,
  connectionBanner: null,
  activeTurnMessageId: null,
};

export type AcpSessionAction =
  /** Clears all timeline/turn state and points it at a (possibly null) session — dispatched whenever the workspace opens/creates/deletes-the-active session. */
  | { type: 'session_reset'; sessionId: string | null }
  | { type: 'user_message_chunk'; id: string; text: string }
  | { type: 'message_chunk'; id: string; text: string }
  | { type: 'thought_chunk'; id: string; text: string }
  | { type: 'tool_call'; event: ToolCallEvent }
  /** Live shell output batch (append, or replace when `payload.reset`). */
  | { type: 'terminal_output'; payload: TerminalOutputPayload }
  /** Record a `!` passthrough line as a compact transcript card. */
  | { type: 'terminal_card'; id: string; command: string; output: string }
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

/**
 * `activeTurn` gates `streaming`/`thinkingStreaming`: chunks that arrive
 * while a prompt turn is in flight (`state.isPrompting`) mark the message as
 * still-streaming so the typing indicator shows; chunks arriving with no
 * turn in flight — session/load replay of history, or a late/out-of-turn
 * chunk after `prompt_end`/`prompt_error` already ran — render as an already-
 * complete message instead, since nothing will ever call `endStreaming` for
 * them (there's no turn to end). See the module doc comment's D4 note and
 * `acpWorkspaceController.ts`'s `buildSessionHandlers` (the standing
 * subscription routes both live and replayed chunks through these same
 * actions).
 */
function upsertMessage(
  messages: Record<string, AcpChatMessage>,
  id: string,
  role: 'user' | 'assistant',
  patch: { text?: string; thinking?: string },
  activeTurn: boolean,
): Record<string, AcpChatMessage> {
  const existing = messages[id];
  const next: AcpChatMessage = existing ? { ...existing } : { id, role, text: '' };
  if (patch.text !== undefined) {
    next.text = next.text + patch.text;
    next.streaming = activeTurn;
  }
  if (patch.thinking !== undefined) {
    next.thinking = (next.thinking ?? '') + patch.thinking;
    next.thinkingStreaming = activeTurn;
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
        messages: upsertMessage(state.messages, action.id, 'user', { text: action.text }, state.isPrompting),
      };

    case 'message_chunk': {
      // See `activeTurnMessageId`'s doc comment: while a turn is active, every
      // message/thought chunk resolves onto the turn's single canonical id
      // (established by whichever chunk arrived first), not onto whatever id
      // this particular chunk happens to carry.
      const id = state.isPrompting ? (state.activeTurnMessageId ?? action.id) : action.id;
      return {
        ...state,
        items: ensureItem(state.items, 'message', id),
        messages: upsertMessage(state.messages, id, 'assistant', { text: action.text }, state.isPrompting),
        activeTurnMessageId: state.isPrompting ? id : state.activeTurnMessageId,
      };
    }

    case 'thought_chunk': {
      const id = state.isPrompting ? (state.activeTurnMessageId ?? action.id) : action.id;
      return {
        ...state,
        items: ensureItem(state.items, 'message', id),
        messages: upsertMessage(state.messages, id, 'assistant', { thinking: action.text }, state.isPrompting),
        activeTurnMessageId: state.isPrompting ? id : state.activeTurnMessageId,
      };
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
        items: ensureItem(state.items, 'tool_call', event.toolCallId),
        toolCalls: { ...state.toolCalls, [event.toolCallId]: merged },
      };
    }

    case 'terminal_output': {
      const { chunk, offset, reset } = action.payload;
      const text = reset ? chunk : (state.terminal?.text ?? '') + chunk;
      return { ...state, terminal: { text, offset } };
    }

    case 'terminal_card':
      return {
        ...state,
        items: ensureItem(state.items, 'terminal', action.id),
        terminals: { ...state.terminals, [action.id]: { id: action.id, command: action.command, output: action.output } },
      };

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
      // Fresh turn, fresh canonical-id slot — see `activeTurnMessageId`'s doc
      // comment. Without this reset a second turn's first chunk would
      // silently alias onto the PREVIOUS turn's assistant message instead of
      // starting a new one.
      return { ...state, isPrompting: true, error: null, activeTurnMessageId: null };

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
      // Clears per-message `streaming`/`thinkingStreaming` too (not just
      // `isPrompting`) — a drop mid-turn means no `prompt_end`/`prompt_error`
      // is coming to call `endStreaming` for whatever was in flight, so the
      // "..." indicator would otherwise be stuck forever.
      return {
        ...state,
        connectionBanner: 'disconnected',
        pendingPermission: null,
        isPrompting: false,
        messages: endStreaming(state.messages),
      };

    case 'connection_resumed':
      return { ...state, connectionBanner: 'resumed' };

    default:
      return state;
  }
}

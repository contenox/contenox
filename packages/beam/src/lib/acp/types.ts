/**
 * Wire types for the Agent Client Protocol (ACP), mirrored field-for-field from
 * the Go reference implementation in `libacp` (see `/libacp/*.go` at the repo
 * root). Field names are camelCase to match the JSON that travels the wire —
 * do not rename them to "look more TypeScript-y".
 *
 * This module knows nothing about contenox, beam, or any specific agent. It is
 * a plain description of the protocol and may be lifted into its own package
 * unchanged.
 *
 * As of Stage 2.5, this module leans on the official `@agentclientprotocol/sdk`
 * (pinned in package.json) where it safely can — see client.ts's header
 * comment for the full account of what the SDK owns vs. what stays local, and
 * why (short version: the SDK's `Stream`/`Connection`/`ClientApp` engine is
 * incompatible with two hard constraints this client must meet — synchronous
 * send-on-call, and libacp's actual wire traffic being more lenient than the
 * SDK's generated schema marks as "required" — so the JSON-RPC dispatch loop
 * stays hand-rolled in client.ts; the SDK is a compile-time-only dependency —
 * a handful of byte-identical leaf enums are re-exported as types here, at
 * zero bundle cost). The JSON-RPC
 * envelope types (`JsonRpcRequest`/`JsonRpcNotification`/`JsonRpcResponse`)
 * that used to live in this file are gone — they're now private to client.ts.
 */
import type {
  PermissionOptionKind as SdkPermissionOptionKind,
  PlanEntryPriority as SdkPlanEntryPriority,
  PlanEntryStatus as SdkPlanEntryStatus,
  StopReason as SdkStopReason,
  ToolCallStatus as SdkToolCallStatus,
} from '@agentclientprotocol/sdk';

/** The ACP protocol version this client speaks (libacp.ProtocolVersion). */
export const ACP_PROTOCOL_VERSION = 1;

// ---------------------------------------------------------------------------
// initialize
// ---------------------------------------------------------------------------

export interface Implementation {
  name: string;
  title?: string;
  version?: string;
}

export interface FileSystemCapabilities {
  readTextFile?: boolean;
  writeTextFile?: boolean;
}

/** Gates which auth method `type`s the client can handle (unstable spec surface). */
export interface AuthCapabilities {
  terminal?: boolean;
}

export interface SessionConfigOptionsCapabilities {
  /** Presence (even `{}`) means the client accepts type:"boolean" config options. */
  boolean?: Record<string, never>;
}

export interface ClientSessionCapabilities {
  configOptions?: SessionConfigOptionsCapabilities;
}

export interface ClientCapabilities {
  fs?: FileSystemCapabilities;
  terminal?: boolean;
  session?: ClientSessionCapabilities;
  auth?: AuthCapabilities;
}

export interface PromptCapabilities {
  image?: boolean;
  audio?: boolean;
  embeddedContext?: boolean;
}

export interface McpCapabilities {
  http?: boolean;
  sse?: boolean;
}

export interface SessionCapabilities {
  list?: Record<string, never>;
  resume?: Record<string, never>;
  close?: Record<string, never>;
  delete?: Record<string, never>;
}

export interface AgentCapabilities {
  loadSession?: boolean;
  promptCapabilities?: PromptCapabilities;
  mcpCapabilities?: McpCapabilities;
  sessionCapabilities?: SessionCapabilities;
}

export type AuthMethodType = 'terminal' | 'env_var' | '';

export interface AuthEnvVar {
  name: string;
  label?: string;
  secret?: boolean;
  optional?: boolean;
}

export interface AuthMethod {
  id: string;
  name: string;
  description?: string;
  type?: AuthMethodType;
  args?: string[];
  env?: Record<string, string>;
  vars?: AuthEnvVar[];
  link?: string;
}

export interface InitializeResponse {
  protocolVersion: number;
  agentCapabilities?: AgentCapabilities;
  agentInfo?: Implementation;
  authMethods?: AuthMethod[];
  /** Spec-reserved extension namespace — see `WORKSPACE_CONFIG_OPTIONS_META_KEY`. */
  _meta?: Record<string, unknown>;
}

/**
 * The `_meta` key under which a contenox agent advertises the workspace-level
 * (session-less) config options in its `initialize` response — mirrored from
 * `acpsvc.WorkspaceConfigOptionsMetaKey`. Sessions are minted lazily (on the
 * first prompt), so a fresh chat has no session and thus none of the
 * per-session config options that normally arrive with `session/new`. This
 * extension lets the client render the model/think/HITL/token-limit controls
 * on the empty chat, stage the user's choices, and re-apply them via
 * `set_config_option` right after `session/new`. A contenox extension in the
 * spec's reserved `_meta` namespace: non-contenox agents simply omit it.
 */
export const WORKSPACE_CONFIG_OPTIONS_META_KEY = 'contenox.workspaceConfigOptions';

/**
 * Extracts the workspace-level config options from an `initialize` response's
 * `_meta` (see `WORKSPACE_CONFIG_OPTIONS_META_KEY`). Returns `[]` for any
 * agent that doesn't advertise the extension, or malformed payloads — the
 * empty-chat controls degrade to nothing rather than throwing.
 */
export function workspaceConfigOptionsFromInit(init: InitializeResponse): SessionConfigOption[] {
  const raw = init._meta?.[WORKSPACE_CONFIG_OPTIONS_META_KEY];
  if (!Array.isArray(raw)) return [];
  return raw.filter(
    (o): o is SessionConfigOption =>
      typeof o === 'object' && o !== null && typeof (o as SessionConfigOption).id === 'string',
  );
}

export interface AuthenticateResponse {
  _meta?: unknown;
}

// ---------------------------------------------------------------------------
// content blocks (prompt input + streamed output)
// ---------------------------------------------------------------------------

export type ContentBlockKind = 'text' | 'image' | 'audio' | 'resource' | 'resource_link';

export interface EmbeddedResource {
  uri: string;
  mimeType?: string;
  text?: string;
  blob?: string;
}

export interface Annotations {
  audience?: string[];
  priority?: number;
}

export interface ContentBlock {
  type: ContentBlockKind | string;
  text?: string;
  data?: string;
  mimeType?: string;
  uri?: string;
  name?: string;
  title?: string;
  description?: string;
  size?: number;
  resource?: EmbeddedResource;
  annotations?: Annotations;
}

export function textContent(text: string): ContentBlock {
  return { type: 'text', text };
}

// ---------------------------------------------------------------------------
// tool calls
// ---------------------------------------------------------------------------

export type ToolKind =
  | 'read'
  | 'edit'
  | 'delete'
  | 'move'
  | 'search'
  | 'execute'
  | 'think'
  | 'fetch'
  | 'other';

/** Byte-identical to `@agentclientprotocol/sdk`'s `ToolCallStatus` — re-exported rather than redeclared. */
export type ToolCallStatus = SdkToolCallStatus;

export interface ToolCallLocation {
  path: string;
  line?: number;
}

export type ToolCallContentKind = 'content' | 'diff' | 'terminal';

export interface ToolCallContent {
  type: ToolCallContentKind;
  content?: ContentBlock;
  path?: string;
  oldText?: string;
  newText?: string;
  terminalId?: string;
}

// ---------------------------------------------------------------------------
// plan / available commands
// ---------------------------------------------------------------------------

/** Byte-identical to the SDK's `PlanEntryPriority`/`PlanEntryStatus` — re-exported rather than redeclared. */
export type PlanEntryPriority = SdkPlanEntryPriority;
export type PlanEntryStatus = SdkPlanEntryStatus;

export interface PlanEntry {
  content: string;
  priority: PlanEntryPriority;
  status: PlanEntryStatus;
}

export interface AvailableCommandInput {
  hint?: string;
}

export interface AvailableCommand {
  name: string;
  description: string;
  input?: AvailableCommandInput;
}

// ---------------------------------------------------------------------------
// sessions
// ---------------------------------------------------------------------------

export type SessionId = string;

export interface EnvVariable {
  name: string;
  value: string;
}

export interface HttpHeader {
  name: string;
  value: string;
}

export type McpServerKind = '' | 'http' | 'sse';

export interface McpServer {
  type?: McpServerKind;
  name: string;
  command?: string;
  args?: string[];
  env?: EnvVariable[];
  url?: string;
  headers?: HttpHeader[];
}

export interface SessionMode {
  id: string;
  name: string;
  description?: string;
}

export interface SessionModeState {
  currentModeId: string;
  availableModes: SessionMode[];
}

export interface SessionConfigValue {
  value: string;
  name: string;
  description?: string;
}

export interface SessionConfigGroup {
  group: string;
  name: string;
  options: SessionConfigValue[];
}

/** `SessionConfigOption.options` is either a flat value list or grouped values. */
export type SessionConfigOptionValues = SessionConfigValue[] | SessionConfigGroup[];

export interface SessionConfigOption {
  id: string;
  name: string;
  description?: string;
  category?: string;
  type: string;
  currentValue: string;
  options: SessionConfigOptionValues;
}

export interface NewSessionResponse {
  sessionId: SessionId;
  modes?: SessionModeState;
  configOptions?: SessionConfigOption[];
}

export interface LoadSessionResponse {
  modes?: SessionModeState;
  configOptions?: SessionConfigOption[];
}

export interface ResumeSessionResponse {
  modes?: SessionModeState;
  configOptions?: SessionConfigOption[];
}

/** The value union of session/set_config_option: a string value id, or a boolean. */
export type SessionConfigOptionValue = string | boolean;

export interface SetSessionConfigOptionResponse {
  configOptions: SessionConfigOption[];
}

export interface SessionInfo {
  sessionId: SessionId;
  cwd?: string;
  title?: string;
  updatedAt?: string;
}

export interface ListSessionsResponse {
  sessions: SessionInfo[];
  nextCursor?: string;
}

// ---------------------------------------------------------------------------
// session/prompt
// ---------------------------------------------------------------------------

/** Byte-identical to the SDK's `StopReason` — re-exported rather than redeclared. */
export type StopReason = SdkStopReason;

// PromptResponse mirrors the ACP v1 schema: only `stopReason` and `_meta`
// (the latter omitted here, unused by this client). A prior revision typed a
// non-spec `usage` field here; it was removed — nothing produced or
// consumed it (see libacp/prompt.go's PromptResponse doc comment for the
// full rationale). Session context/cost reporting uses the spec-sanctioned
// `usage_update` channel instead (see UsageEvent in client.ts).
export interface PromptResponse {
  stopReason: StopReason;
}

export interface UsageCost {
  amount: number;
  currency: string;
}

// ---------------------------------------------------------------------------
// session/update (server -> client notification)
// ---------------------------------------------------------------------------

export type SessionUpdateKind =
  | 'user_message_chunk'
  | 'agent_message_chunk'
  | 'agent_thought_chunk'
  | 'tool_call'
  | 'tool_call_update'
  | 'plan'
  | 'available_commands_update'
  | 'current_mode_update'
  | 'config_option_update'
  | 'usage_update'
  | 'session_info_update'
  | typeof TERMINAL_OUTPUT_UPDATE_KIND;

/**
 * Extension `session/update` discriminator (underscore-prefixed = extension)
 * that carries live shell-session output in its `_meta` under
 * {@link TERMINAL_OUTPUT_META_KEY}. Mirrors `acpsvc.TerminalOutputUpdateKind`. A
 * conformant non-contenox client that doesn't know the kind ignores the update.
 */
export const TERMINAL_OUTPUT_UPDATE_KIND = '_contenox.terminalOutput';

/** The `_meta` key carrying a {@link TerminalOutputPayload}. Mirrors `acpsvc.TerminalOutputMetaKey`. */
export const TERMINAL_OUTPUT_META_KEY = 'contenox.terminalOutput';

/** Payload streamed under {@link TERMINAL_OUTPUT_META_KEY} on each shell output batch. */
export interface TerminalOutputPayload {
  sessionId: string;
  /** Absolute scrollback offset where `chunk` begins. */
  offset: number;
  chunk: string;
  /** True for the initial snapshot on (re)subscribe: replace the buffer, don't append. */
  reset?: boolean;
}

/** Extracts the terminal-output payload from an extension `session/update`, or null. */
export function terminalOutputFromUpdate(update: {
  _meta?: Record<string, unknown>;
}): TerminalOutputPayload | null {
  const raw = update._meta?.[TERMINAL_OUTPUT_META_KEY];
  if (raw === null || typeof raw !== 'object') return null;
  const p = raw as Partial<TerminalOutputPayload>;
  if (typeof p.chunk !== 'string') return null;
  return {
    sessionId: typeof p.sessionId === 'string' ? p.sessionId : '',
    offset: typeof p.offset === 'number' ? p.offset : 0,
    chunk: p.chunk,
    reset: p.reset === true,
  };
}

interface ToolCallFields {
  toolCallId: string;
  title?: string;
  kind?: ToolKind;
  status?: ToolCallStatus;
  content?: ToolCallContent[];
  locations?: ToolCallLocation[];
  rawInput?: unknown;
  rawOutput?: unknown;
}

/**
 * Discriminated union on `sessionUpdate`. Each variant carries exactly the
 * wire fields libacp's `SessionUpdate.MarshalJSON`/`UnmarshalJSON` puts on the
 * wire for that kind (see libacp/prompt.go).
 */
export type SessionUpdate =
  | ({ sessionUpdate: 'user_message_chunk'; content: ContentBlock; messageId?: string })
  | ({ sessionUpdate: 'agent_message_chunk'; content: ContentBlock; messageId?: string })
  | ({ sessionUpdate: 'agent_thought_chunk'; content: ContentBlock; messageId?: string })
  | ({ sessionUpdate: 'tool_call' } & ToolCallFields)
  | ({ sessionUpdate: 'tool_call_update' } & ToolCallFields)
  | { sessionUpdate: 'plan'; entries: PlanEntry[] }
  | { sessionUpdate: 'available_commands_update'; availableCommands: AvailableCommand[] }
  | { sessionUpdate: 'current_mode_update'; currentModeId: string }
  | { sessionUpdate: 'config_option_update'; configOptions: SessionConfigOption[] }
  | { sessionUpdate: 'usage_update'; used: number; size: number; cost?: UsageCost }
  | { sessionUpdate: 'session_info_update'; title?: string; updatedAt?: string }
  | { sessionUpdate: typeof TERMINAL_OUTPUT_UPDATE_KIND; _meta?: Record<string, unknown> };

export interface SessionNotification {
  sessionId: SessionId;
  update: SessionUpdate;
}

// ---------------------------------------------------------------------------
// session/request_permission (server -> client request)
// ---------------------------------------------------------------------------

/** Byte-identical to the SDK's `PermissionOptionKind` — re-exported rather than redeclared. */
export type PermissionOptionKind = SdkPermissionOptionKind;

export interface PermissionOption {
  optionId: string;
  name: string;
  kind: PermissionOptionKind;
}

export interface PermissionToolCall {
  toolCallId: string;
  title?: string;
  kind?: ToolKind;
  status?: ToolCallStatus;
  content?: ToolCallContent[];
  locations?: ToolCallLocation[];
  rawInput?: unknown;
  rawOutput?: unknown;
}

export interface RequestPermissionRequest {
  sessionId: SessionId;
  toolCall: PermissionToolCall;
  options: PermissionOption[];
}

export type RequestPermissionOutcome =
  | { outcome: 'cancelled' }
  | { outcome: 'selected'; optionId: string };

export interface RequestPermissionResponse {
  outcome: RequestPermissionOutcome;
}

// ---------------------------------------------------------------------------
// fs/* and terminal/* (server -> client requests; this client answers with a
// "not supported" error — see client.ts — but the shapes are recorded here
// for completeness / future use).
// ---------------------------------------------------------------------------

export interface ReadTextFileRequest {
  sessionId: SessionId;
  path: string;
  line?: number;
  limit?: number;
}

export interface ReadTextFileResponse {
  content: string;
}

export interface WriteTextFileRequest {
  sessionId: SessionId;
  path: string;
  content: string;
}

export interface CreateTerminalRequest {
  sessionId: SessionId;
  command: string;
  args?: string[];
  env?: EnvVariable[];
  cwd?: string;
  outputByteLimit?: number;
}

export interface TerminalExitStatus {
  exitCode?: number;
  signal?: string;
}

export interface TerminalOutputRequest {
  sessionId: SessionId;
  terminalId: string;
}

export interface WaitForTerminalExitRequest {
  sessionId: SessionId;
  terminalId: string;
}

export interface KillTerminalRequest {
  sessionId: SessionId;
  terminalId: string;
}

export interface ReleaseTerminalRequest {
  sessionId: SessionId;
  terminalId: string;
}

// ---------------------------------------------------------------------------
// JSON-RPC error codes (libacp/errors.go)
// ---------------------------------------------------------------------------

export const JSON_RPC_ERROR_CODES = {
  parseError: -32700,
  invalidRequest: -32600,
  methodNotFound: -32601,
  invalidParams: -32602,
  internalError: -32603,
  authRequired: -32000,
  resourceNotFound: -32002,
} as const;

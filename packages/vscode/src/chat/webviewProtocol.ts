// Wire contract between the extension host (ChatWebviewViewProvider) and the
// bundled webview script (webview-src/chat-entry.tsx). Kept independent of
// packages/beam's types since the two sides compile separately (tsc vs esbuild).

export type WireSession = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  lastMessageAt?: string | null;
};

export type WireMessageRole = "system" | "user" | "assistant" | "tool";

export type WireCitation = {
  title?: string;
  source?: string;
  url?: string;
  path?: string;
};

export type WireToolCall = {
  id: string;
  title?: string;
  status: string;
  toolName?: string;
  output?: string;
  error?: string;
  diff?: { path?: string; before?: string; after?: string };
};

export type WireMessage = {
  id: string;
  sessionId: string;
  role: WireMessageRole;
  content: string;
  createdAt: string;
  citations?: WireCitation[];
  toolCalls?: WireToolCall[];
  error?: string;
};

export type WireSessionResponse = {
  session?: WireSession;
  messages?: WireMessage[];
};

export type WireTool = {
  id: string;
  label: string;
  mode: string;
  enabled: boolean;
};

export type WireRuntimeSummary = {
  provider?: string;
  model?: string;
  think?: string;
  hitlPolicy?: string;
  connected: boolean;
  // context usage indicator (from engine token events + session effective token_limit / chain budget)
  // size = the controllable session context budget (capped by model if reported >0)
  contextUsed?: number;
  contextSize?: number;
};

export type WireApprovalOption = {
  id: string;
  label: string;
  kind: string;
};

export type WireApprovalRequest = {
  approvalId: string;
  title: string;
  toolName?: string;
  details?: string;
  diff?: { path?: string; before?: string; after?: string };
  options: WireApprovalOption[];
};

export type ChatWebviewToHostMessage =
  | { type: "ready" }
  | { type: "listSessions"; requestId: string }
  | { type: "createSession"; requestId: string; title: string }
  | { type: "getSession"; requestId: string; id: string }
  | { type: "renameSession"; requestId: string; id: string; title: string }
  | { type: "deleteSession"; requestId: string; id: string }
  | { type: "sendMessage"; requestId: string; id: string; content: string }
  | { type: "cancelTurn"; id: string }
  | { type: "listTools"; requestId: string }
  | { type: "approvalResponse"; requestId: string; optionId?: string }
  | { type: "openDiff"; call: WireToolCall }
  | { type: "confirmDelete"; requestId: string; id: string; title: string }
  | { type: "promptRename"; requestId: string; id: string; title: string }
  | { type: "getRuntimeSummary"; requestId: string }
  | { type: "openRuntimeSettings" };

export type ChatHostToWebviewMessage =
  | { type: "result"; requestId: string; ok: true; value: unknown }
  | { type: "result"; requestId: string; ok: false; error: string }
  | { type: "delta"; requestId: string; content?: string; thinking?: string }
  | { type: "toolCall"; requestId: string; call: WireToolCall }
  | { type: "approvalRequest"; requestId: string; request: WireApprovalRequest }
  | { type: "composerAction"; nonce: string; content: string; submit: boolean }
  | { type: "selectSession"; id: string }
  | { type: "runtimeConfig"; summary: WireRuntimeSummary };

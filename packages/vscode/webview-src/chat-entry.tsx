import { createRoot } from "react-dom/client";
import React, { useCallback, useEffect, useState } from "react";
import {
  BeamChat,
  BeamChatClient,
  BeamChatComposerAction,
  BeamChatReadiness,
  BeamChatRuntimeSummary,
  BeamChatSession,
} from "../../beam/src/chat";
import type {
  ChatHostToWebviewMessage,
  ChatWebviewToHostMessage,
  WireRuntimeSummary,
  WireToolCall,
} from "../src/chat/webviewProtocol";

declare function acquireVsCodeApi(): { postMessage: (message: unknown) => void };

const vscode = acquireVsCodeApi();
const PRODUCT_NAME = "Contenox";

// Branded Contenox icon for empty states and assistant avatar contexts.
const ContenoxIcon: React.FC<{ className?: string }> = ({ className }) => (
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 500 500"
    className={className}
    aria-hidden="true"
  >
    <path
      fill="#6366f1"
      d="M207.28 164.77h170.47V79.81c-54.58 0-106.94 21.64-145.6 60.17l-24.87 24.79Z"
    />
    <path
      fill="#7c3aed"
      d="M207.21 164.77v170.47h-84.96c0-54.58 21.64-106.94 60.17-145.6l24.79-24.87Z"
    />
    <path
      fill="#8b5cf6"
      d="M207.21 335.23h170.47v84.96c-54.58 0-106.94-21.64-145.6-60.17l-24.87-24.79Z"
    />
  </svg>
);

let requestSeq = 0;
function nextRequestId(): string {
  requestSeq += 1;
  return `req-${requestSeq}-${Date.now()}`;
}

type ResultWaiter = { resolve: (value: unknown) => void; reject: (error: Error) => void };
type TurnListener = {
  onDelta?: (chunk: { content?: string; thinking?: string }) => void;
  onToolCall?: (call: WireToolCall) => void;
  onApprovalRequest?: (requestId: string, request: ChatHostToWebviewMessage & { type: "approvalRequest" }) => void;
};

const waiters = new Map<string, ResultWaiter>();
const turnListeners = new Map<string, TurnListener>();

function send(message: ChatWebviewToHostMessage): void {
  vscode.postMessage(message);
}

function request<T>(message: Extract<ChatWebviewToHostMessage, { requestId: string }>): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    waiters.set(message.requestId, { resolve: resolve as (value: unknown) => void, reject });
    send(message);
  });
}

type ExternalHandlers = {
  onSelectSession?: (id: string) => void;
  onComposerAction?: (action: BeamChatComposerAction) => void;
  onRuntimeConfig?: (summary: BeamChatRuntimeSummary) => void;
};

const externalHandlers: ExternalHandlers = {};

function toRuntimeSummary(summary: WireRuntimeSummary): BeamChatRuntimeSummary {
  return {
    provider: summary.provider,
    model: summary.model,
    think: summary.think,
    hitlPolicy: summary.hitlPolicy,
    connected: summary.connected,
    contextUsed: summary.contextUsed,
    contextSize: summary.contextSize,
  };
}

window.addEventListener("message", (event: MessageEvent<ChatHostToWebviewMessage>) => {
  const message = event.data;
  switch (message.type) {
    case "result": {
      const waiter = waiters.get(message.requestId);
      if (!waiter) return;
      waiters.delete(message.requestId);
      if (message.ok) {
        waiter.resolve(message.value);
      } else {
        waiter.reject(new Error(message.error));
      }
      return;
    }
    case "delta": {
      turnListeners.get(message.requestId)?.onDelta?.({ content: message.content, thinking: message.thinking });
      return;
    }
    case "toolCall": {
      turnListeners.get(message.requestId)?.onToolCall?.(message.call);
      return;
    }
    case "approvalRequest": {
      turnListeners.get(message.requestId)?.onApprovalRequest?.(message.requestId, message);
      return;
    }
    case "composerAction": {
      externalHandlers.onComposerAction?.({
        nonce: message.nonce,
        content: message.content,
        submit: message.submit,
      });
      return;
    }
    case "selectSession": {
      externalHandlers.onSelectSession?.(message.id);
      return;
    }
    case "runtimeConfig": {
      externalHandlers.onRuntimeConfig?.(toRuntimeSummary(message.summary));
      return;
    }
  }
});

const client: BeamChatClient = {
  listSessions: () => request({ type: "listSessions", requestId: nextRequestId() }),
  createSession: (input) => request({ type: "createSession", requestId: nextRequestId(), title: input.title }),
  getSession: (id) => request({ type: "getSession", requestId: nextRequestId(), id }),
  deleteSession: (id) => request({ type: "deleteSession", requestId: nextRequestId(), id }),
  listTools: () => request({ type: "listTools", requestId: nextRequestId() }),
  cancelTurn: (id) => send({ type: "cancelTurn", id }),
  openDiff: (call) => send({ type: "openDiff", call }),
  sendMessage: (id, input, handlers) => {
    const requestId = nextRequestId();
    turnListeners.set(requestId, {
      onDelta: handlers?.onDelta,
      onToolCall: handlers?.onToolCall,
      onApprovalRequest: (approvalRequestId, message) => {
        void handlers?.onApprovalRequest?.(message.request).then((optionId) => {
          send({ type: "approvalResponse", requestId: approvalRequestId, optionId });
        });
      },
    });
    return request({ type: "sendMessage", requestId, id, content: input.content }).finally(() => {
      turnListeners.delete(requestId);
    });
  },
};

const readiness: BeamChatReadiness = {
  aiReady: true, // default; will be overridden from runtimeConfig if available
  appCount: 0,
  canManage: false,
  enabledToolCount: 0,
  searchReady: false,
  sourceCount: 0,
  syncedSourceCount: 0,
};

function App() {
  const [composerAction, setComposerAction] = useState<BeamChatComposerAction | null>(null);
  const [selectSessionId, setSelectSessionId] = useState<string | null>(null);
  const [runtimeSummary, setRuntimeSummary] = useState<BeamChatRuntimeSummary | null>(null);
  const [aiReady, setAiReady] = useState<boolean>(true);

  useEffect(() => {
    externalHandlers.onComposerAction = setComposerAction;
    externalHandlers.onSelectSession = setSelectSessionId;
    externalHandlers.onRuntimeConfig = (s) => {
      setRuntimeSummary(s);
      // Compute real readiness: needs provider + model + connected (configured preferred)
      const ready = !!(s.provider && s.model && s.connected && (s.configured ?? true));
      setAiReady(ready);
    };
    send({ type: "ready" });
    void request<WireRuntimeSummary>({ type: "getRuntimeSummary", requestId: nextRequestId() })
      .then((summary) => {
        const s = toRuntimeSummary(summary);
        setRuntimeSummary(s);
        const ready = !!(summary.provider && summary.model && summary.connected && (summary.configured ?? true));
        setAiReady(ready);
      })
      .catch(() => undefined);
  }, []);

  const confirmDeleteSession = useCallback(async (session: BeamChatSession) => {
    return request<boolean>({
      type: "confirmDelete",
      requestId: nextRequestId(),
      id: session.id,
      title: session.title,
    });
  }, []);

  const promptRenameSession = useCallback(async (session: BeamChatSession, currentTitle: string) => {
    return request<string | undefined>({
      type: "promptRename",
      requestId: nextRequestId(),
      id: session.id,
      title: currentTitle,
    });
  }, []);

  const openRuntimeSettings = useCallback(() => {
    send({ type: "openRuntimeSettings" });
  }, []);

  const dynamicReadiness: BeamChatReadiness = { ...readiness, aiReady };

  return (
    <BeamChat
      client={client}
      readiness={dynamicReadiness}
      embedded
      productName={PRODUCT_NAME}
      productIcon={<ContenoxIcon className="h-8 w-8 opacity-70" />}
      composerAction={composerAction}
      onComposerActionHandled={() => setComposerAction(null)}
      selectSessionId={selectSessionId}
      confirmDeleteSession={confirmDeleteSession}
      promptRenameSession={promptRenameSession}
      runtimeSummary={runtimeSummary}
      onOpenRuntimeSettings={openRuntimeSettings}
    />
  );
}

const container = document.getElementById("root");
if (container) {
  createRoot(container).render(<App />);
}
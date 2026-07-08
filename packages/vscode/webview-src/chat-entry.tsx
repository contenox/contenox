import { createRoot } from "react-dom/client";
import { useEffect, useState } from "react";
import { BeamChat, BeamChatClient, BeamChatComposerAction, BeamChatReadiness } from "../../beam/src/chat";
import type {
  ChatHostToWebviewMessage,
  ChatWebviewToHostMessage,
  WireToolCall,
} from "../src/chat/webviewProtocol";

declare function acquireVsCodeApi(): { postMessage: (message: unknown) => void };

const vscode = acquireVsCodeApi();

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
};

const externalHandlers: ExternalHandlers = {};

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
  aiReady: true,
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

  useEffect(() => {
    externalHandlers.onComposerAction = setComposerAction;
    externalHandlers.onSelectSession = setSelectSessionId;
    send({ type: "ready" });
  }, []);

  return (
    <BeamChat
      client={client}
      readiness={readiness}
      embedded
      composerAction={composerAction}
      onComposerActionHandled={() => setComposerAction(null)}
      selectSessionId={selectSessionId}
    />
  );
}

const container = document.getElementById("root");
if (container) {
  createRoot(container).render(<App />);
}

import * as assert from "node:assert/strict";
import * as vscode from "vscode";
import type { BridgeProcess } from "../bridge/BridgeProcess";
import { ChatWebviewViewProvider } from "../chat/ChatWebviewViewProvider";
import type { ChatHostToWebviewMessage, ChatWebviewToHostMessage } from "../chat/webviewProtocol";
import { DiffStore } from "../editor/diffStore";
import { ContenoxOutput } from "../logging/output";
import { TelemetryLogger } from "../logging/telemetry";

type Listener<T> = (event: T) => void;

class FakeBridgeClient {
  private readonly listeners = new Map<string, Set<Listener<unknown>>>();
  private readonly permissionHandlers = new Map<string, (params: unknown, token: vscode.CancellationToken) => Promise<unknown>>();
  public cancelledTurnIds: string[] = [];

  public onChatStarted(listener: Listener<unknown>): vscode.Disposable {
    return this.on("started", listener);
  }
  public onChatDelta(listener: Listener<unknown>): vscode.Disposable {
    return this.on("delta", listener);
  }
  public onToolCall(listener: Listener<unknown>): vscode.Disposable {
    return this.on("toolCall", listener);
  }
  public onChatCompleted(listener: Listener<unknown>): vscode.Disposable {
    return this.on("completed", listener);
  }
  public onChatFailed(listener: Listener<unknown>): vscode.Disposable {
    return this.on("failed", listener);
  }
  public onChatCancelled(listener: Listener<unknown>): vscode.Disposable {
    return this.on("cancelled", listener);
  }
  public pushPermissionRequestHandler(
    sessionId: string,
    handler: (params: unknown, token: vscode.CancellationToken) => Promise<unknown>,
  ): vscode.Disposable {
    this.permissionHandlers.set(sessionId, handler);
    return { dispose: () => this.permissionHandlers.delete(sessionId) };
  }
  public async chatSend(params: { sessionId?: string; input: string }): Promise<{ sessionId: string; turnId: string; title?: string }> {
    return { sessionId: params.sessionId ?? "session-1", turnId: "turn-1" };
  }
  public async chatCancel(params: { turnId: string }): Promise<{ cancelled: boolean }> {
    this.cancelledTurnIds.push(params.turnId);
    return { cancelled: true };
  }
  public async sessionList() {
    return { sessions: [] };
  }
  public async sessionCreate(params: { name?: string }) {
    return { session: { id: "session-1", name: params.name ?? "session-1", messageCount: 0, isActive: true }, messages: [] };
  }
  public async sessionLoad(params: { sessionId?: string }) {
    return { session: { id: params.sessionId ?? "session-1", name: "session-1", messageCount: 0, isActive: true }, messages: [] };
  }
  public async sessionDelete() {
    return { deleted: true, wasActive: true };
  }

  public hasListener(kind: string): boolean {
    return (this.listeners.get(kind)?.size ?? 0) > 0;
  }

  public emit(kind: string, event: unknown): void {
    for (const listener of this.listeners.get(kind) ?? []) {
      listener(event);
    }
  }

  public async triggerPermission(sessionId: string, params: unknown, token: vscode.CancellationToken): Promise<unknown> {
    const handler = this.permissionHandlers.get(sessionId);
    assert.ok(handler, `expected a permission handler for ${sessionId}`);
    return handler(params, token);
  }

  private on(kind: string, listener: Listener<unknown>): vscode.Disposable {
    const set = this.listeners.get(kind) ?? new Set();
    set.add(listener);
    this.listeners.set(kind, set);
    return { dispose: () => set.delete(listener) };
  }
}

function fakeWebviewView(receive: (cb: (message: ChatWebviewToHostMessage) => void) => void) {
  const posted: ChatHostToWebviewMessage[] = [];
  const view = {
    webview: {
      options: {},
      html: "",
      cspSource: "https://webview.test",
      asWebviewUri: (uri: vscode.Uri) => uri,
      postMessage: async (message: ChatHostToWebviewMessage) => {
        posted.push(message);
        return true;
      },
      onDidReceiveMessage: (cb: (message: ChatWebviewToHostMessage) => void) => {
        receive(cb);
        return { dispose: () => undefined };
      },
    },
  } as unknown as vscode.WebviewView;
  return { view, posted };
}

async function eventually(predicate: () => boolean): Promise<void> {
  const deadline = Date.now() + 1000;
  while (Date.now() < deadline) {
    if (predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
  assert.ok(predicate(), "condition was not met before timeout");
}

function setup() {
  const client = new FakeBridgeClient();
  const bridge = {
    ensureStarted: async () => ({ initialize: { capabilities: { chat: true, sessionList: true } } }),
    currentClient: client,
  } as unknown as BridgeProcess;
  const telemetry = new TelemetryLogger("test");
  const output = new ContenoxOutput();
  const diffStore = new DiffStore(telemetry);
  const provider = new ChatWebviewViewProvider(
    bridge,
    diffStore,
    vscode.Uri.file("/tmp/contenox-test"),
    output,
    telemetry,
    () => undefined,
  );
  return {
    client,
    provider,
    teardown: () => {
      provider.dispose();
      diffStore.dispose();
      telemetry.dispose();
      output.dispose();
    },
  };
}

suite("ChatWebviewViewProvider", () => {
  test("sendMessage streams deltas and tool calls before resolving", async () => {
    const { client, provider, teardown } = setup();
    let receiveMessage: ((message: ChatWebviewToHostMessage) => void) | undefined;
    const { view, posted } = fakeWebviewView((cb) => (receiveMessage = cb));
    provider.resolveWebviewView(view);
    assert.ok(receiveMessage, "onDidReceiveMessage should be registered");

    try {
      receiveMessage!({ type: "sendMessage", requestId: "req-1", id: "session-1", content: "hello" });
      await eventually(() => client.hasListener("delta"));

      client.emit("delta", { sessionId: "session-1", turnId: "turn-1", content: "Hi " });
      client.emit("delta", { sessionId: "session-1", turnId: "turn-1", content: "there" });
      client.emit("toolCall", {
        sessionId: "session-1",
        turnId: "turn-1",
        toolCallId: "call-1",
        title: "local_fs.read",
        status: "completed",
        toolName: "read",
      });
      client.emit("completed", {
        sessionId: "session-1",
        turnId: "turn-1",
        messages: [{ role: "assistant", content: "Hi there" }],
      });

      await eventually(() => posted.some((m) => m.type === "result"));

      const deltas = posted.filter((m) => m.type === "delta") as Array<{ type: "delta"; content?: string }>;
      assert.deepEqual(deltas.map((d) => d.content), ["Hi ", "there"]);

      const toolCalls = posted.filter((m) => m.type === "toolCall");
      assert.equal(toolCalls.length, 1);

      const result = posted.find((m) => m.type === "result") as { type: "result"; ok: boolean; value: unknown };
      assert.equal(result.ok, true);
      const value = result.value as { messages: Array<{ role: string; content: string; sessionId: string }> };
      assert.equal(value.messages.length, 1);
      assert.equal(value.messages[0].role, "assistant");
      assert.equal(value.messages[0].content, "Hi there");
      assert.equal(value.messages[0].sessionId, "session-1");
    } finally {
      teardown();
    }
  });

  test("cancelTurn cancels the in-flight bridge turn by session id", async () => {
    const { client, provider, teardown } = setup();
    let receiveMessage: ((message: ChatWebviewToHostMessage) => void) | undefined;
    const { view, posted } = fakeWebviewView((cb) => (receiveMessage = cb));
    provider.resolveWebviewView(view);

    try {
      receiveMessage!({ type: "sendMessage", requestId: "req-2", id: "session-2", content: "hello" });
      await eventually(() => client.hasListener("cancelled"));

      receiveMessage!({ type: "cancelTurn", id: "session-2" });
      await eventually(() => client.cancelledTurnIds.length === 1);
      assert.equal(client.cancelledTurnIds[0], "turn-1");

      client.emit("cancelled", { sessionId: "session-2", turnId: "turn-1", error: "cancelled" });
      await eventually(() => posted.some((m) => m.type === "result" && !m.ok));
    } finally {
      teardown();
    }
  });

  test("approval request round-trips the selected option back to the bridge", async () => {
    const { client, provider, teardown } = setup();
    let receiveMessage: ((message: ChatWebviewToHostMessage) => void) | undefined;
    const { view, posted } = fakeWebviewView((cb) => (receiveMessage = cb));
    provider.resolveWebviewView(view);

    try {
      receiveMessage!({ type: "sendMessage", requestId: "req-3", id: "session-3", content: "run a command" });
      await eventually(() => client.hasListener("completed"));

      const tokenSource = new vscode.CancellationTokenSource();
      const outcomePromise = client.triggerPermission(
        "session-3",
        {
          sessionId: "session-3",
          toolCall: { toolCallId: "call-1", title: "local_shell.local_shell: rm file", status: "pending" },
          options: [
            { optionId: "allow", name: "Allow", kind: "allow_once" },
            { optionId: "deny", name: "Deny", kind: "reject_once" },
          ],
        },
        tokenSource.token,
      );

      await eventually(() => posted.some((m) => m.type === "approvalRequest"));
      const approvalRequest = posted.find((m) => m.type === "approvalRequest") as {
        type: "approvalRequest";
        requestId: string;
        request: { options: Array<{ id: string }> };
      };
      assert.deepEqual(
        approvalRequest.request.options.map((option) => option.id),
        ["allow", "deny"],
      );

      receiveMessage!({ type: "approvalResponse", requestId: approvalRequest.requestId, optionId: "allow" });

      const outcome = await outcomePromise;
      assert.deepEqual(outcome, { outcome: { outcome: "selected", optionId: "allow" } });

      client.emit("completed", { sessionId: "session-3", turnId: "turn-1", messages: [] });
      await eventually(() => posted.some((m) => m.type === "result"));
      tokenSource.dispose();
    } finally {
      teardown();
    }
  });
});

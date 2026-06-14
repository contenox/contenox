import * as vscode from "vscode";
import { BridgeProcess } from "../bridge/BridgeProcess";
import { ConfigSnapshot, SessionInfo, SessionMessage } from "../bridge/protocol";

type SessionTreeNode =
  | { kind: "config"; id: "provider" | "model" | "think" | "hitl"; label: string; description: string; command: string; icon: string }
  | { kind: "session"; session: SessionInfo }
  | { kind: "sessionMessage"; sessionId: string; message: SessionMessage; index: number }
  | { kind: "message"; label: string; description?: string };

const maxPreviewMessages = 8;

export class SessionTreeProvider implements vscode.TreeDataProvider<SessionTreeNode>, vscode.Disposable {
  private readonly changeEmitter = new vscode.EventEmitter<SessionTreeNode | undefined | null | void>();
  public readonly onDidChangeTreeData = this.changeEmitter.event;

  public constructor(private readonly bridge: BridgeProcess) {}

  public refresh(): void {
    this.changeEmitter.fire();
  }

  public async getChildren(element?: SessionTreeNode): Promise<SessionTreeNode[]> {
    if (element?.kind === "session") {
      return this.sessionPreviewChildren(element.session);
    }
    if (element) {
      return [];
    }
    try {
      const state = await this.bridge.ensureStarted();
      if (!state.initialize.capabilities.sessionList) {
        return [{ kind: "message", label: "Sessions are not supported by this Contenox runtime" }];
      }
      const client = this.bridge.currentClient;
      if (!client) {
        return [{ kind: "message", label: "Contenox runtime connection is not available" }];
      }
      const [config, result] = await Promise.all([client.getConfig(), client.sessionList()]);
      const nodes: SessionTreeNode[] = [...configNodes(config)];
      if (result.sessions.length === 0) {
        nodes.push({ kind: "message", label: "No sessions yet" });
        return nodes;
      }
      nodes.push(...result.sessions.map((session) => ({ kind: "session" as const, session })));
      return nodes;
    } catch (error) {
      return [{ kind: "message", label: "Contenox runtime unavailable", description: errorMessage(error) }];
    }
  }

  public getTreeItem(element: SessionTreeNode): vscode.TreeItem {
    if (element.kind === "message") {
      const item = new vscode.TreeItem(element.label, vscode.TreeItemCollapsibleState.None);
      item.description = element.description;
      return item;
    }

    if (element.kind === "config") {
      const item = new vscode.TreeItem(element.label, vscode.TreeItemCollapsibleState.None);
      item.description = element.description;
      item.tooltip = `${element.label}: ${element.description}`;
      item.contextValue = "contenoxConfig";
      item.iconPath = new vscode.ThemeIcon(element.icon);
      item.command = {
        command: element.command,
        title: element.label,
      };
      return item;
    }

    if (element.kind === "sessionMessage") {
      const role = roleLabel(element.message.role);
      const item = new vscode.TreeItem(previewText(element.message), vscode.TreeItemCollapsibleState.None);
      item.description = role;
      item.tooltip = messageTooltip(element.message);
      item.contextValue = "contenoxSessionMessage";
      item.iconPath = new vscode.ThemeIcon(iconForRole(element.message.role));
      return item;
    }

    const { session } = element;
    const item = new vscode.TreeItem(
      session.name || session.id,
      session.messageCount > 0 ? vscode.TreeItemCollapsibleState.Collapsed : vscode.TreeItemCollapsibleState.None,
    );
    item.id = session.id;
    item.description = sessionDescription(session);
    item.tooltip = sessionTooltip(session);
    item.contextValue = "contenoxSession";
    item.iconPath = new vscode.ThemeIcon(session.isActive ? "comment-discussion" : "comment");
    item.command = {
      command: "contenox.openSession",
      title: "Open Contenox Session",
      arguments: [session.id],
    };
    return item;
  }

  public dispose(): void {
    this.changeEmitter.dispose();
  }

  private async sessionPreviewChildren(session: SessionInfo): Promise<SessionTreeNode[]> {
    try {
      const client = this.bridge.currentClient;
      if (!client) {
        return [{ kind: "message", label: "Contenox runtime connection is not available" }];
      }
      const result = await client.sessionRead({ sessionId: session.id });
      const messages = result.messages.filter((message) => Boolean(message.content?.trim() || message.toolCalls?.length));
      if (messages.length === 0) {
        return [{ kind: "message", label: "No messages in this session" }];
      }
      return messages.slice(-maxPreviewMessages).map((message, index) => ({
        kind: "sessionMessage" as const,
        sessionId: session.id,
        message,
        index,
      }));
    } catch (error) {
      return [{ kind: "message", label: "Could not load session", description: errorMessage(error) }];
    }
  }
}

function configNodes(config: ConfigSnapshot): SessionTreeNode[] {
  return [
    {
      kind: "config",
      id: "provider",
      label: "Provider",
      description: config.defaultProvider || "not set",
      command: "contenox.selectProvider",
      icon: "plug",
    },
    {
      kind: "config",
      id: "model",
      label: "Model",
      description: config.defaultModel || "not set",
      command: "contenox.selectChatModel",
      icon: "symbol-method",
    },
    {
      kind: "config",
      id: "think",
      label: "Thinking",
      description: config.defaultThink || "auto",
      command: "contenox.selectThinkLevel",
      icon: "lightbulb",
    },
    {
      kind: "config",
      id: "hitl",
      label: "HITL Policy",
      description: config.hitlPolicyName || "default",
      command: "contenox.selectHitlPolicy",
      icon: "shield",
    },
  ];
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}

function sessionDescription(session: SessionInfo): string {
  const count = `${session.messageCount} ${session.messageCount === 1 ? "message" : "messages"}`;
  const updated = relativeTime(session.updatedAt);
  const parts = [count];
  if (updated) {
    parts.push(updated);
  }
  if (session.isActive) {
    parts.push("active");
  }
  return parts.join(" · ");
}

function sessionTooltip(session: SessionInfo): string {
  const lines = [session.name || session.id, `ID: ${session.id}`, `Messages: ${session.messageCount}`];
  if (session.updatedAt) {
    lines.push(`Updated: ${session.updatedAt}`);
  }
  if (session.isActive) {
    lines.push("Active Contenox session");
  }
  return lines.join("\n");
}

function previewText(message: SessionMessage): string {
  const content = message.content?.trim();
  if (content) {
    return compact(content);
  }
  if (message.toolCalls?.length) {
    return compact(message.toolCalls.map((call) => call.name || call.id || "tool").join(", "));
  }
  return "Message";
}

function messageTooltip(message: SessionMessage): string {
  const parts = [roleLabel(message.role)];
  if (message.timestamp) {
    parts.push(message.timestamp);
  }
  if (message.content?.trim()) {
    parts.push("", message.content.trim());
  }
  return parts.join("\n");
}

function roleLabel(role: string): string {
  switch (role) {
    case "assistant":
      return "assistant";
    case "user":
      return "user";
    case "tool":
      return "tool";
    case "system":
      return "system";
    default:
      return role || "message";
  }
}

function iconForRole(role: string): string {
  switch (role) {
    case "assistant":
      return "sparkle";
    case "user":
      return "account";
    case "tool":
      return "tools";
    case "system":
      return "gear";
    default:
      return "comment";
  }
}

function compact(value: string): string {
  const clean = value.replace(/\s+/g, " ").trim();
  return clean.length > 96 ? `${clean.slice(0, 93)}...` : clean;
}

function relativeTime(value: string | undefined): string | undefined {
  if (!value) {
    return undefined;
  }
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return undefined;
  }
  const diffMs = Date.now() - date.getTime();
  const abs = Math.abs(diffMs);
  const minute = 60 * 1000;
  const hour = 60 * minute;
  const day = 24 * hour;
  if (abs < minute) {
    return "just now";
  }
  if (abs < hour) {
    const minutes = Math.round(abs / minute);
    return `${minutes}m ago`;
  }
  if (abs < day) {
    const hours = Math.round(abs / hour);
    return `${hours}h ago`;
  }
  const days = Math.round(abs / day);
  return `${days}d ago`;
}

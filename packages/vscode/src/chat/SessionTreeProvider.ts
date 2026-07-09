import * as vscode from "vscode";
import { BridgeProcess } from "../bridge/BridgeProcess";
import { SessionInfo } from "../bridge/protocol";

type SessionTreeNode =
  | { kind: "session"; session: SessionInfo }
  | { kind: "message"; label: string; description?: string };

export class SessionTreeProvider implements vscode.TreeDataProvider<SessionTreeNode>, vscode.Disposable {
  private readonly changeEmitter = new vscode.EventEmitter<SessionTreeNode | undefined | null | void>();
  public readonly onDidChangeTreeData = this.changeEmitter.event;

  public constructor(private readonly bridge: BridgeProcess) {}

  public refresh(): void {
    this.changeEmitter.fire();
  }

  public async getChildren(element?: SessionTreeNode): Promise<SessionTreeNode[]> {
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
      const result = await client.sessionList();
      if (result.sessions.length === 0) {
        return [{ kind: "message", label: "No sessions yet. Start chatting in the Chat view." }];
      }
      return result.sessions.map((session) => ({ kind: "session" as const, session }));
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

    const { session } = element;
    const item = new vscode.TreeItem(session.name || session.id, vscode.TreeItemCollapsibleState.None);
    item.id = session.id;
    item.description = sessionDescription(session);
    item.tooltip = `${sessionTooltip(session)}\n\nClick to resume in Chat.`;
    item.contextValue = "contenoxSession";
    item.iconPath = new vscode.ThemeIcon(session.isActive ? "comment-discussion" : "comment");
    item.command = {
      command: "contenox.openSession",
      title: "Resume in Chat",
      arguments: [session.id],
    };
    return item;
  }

  public dispose(): void {
    this.changeEmitter.dispose();
  }
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
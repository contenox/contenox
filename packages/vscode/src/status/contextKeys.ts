import * as vscode from "vscode";

export async function setBridgeContext(connected: boolean, healthy: boolean): Promise<void> {
  await vscode.commands.executeCommand("setContext", "contenox.connected", connected);
  await vscode.commands.executeCommand("setContext", "contenox.bridgeHealthy", healthy);
}

export async function setChatContext(visible: boolean): Promise<void> {
  await vscode.commands.executeCommand("setContext", "contenox.chatVisible", visible);
}

export async function setTurnContext(inProgress: boolean): Promise<void> {
  await vscode.commands.executeCommand("setContext", "contenox.turnInProgress", inProgress);
}

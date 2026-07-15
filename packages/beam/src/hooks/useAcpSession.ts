import { useEffect, useMemo, useRef } from 'react';
import type { PlanEntry, RequestPermissionRequest } from '../lib/acp';
import type { AcpChatMessage, AcpTimelineItem, AcpToolCallState, AcpUsageState } from './acpSessionState';
import type { AcpWorkspaceStatus } from './acpWorkspaceState';
import { useAcpWorkspace } from './useAcpWorkspace';

/**
 * @deprecated Thin single-session adapter kept only so `pages/admin/acpchat/AcpChatPage.tsx`
 * (Stage 3 rebuilds it against `useAcpWorkspace` directly) keeps compiling and
 * behaving the way it did before the workspace layer landed. All protocol/state
 * logic now lives in `useAcpWorkspace.ts` / `acpWorkspaceController.ts` /
 * `acpSessionState.ts` — this file only adapts their multi-session,
 * unified-timeline shape back down to the single-session, flat-lists shape
 * the page was written against:
 *  - `workspace.status` (6 values) collapses to the old 3-value status.
 *  - `session.items`/`session.messages` (unified timeline) splits back into
 *    `messages`/`toolCallOrder`/`toolCalls`.
 *  - Session creation, lazy by default in the workspace layer (D5), is forced
 *    eager here (on first `ready`) to match the old auto-connect UX.
 */

export type AcpConnectionStatus = 'connecting' | 'ready' | 'error';

export interface UseAcpSessionResult {
  status: AcpConnectionStatus;
  error: string | null;
  agentName: string | null;
  sessionId: string | null;
  messages: AcpChatMessage[];
  toolCalls: Record<string, AcpToolCallState>;
  toolCallOrder: string[];
  plan: PlanEntry[];
  usage: AcpUsageState | null;
  pendingPermission: RequestPermissionRequest | null;
  isPrompting: boolean;
  sendPrompt: (text: string) => void;
  respondPermission: (optionId: string) => void;
  cancel: () => void;
}

/** Collapses the workspace's 6-value status onto the page's original 3-value one. Exported (pure, no React) so it can be unit-tested directly — see useAcpSession.test.ts. */
export function mapStatus(workspaceStatus: AcpWorkspaceStatus): AcpConnectionStatus {
  switch (workspaceStatus) {
    case 'ready':
      return 'ready';
    case 'connecting':
    case 'reconnecting':
      return 'connecting';
    default: // 'disconnected' | 'setup_required' | 'error'
      return 'error';
  }
}

/** Un-interleaves the unified timeline's message items back into a flat, arrival-ordered list — the shape the page was written against. Pure, exported for direct testing. */
export function deriveMessages(items: AcpTimelineItem[], messages: Record<string, AcpChatMessage>): AcpChatMessage[] {
  return items
    .filter(item => item.kind === 'message')
    .map(item => messages[item.id])
    .filter((m): m is AcpChatMessage => m !== undefined);
}

/** Un-interleaves the unified timeline's tool-call items back into arrival order. Pure, exported for direct testing. */
export function deriveToolCallOrder(items: AcpTimelineItem[]): string[] {
  return items.filter(item => item.kind === 'tool_call').map(item => item.id);
}

export function useAcpSession(): UseAcpSessionResult {
  const acp = useAcpWorkspace();
  const { workspace, session } = acp;

  // The old hook auto-created one session on mount; the workspace layer's
  // newSession() is deliberately lazy (D5) so callers choose when — this
  // adapter is the caller, forcing it eager exactly once per connection to
  // preserve the page's original UX.
  const createdRef = useRef(false);
  useEffect(() => {
    if (workspace.status === 'ready' && workspace.activeSessionId === null && !createdRef.current) {
      createdRef.current = true;
      void acp.newSession();
    }
    if (workspace.status !== 'ready') {
      createdRef.current = false;
    }
  }, [workspace.status, workspace.activeSessionId, acp]);

  const messages = useMemo(() => deriveMessages(session.items, session.messages), [session.items, session.messages]);

  const toolCallOrder = useMemo(() => deriveToolCallOrder(session.items), [session.items]);

  return {
    status: mapStatus(workspace.status),
    error: workspace.error ?? session.error,
    agentName: workspace.agentName,
    sessionId: session.sessionId,
    messages,
    toolCalls: session.toolCalls,
    toolCallOrder,
    plan: session.plan,
    usage: session.usage,
    pendingPermission: session.pendingPermission,
    isPrompting: session.isPrompting,
    sendPrompt: acp.sendPrompt,
    respondPermission: acp.respondPermission,
    cancel: acp.cancel,
  };
}

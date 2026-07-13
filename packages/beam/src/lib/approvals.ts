import type { PendingApproval } from './taskEvents';
import type { TaskEvent } from './types';

/** Mirrors localtools.DenyMessage (Go) — the tool result text of a denied operation. */
export const DENY_MESSAGE =
  'User denied the operation. Please ask for clarification or try a different, less destructive approach.';

export type ApprovalStatus = 'pending' | 'approved' | 'denied' | 'resolved';

export type ApprovalEntry = {
  approvalId: string;
  toolName: string;
  status: ApprovalStatus;
};

/**
 * Derives the approval history of a run from its retained events.
 *
 * The runtime emits `approval_requested` when a gated tool pauses, but no
 * explicit resolution event: an approved tool simply executes (a `tool_call`
 * event follows), while a denied one yields a `tool_call` whose content is
 * the deny message. A request with no subsequent activity is still pending.
 */
export function deriveApprovals(
  events: TaskEvent[],
  pending: PendingApproval | null,
): ApprovalEntry[] {
  const entries: ApprovalEntry[] = [];

  for (let i = 0; i < events.length; i++) {
    const ev = events[i];
    if (ev.kind !== 'approval_requested') continue;

    const approvalId = ev.approval_id ?? '';
    const toolName = ev.tool_name ?? '';

    if (pending && pending.approvalId === approvalId) {
      entries.push({ approvalId, toolName, status: 'pending' });
      continue;
    }

    let status: ApprovalStatus = 'pending';
    for (let j = i + 1; j < events.length; j++) {
      const later = events[j];
      // tool_call events are namespaced "provider.tool" (e.g. local_shell.local_shell)
      // while approval_requested carries the bare tool name — match loosely.
      const nameMatches =
        !toolName ||
        later.tool_name === toolName ||
        later.tool_name?.endsWith(`.${toolName}`) === true ||
        later.tool_name?.startsWith(`${toolName}.`) === true;
      if (later.kind === 'tool_call' && nameMatches) {
        status = later.content?.trim() === DENY_MESSAGE ? 'denied' : 'approved';
        break;
      }
      // The step moving on without a tool_call still proves resolution.
      if (later.kind === 'step_completed' || later.kind === 'step_failed') {
        status = 'approved';
        break;
      }
      // The chain ending resolves the pause, but without tool evidence the
      // decision itself is unknowable client-side (no resolution event yet).
      if (later.kind === 'chain_completed' || later.kind === 'chain_failed') {
        status = 'resolved';
        break;
      }
    }
    entries.push({ approvalId, toolName, status });
  }

  return entries;
}

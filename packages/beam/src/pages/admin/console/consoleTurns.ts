import { matchesOptimisticEcho } from '../../../lib/optimisticEcho';
import type { ChatMessage } from '../../../lib/types';
import type { OptimisticTurn } from '../../../hooks/useAgentTurn';

/**
 * A console turn: command → work → result. The terminal-agent scrollback unit.
 *
 * `work` holds the intermediate evidence persisted with the thread (assistant
 * tool-call requests, tool results, failure annotations); `result` is the
 * final content-bearing assistant message. Live-run events are NOT part of
 * the turn — the page looks them up in the run log by `requestId`.
 */
export type ConsoleTurn = {
  key: string;
  /** Turn provenance when the messages carry it (post-provenance sessions). */
  requestId?: string;
  chainRef?: string;
  /** The user message that started the turn. Absent for preamble messages. */
  command?: ChatMessage;
  work: ChatMessage[];
  result: ChatMessage | null;
  /** True while this turn's run is still executing. */
  live?: boolean;
};

function isFailureAnnotation(content: string): boolean {
  return /^\[(step|chain) .*\]$/s.test(content.trim());
}

/** True when the assistant message is intermediate work, not the final answer. */
function isWorkMessage(m: ChatMessage): boolean {
  if (m.role === 'tool') return true;
  if (m.role === 'assistant' && m.callTools && m.callTools.length > 0) return true;
  return false;
}

export function buildConsoleTurns(
  messages: ChatMessage[],
  optimistic?: OptimisticTurn | null,
  activeRequestId?: string | null,
): ConsoleTurn[] {
  const turns: ConsoleTurn[] = [];
  let current: ConsoleTurn | null = null;

  const finalize = (turn: ConsoleTurn | null) => {
    if (!turn) return;
    // The last content-bearing assistant message that isn't a tool-call
    // request is the result; everything else stays work.
    for (let i = turn.work.length - 1; i >= 0; i--) {
      const m = turn.work[i];
      if (m.role === 'assistant' && !isWorkMessage(m) && m.content) {
        turn.result = m;
        turn.work.splice(i, 1);
        break;
      }
    }
    turns.push(turn);
  };

  for (const m of messages) {
    if (m.role === 'user') {
      finalize(current);
      current = {
        key: m.requestId || m.id || `turn-${turns.length}-${m.sentAt}`,
        requestId: m.requestId,
        chainRef: m.chainRef,
        command: m,
        work: [],
        result: null,
      };
      continue;
    }
    if (!current) {
      // Preamble (e.g. persisted system/context rows before any user turn).
      current = { key: `preamble-${m.id ?? turns.length}`, work: [], result: null };
    }
    // Adopt provenance from any stamped message in the group (legacy user
    // rows predate stamping but later rows of the same turn may carry it).
    if (!current.requestId && m.requestId) {
      current.requestId = m.requestId;
      current.chainRef = current.chainRef || m.chainRef;
      if (!current.key.startsWith('preamble-')) current.key = m.requestId;
    }
    current.work.push(m);
  }
  finalize(current);

  // Mark the active turn live (matched by provenance or optimistic echo).
  if (activeRequestId) {
    for (const turn of turns) {
      if (turn.requestId === activeRequestId) turn.live = true;
    }
  }

  // Append the optimistic turn unless the persisted history already echoes it.
  // A turn with provenance (own or adopted) matches by requestId only — an
  // earlier run of the same command must not swallow the fresh turn. Unstamped
  // turns fall back to windowed content matching (see matchesOptimisticEcho).
  if (optimistic) {
    const echoed = turns.some(turn =>
      turn.requestId
        ? turn.requestId === optimistic.requestId
        : !!turn.command && matchesOptimisticEcho(turn.command, optimistic),
    );
    if (!echoed) {
      turns.push({
        key: `optimistic-${optimistic.requestId}`,
        requestId: optimistic.requestId,
        command: {
          id: `optimistic-${optimistic.requestId}`,
          role: 'user',
          content: optimistic.content,
          sentAt: optimistic.sentAt,
          isUser: true,
          isLatest: true,
          attachments: optimistic.attachments,
        },
        work: [],
        result: null,
        live: optimistic.requestId === activeRequestId,
      });
    }
  }

  return turns;
}

export { isFailureAnnotation };

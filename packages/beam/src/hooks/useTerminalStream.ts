import { useMemo } from 'react';
import { useAcpWorkspace } from './useAcpWorkspace';
import { EMPTY_SESSION_KEY } from './acpWorkspaceState';
import { splitTerminalLines } from '../pages/chat/lib/terminalPassthrough';

export interface TerminalStream {
  /** Scrollback split into display lines (CRs stripped). */
  lines: string[];
  /** True once any shell output has been received for this session. */
  hasOutput: boolean;
}

/**
 * Reads a SPECIFIC session's live shell scrollback from its multiplexed slice
 * (`sessions.slices[sessionId]`, fed by `_contenox.terminalOutput` stream events
 * → the `terminal_output` reducer action) and presents it as display lines for
 * that session's terminal panel. A single-purpose derived-state hook: the
 * offset/append/reset bookkeeping lives in the reducer, this only splits the
 * accumulated text.
 *
 * The `sessionId` is threaded from the owning `ChatSessionTab` — reading the
 * FOCUSED session instead would show the wrong session's shell inside a
 * backgrounded (but still mounted) tab's terminal panel. `null` is the
 * empty/new-chat surface, whose slice has no terminal output until it spawns a
 * session.
 */
export function useTerminalStream(sessionId: string | null): TerminalStream {
  const { sessions } = useAcpWorkspace();
  const text = sessions.slices[sessionId ?? EMPTY_SESSION_KEY]?.terminal?.text ?? '';
  const lines = useMemo(() => splitTerminalLines(text), [text]);
  return { lines, hasOutput: text.length > 0 };
}

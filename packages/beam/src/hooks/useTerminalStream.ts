import { useMemo } from 'react';
import { useAcpWorkspace } from './useAcpWorkspace';
import { splitTerminalLines } from '../pages/chat/lib/terminalPassthrough';

export interface TerminalStream {
  /** Scrollback split into display lines (CRs stripped). */
  lines: string[];
  /** True once any shell output has been received for the open session. */
  hasOutput: boolean;
}

/**
 * Reads the open session's live shell scrollback from the workspace session
 * state (fed by `_contenox.terminalOutput` stream events → the `terminal_output`
 * reducer action) and presents it as display lines for the terminal panel. A
 * single-purpose derived-state hook: the offset/append/reset bookkeeping lives
 * in the reducer, this only splits the accumulated text.
 */
export function useTerminalStream(): TerminalStream {
  const { session } = useAcpWorkspace();
  const text = session.terminal?.text ?? '';
  const lines = useMemo(() => splitTerminalLines(text), [text]);
  return { lines, hasOutput: text.length > 0 };
}

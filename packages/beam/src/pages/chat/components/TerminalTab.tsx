/**
 * i18n keys referenced in this file (namespace `terminal`; see i18n.ts):
 *   terminal.empty = "No terminal output yet. Type ! followed by a command to run it here."
 */
import { TerminalOutput } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { useTerminalStream } from '../../../hooks/useTerminalStream';

export interface TerminalTabProps {
  /** The session whose PTY scrollback this surface observes (`null` = the empty/new-chat surface). */
  sessionId: string | null;
}

/**
 * The terminal surface hosted as a CANVAS tab (workspace-canvas Slice B1) —
 * successor to the old `TerminalPanel` right sidebar. An observer of ITS
 * session's persistent shell (per-tab, via `sessionId`): it renders the live
 * output stream — both the `!`-passthrough lines the user runs and the lines the
 * agent runs, which share one PTY. Read-only. Pure presentation over
 * `useTerminalStream` + the packages/ui `TerminalOutput` component.
 *
 * Unlike `TerminalPanel` it carries no chrome of its own (no title bar, no
 * collapse button): the canvas tab strip supplies the label and the ✕, and this
 * simply fills the panel body.
 */
export function TerminalTab({ sessionId }: TerminalTabProps) {
  const { t } = useTranslation();
  const { lines, hasOutput } = useTerminalStream(sessionId);

  return (
    <TerminalOutput
      lines={hasOutput ? lines : [t('terminal.empty')]}
      maxHeight="100%"
      className="min-h-0 flex-1 rounded-none border-0"
    />
  );
}

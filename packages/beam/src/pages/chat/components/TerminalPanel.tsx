/**
 * i18n keys referenced in this file (namespace `terminal`; see i18n.ts):
 *   terminal.panel_title = "Terminal"
 *   terminal.collapse    = "Collapse terminal"
 *   terminal.empty       = "No terminal output yet. Type ! followed by a command to run it here."
 */
import { Button, cn, TerminalOutput } from '@contenox/ui';
import { PanelRightClose } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { useTerminalStream } from '../../../hooks/useTerminalStream';

export interface TerminalPanelProps {
  /** Collapses the panel. */
  onClose: () => void;
}

/**
 * IDE-style terminal panel — the second toggleable surface on the chat page,
 * sibling of the workspace file panel. An observer of the session's persistent
 * shell: it renders the live output stream (both `!` passthrough lines the user
 * runs and lines the agent runs — they share one PTY). Read-only in phases 1–2;
 * user typing into the shell is Phase 3. Pure presentation over the
 * `useTerminalStream` hook and the packages/ui `TerminalOutput` component.
 */
export function TerminalPanel({ onClose }: TerminalPanelProps) {
  const { t } = useTranslation();
  const { lines, hasOutput } = useTerminalStream();

  return (
    <div className="border-surface-200 dark:border-dark-surface-600 flex h-full w-80 min-w-0 shrink-0 flex-col border-l sm:w-96">
      <TerminalOutput
        title={t('terminal.panel_title')}
        lines={hasOutput ? lines : [t('terminal.empty')]}
        maxHeight="100%"
        className={cn('h-full rounded-none border-0')}
        actions={
          <Button type="button" variant="ghost" size="icon" aria-label={t('terminal.collapse')} onClick={onClose}>
            <PanelRightClose className="h-3.5 w-3.5" />
          </Button>
        }
      />
    </div>
  );
}

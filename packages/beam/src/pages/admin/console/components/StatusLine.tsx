import { Button, Span } from '@contenox/ui';
import { TerminalSquare } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { ContextUsageMeter } from '../../../../components/ContextUsageMeter';
import type { TaskEventConnectionState } from '../../../../hooks/useTaskEvents';

type StatusLineProps = {
  selectedChainId: string;
  activePolicyName: string;
  contextUsed: number;
  contextSize: number;
  sseConnection: TaskEventConnectionState;
  isProcessing: boolean;
  terminalAvailable: boolean;
  terminalOpen: boolean;
  onToggleTerminal: () => void;
};

function connectionDot(state: TaskEventConnectionState, isProcessing: boolean): string {
  if (!isProcessing) return 'bg-surface-400 dark:bg-dark-surface-400';
  switch (state) {
    case 'open':
      return 'bg-success';
    case 'connecting':
      return 'bg-warning';
    case 'error':
      return 'bg-error';
    default:
      return 'bg-surface-400 dark:bg-dark-surface-400';
  }
}

/**
 * TUI-style status bar for the console: read-only chain/policy display
 * (switched via /chain and /policy), context meter, connection dot, and the
 * terminal toggle. Selectors retired once the slash commands proved parity.
 */
export function StatusLine({
  selectedChainId,
  activePolicyName,
  contextUsed,
  contextSize,
  sseConnection,
  isProcessing,
  terminalAvailable,
  terminalOpen,
  onToggleTerminal,
}: StatusLineProps) {
  const { t } = useTranslation();

  return (
    <div className="border-surface-300 dark:border-dark-surface-400 bg-surface-50 dark:bg-dark-surface-100 flex flex-wrap items-center gap-x-3 gap-y-1 border-b px-3 py-1.5 font-mono text-xs">
      <span
        className={`inline-block h-2 w-2 shrink-0 rounded-full ${connectionDot(sseConnection, isProcessing)}`}
        title={isProcessing ? sseConnection : 'idle'}
      />
      <Span variant="muted" title={t('chat.chain', 'Chain') + ' — /chain'}>
        {selectedChainId || '—'}
      </Span>
      {activePolicyName && (
        <Span variant="muted" title={t('chat.hitl_policy', 'Policy') + ' — /policy'}>
          ⛨ {activePolicyName}
        </Span>
      )}
      <div className="ml-auto flex items-center gap-2">
        {contextSize > 0 && <ContextUsageMeter used={contextUsed} size={contextSize} />}
        {terminalAvailable && (
          <Button
            variant={terminalOpen ? 'primary' : 'ghost'}
            size="sm"
            className="h-6"
            onClick={onToggleTerminal}
            aria-label={t('chat.terminal', 'Terminal')}>
            <TerminalSquare className="h-3.5 w-3.5" />
          </Button>
        )}
        <Span variant="muted" className="text-[10px]">
          console
        </Span>
      </div>
    </div>
  );
}

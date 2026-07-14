import type { TaskEventConnectionState } from '../../../../hooks/useTaskEvents';
import { TERM } from '../term';

type StatusLineProps = {
  selectedChainId: string;
  activePolicyName: string;
  model?: string;
  contextUsed: number;
  contextSize: number;
  sseConnection: TaskEventConnectionState;
  isProcessing: boolean;
};

function dotColor(state: TaskEventConnectionState, isProcessing: boolean): string {
  if (!isProcessing) return 'bg-surface-400 dark:bg-dark-surface-500';
  switch (state) {
    case 'open':
      return 'bg-success';
    case 'connecting':
      return 'bg-warning';
    case 'error':
      return 'bg-error';
    default:
      return 'bg-surface-400 dark:bg-dark-surface-500';
  }
}

/** Dense terminal status bar: connection dot · chain · policy · model · context. */
export function StatusLine({
  selectedChainId,
  activePolicyName,
  model,
  contextUsed,
  contextSize,
  sseConnection,
  isProcessing,
}: StatusLineProps) {
  const pct = contextSize > 0 ? Math.round((Math.max(0, contextUsed) / contextSize) * 100) : 0;
  const ctxClass = pct > 90 ? TERM.err : pct > 70 ? 'text-warning' : TERM.dim;

  const Seg = ({ children }: { children: React.ReactNode }) => (
    <span className={TERM.dim}>{children}</span>
  );

  return (
    <div
      className={`flex flex-wrap items-center gap-x-3 gap-y-0.5 border-b ${TERM.border} ${TERM.surface} px-3 py-1 ${TERM.small}`}>
      <span
        className={`inline-block h-1.5 w-1.5 shrink-0 rounded-full ${dotColor(sseConnection, isProcessing)}`}
        title={isProcessing ? sseConnection : 'idle'}
      />
      <Seg>
        chain <span className={TERM.text}>{selectedChainId || '—'}</span>
      </Seg>
      {activePolicyName && (
        <Seg>
          policy <span className={TERM.text}>{activePolicyName}</span>
        </Seg>
      )}
      {model && (
        <Seg>
          model <span className={TERM.text}>{model}</span>
        </Seg>
      )}
      {contextSize > 0 && (
        <Seg>
          ctx <span className={ctxClass}>{pct}%</span>{' '}
          <span className="tabular-nums">
            ({Math.max(0, contextUsed).toLocaleString()}/{contextSize.toLocaleString()})
          </span>
        </Seg>
      )}
      <span className={`ml-auto ${TERM.dim}`}>console</span>
    </div>
  );
}

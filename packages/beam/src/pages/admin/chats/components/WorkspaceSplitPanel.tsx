import { Spinner, Button, Span } from '@contenox/ui';
import { forwardRef, lazy, Suspense, useImperativeHandle, useState } from 'react';

import type { ChatContextPayload, TaskEvent } from '../../../../lib/types';
import { cn } from '../../../../lib/utils';
import { TimelinePanel } from './TimelinePanel';
import { CanvasPanel, type CanvasArtifact } from './CanvasPanel';
import { useCanvas } from '../../../../hooks/useCanvas';
import type { CanvasArtifact } from '../../../lib/artifacts/canvas';

const TerminalPanel = lazy(() =>
  import('./TerminalPanel').then(m => ({ default: m.TerminalPanel })),
);

export type WorkspaceSplitHandle = {
  buildChatContext: () => ChatContextPayload | undefined;
};

type Props = {
  className?: string;
  events?: TaskEvent[];
  isProcessing?: boolean;
  selectedEvent?: TaskEvent | null;
  onSelectEvent?: (event: TaskEvent | null) => void;
  currentRunStatus?: string;
};

const WorkspaceSplitPanel = forwardRef<WorkspaceSplitHandle, Props>(function WorkspaceSplitPanel(
  { className, events = [], isProcessing = false, selectedEvent, onSelectEvent, currentRunStatus },
  ref,
) {
  useImperativeHandle(
    ref,
    () => ({
      buildChatContext: () => undefined,
    }),
    [],
  );

  // Simple mode for the slice: Timeline + Canvas in the sovereign workspace rail.
  // Terminal remains accessible (coexistence during transition per the architecture plan).
  const [mode, setMode] = useState<'canvas' | 'terminal'>('canvas');

  const canvas = useCanvas(events);

  // Wire external selection (from top level or timeline) into the hook
  // For this slice we prefer the hook's current, but allow prop-driven selection.
  const effectiveSelected = selectedEvent;
  // If parent selects, we could sync, but for simplicity use hook's current driven by internal timeline clicks.
  // Timeline inside will call onSelectEvent which parent can forward.
  const canvasArtifact: CanvasArtifact | null = canvas.current;

  return (
    <div
      className={cn(
        'border-surface-300 dark:border-dark-surface-400 bg-surface-50 dark:bg-dark-surface-100 flex min-h-0 w-full min-w-0 flex-col border-l',
        className,
      )}>
      <div className="flex items-center gap-1 border-b border-surface-300 p-1 text-xs dark:border-dark-surface-400">
        <Button
          variant={mode === 'canvas' ? 'default' : 'ghost'}
          size="sm"
          onClick={() => setMode('canvas')}
        >
          Canvas
        </Button>
        <Button
          variant={mode === 'terminal' ? 'default' : 'ghost'}
          size="sm"
          onClick={() => setMode('terminal')}
        >
          Terminal
        </Button>
        <div className="ml-auto flex items-center gap-2 pr-1">
          <Span variant="muted" className="text-[10px]">Sovereign workspace slice</Span>
        </div>
      </div>

      {mode === 'canvas' ? (
        <div className="flex min-h-0 flex-1 flex-col">
          {/* Timeline lives here inside the rail for the first slice (evolve inside existing split) */}
          <div className="min-h-[140px] border-b border-surface-300 dark:border-dark-surface-400">
            <TimelinePanel
              open
              onToggle={() => {}}
              events={events}
              isProcessing={isProcessing}
              selectedEvent={selectedEvent}
              onSelectEvent={(ev) => {
                canvas.select(ev);
                onSelectEvent?.(ev);
              }}
            />
          </div>
          <div className="min-h-0 flex-1">
            <CanvasPanel
              artifact={canvasArtifact}
              currentRun={
                isProcessing || events.length > 0
                  ? { status: currentRunStatus || (isProcessing ? 'Running' : 'Idle'), eventsCount: events.length }
                  : undefined
              }
            />
          </div>
        </div>
      ) : (
        <Suspense
          fallback={
            <div className="flex flex-1 items-center justify-center">
              <Spinner size="md" />
            </div>
          }>
          <TerminalPanel className="min-h-0 min-w-0 flex-1" />
        </Suspense>
      )}
    </div>
  );
});

export default WorkspaceSplitPanel;

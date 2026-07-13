import { Spinner } from '@contenox/ui';
import { forwardRef, lazy, Suspense, useImperativeHandle } from 'react';

import type { ChatContextPayload } from '../../../../lib/types';
import { cn } from '../../../../lib/utils';

const TerminalPanel = lazy(() =>
  import('./TerminalPanel').then(m => ({ default: m.TerminalPanel })),
);

export type WorkspaceSplitHandle = {
  buildChatContext: () => ChatContextPayload | undefined;
};

type Props = {
  className?: string;
};

const WorkspaceSplitPanel = forwardRef<WorkspaceSplitHandle, Props>(function WorkspaceSplitPanel(
  { className },
  ref,
) {
  useImperativeHandle(
    ref,
    () => ({
      buildChatContext: () => undefined,
    }),
    [],
  );

  return (
    <div
      className={cn(
        'border-surface-300 dark:border-dark-surface-400 bg-surface-50 dark:bg-dark-surface-100 flex min-h-0 w-full min-w-0 shrink-0 flex-col border-l',
        className,
      )}>
      <Suspense
        fallback={
          <div className="flex flex-1 items-center justify-center">
            <Spinner size="md" />
          </div>
        }>
        <TerminalPanel className="min-h-0 min-w-0 flex-1" />
      </Suspense>
    </div>
  );
});

export default WorkspaceSplitPanel;

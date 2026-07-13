import { useEffect, useRef, useState } from 'react';
import { api } from '../lib/api';
import { applyRunEvent, beginRun, emptyRunLog, type RunLog } from '../lib/runLog';
import type { TaskEventViewState } from '../lib/taskEvents';
import type { TaskEvent } from '../lib/types';
import type { TaskEventConnectionState } from './useTaskEvents';

const MAX_RETRIES = 8;

export type UseRunLogOptions = {
  enabled?: boolean;
  /** Fires when the EventSource has connected (subscribe before POST to avoid missing early events). */
  onConnectionOpen?: () => void;
};

export type UseRunLogResult = {
  /** Retained view state for every run this session, keyed by requestId. */
  runs: Record<string, TaskEventViewState>;
  /** requestIds in arrival order (oldest first). */
  runOrder: string[];
  /** The currently-subscribed run's state, if any. */
  activeRun: TaskEventViewState | null;
  connectionState: TaskEventConnectionState;
  connectionError: string | null;
};

/**
 * Console twin of useTaskEvents: subscribes to the task-event SSE stream for
 * the active requestId and reduces events with the shared reducer — but
 * retains every run's state after completion instead of resetting. This is
 * what makes the console's scrollback keep its work logs within a session.
 */
export function useRunLog(requestId: string | null, options?: UseRunLogOptions): UseRunLogResult {
  const [log, setLog] = useState<RunLog>(emptyRunLog);
  const [connectionState, setConnectionState] = useState<TaskEventConnectionState>('idle');
  const [connectionError, setConnectionError] = useState<string | null>(null);

  const onOpenRef = useRef(options?.onConnectionOpen);
  onOpenRef.current = options?.onConnectionOpen;

  useEffect(() => {
    if (!requestId || options?.enabled === false) {
      // No reset of `log` here — retention is the point.
      setConnectionState('idle');
      setConnectionError(null);
      return;
    }

    let terminated = false;
    let retryCount = 0;
    let timeoutId: ReturnType<typeof setTimeout>;
    let currentSource: EventSource | undefined;

    setLog(prev => beginRun(prev, requestId));
    setConnectionError(null);
    setConnectionState('connecting');

    const connect = () => {
      if (terminated) return;
      currentSource = api.taskEvents(requestId);

      currentSource.onopen = () => {
        if (terminated) return;
        retryCount = 0;
        setConnectionState('open');
        setConnectionError(null);
        try {
          onOpenRef.current?.();
        } catch (e) {
          console.error('run log onConnectionOpen:', e);
        }
      };

      currentSource.onmessage = event => {
        if (terminated) return;
        retryCount = 0;
        try {
          const parsed = JSON.parse(event.data) as TaskEvent;
          setLog(prev => applyRunEvent(prev, requestId, parsed));
          if (parsed.kind === 'chain_completed' || parsed.kind === 'chain_failed') {
            terminated = true;
            setConnectionState('closed');
            currentSource?.close();
          }
        } catch (error) {
          const msg = error instanceof Error ? error.message : String(error);
          setConnectionError(msg);
          console.error('Failed to parse task event:', error);
        }
      };

      currentSource.onerror = () => {
        if (terminated) return;
        currentSource?.close();
        if (retryCount < MAX_RETRIES) {
          const delay = Math.min(1000 * 2 ** retryCount, 30000);
          retryCount++;
          timeoutId = setTimeout(connect, delay);
          return;
        }
        setConnectionState('error');
        setConnectionError('Task event stream unavailable');
      };
    };

    connect();

    return () => {
      terminated = true;
      clearTimeout(timeoutId);
      currentSource?.close();
    };
  }, [requestId, options?.enabled]);

  return {
    runs: log.runs,
    runOrder: log.order,
    activeRun: requestId ? (log.runs[requestId] ?? null) : null,
    connectionState,
    connectionError,
  };
}

import { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { createTaskEventRequestId, type TaskEventViewState } from '../lib/taskEvents';
import type {
  CapturedStateUnit,
  ChatContextPayload,
  InlineAttachment,
} from '../lib/types';
import { useSendMessage } from './useChats';
import { useRunLog } from './useRunLog';
import type { TaskEventConnectionState } from './useTaskEvents';

export type OptimisticTurn = {
  requestId: string;
  content: string;
  attachments: InlineAttachment[];
  sentAt: string;
};

export type UseAgentTurnResult = {
  runs: Record<string, TaskEventViewState>;
  runOrder: string[];
  activeRun: TaskEventViewState | null;
  activeRequestId: string | null;
  isProcessing: boolean;
  httpDispatched: boolean;
  sseConnection: TaskEventConnectionState;
  optimistic: OptimisticTurn | null;
  clearOptimistic: () => void;
  contextUsed: number;
  contextSize: number;
  latestState: CapturedStateUnit[];
  operationError: string | null;
  clearOperationError: () => void;
  canRetry: boolean;
  retryLastFailed: () => void;
  submit: (
    text: string,
    chainId: string,
    context?: ChatContextPayload,
    inlineAttachments?: InlineAttachment[],
  ) => void;
  stop: () => void;
};

/**
 * The console's turn lifecycle: optimistic echo, SSE-subscribe-before-POST
 * dispatch ordering (with a 4s fallback for streams that never open), stop via
 * AbortController, and per-run event retention through useRunLog.
 *
 * The dispatch ordering is copied from ChatPage's proven machinery — the SSE
 * stream must be attached before the HTTP POST fires so early task events are
 * not missed; a fallback timer papers over streams that never reach `open`.
 * ChatPage retires with the console promotion; until then the duplication is
 * deliberate (do not "unify" it into the page scheduled for deletion).
 */
export function useAgentTurn(chatId: string): UseAgentTurnResult {
  const { t } = useTranslation();

  const [isProcessing, setIsProcessing] = useState(false);
  const [activeRequestId, setActiveRequestId] = useState<string | null>(null);
  /**
   * The run whose SSE stream we're subscribed to. Unlike activeRequestId it is
   * NOT cleared when the HTTP response lands: the tail events of a run
   * (step_completed / chain_completed after an approval resolves) race the
   * response, and the stream must stay open until the terminal event arrives —
   * useRunLog closes it on chain_completed/chain_failed by itself.
   */
  const [streamRequestId, setStreamRequestId] = useState<string | null>(null);
  const [httpDispatched, setHttpDispatched] = useState(false);
  const [optimistic, setOptimistic] = useState<OptimisticTurn | null>(null);
  const [operationError, setOperationError] = useState<string | null>(null);
  const [latestState, setLatestState] = useState<CapturedStateUnit[]>([]);
  const [contextUsed, setContextUsed] = useState(0);
  const [contextSize, setContextSize] = useState(0);

  const abortRef = useRef<AbortController | null>(null);
  const cancelledRef = useRef(false);
  const activeRequestIdRef = useRef<string | null>(null);
  const sendDispatchedRef = useRef(false);
  const lastFailedSendRef = useRef<{ text: string; chainId: string } | null>(null);
  const pendingSendRef = useRef<{
    requestId: string;
    message: string;
    chainId: string;
    signal: AbortSignal;
    context?: ChatContextPayload;
  } | null>(null);
  /** Survives the live stream teardown so the status line keeps the last value. */
  const lastKnownContextRef = useRef<{ used: number; size: number }>({ used: 0, size: 0 });

  const { mutate: sendMessage, error: sendError } = useSendMessage(chatId);

  const finishRun = useCallback(() => {
    abortRef.current = null;
    setIsProcessing(false);
    setActiveRequestId(null);
    activeRequestIdRef.current = null;
    pendingSendRef.current = null;
    sendDispatchedRef.current = false;
    setHttpDispatched(false);
  }, []);

  const tryDispatchSend = useCallback(() => {
    const pending = pendingSendRef.current;
    if (!pending || sendDispatchedRef.current) return;
    if (pending.requestId !== activeRequestIdRef.current) return;
    sendDispatchedRef.current = true;
    setHttpDispatched(true);

    sendMessage(
      {
        message: pending.message,
        chainId: pending.chainId,
        signal: pending.signal,
        requestId: pending.requestId,
        context: pending.context,
      },
      {
        onSuccess: response => {
          setLatestState(response.state || []);

          // Live token_usage events are the source of truth for context usage;
          // the response value is a last-resort fallback (often 0 for chains).
          const last = lastKnownContextRef.current;
          if (last.used > 0) {
            setContextUsed(last.used);
          } else if (typeof response.inputTokenCount === 'number' && response.inputTokenCount > 0) {
            setContextUsed(response.inputTokenCount);
            lastKnownContextRef.current.used = response.inputTokenCount;
          }
          if (last.size > 0) {
            setContextSize(last.size);
          }

          if (response.error) {
            lastFailedSendRef.current = pendingSendRef.current
              ? { text: pendingSendRef.current.message, chainId: pendingSendRef.current.chainId }
              : null;
            setOperationError(response.error);
          } else {
            lastFailedSendRef.current = null;
          }
          finishRun();
        },
        onError: (_, variables) => {
          const last = lastKnownContextRef.current;
          if (last.used > 0) setContextUsed(last.used);
          if (last.size > 0) setContextSize(last.size);

          if (variables.signal?.aborted) {
            lastFailedSendRef.current = null;
            setOperationError(t('common.cancel', 'Cancel'));
          } else {
            lastFailedSendRef.current = pendingSendRef.current
              ? { text: pendingSendRef.current.message, chainId: pendingSendRef.current.chainId }
              : null;
          }
          finishRun();
        },
      },
    );
  }, [finishRun, sendMessage, t]);

  const {
    runs,
    runOrder,
    activeRun,
    connectionState: sseConnection,
  } = useRunLog(streamRequestId, {
    enabled: !!streamRequestId,
    onConnectionOpen: () => {
      tryDispatchSend();
    },
  });

  // Sync live context usage into state that outlives the stream.
  useEffect(() => {
    if (activeRun?.contextUsed != null && activeRun.contextUsed > 0) {
      lastKnownContextRef.current.used = activeRun.contextUsed;
      setContextUsed(activeRun.contextUsed);
    }
    if (activeRun?.contextSize != null && activeRun.contextSize > 0) {
      lastKnownContextRef.current.size = activeRun.contextSize;
      setContextSize(activeRun.contextSize);
    }
  }, [activeRun?.contextUsed, activeRun?.contextSize]);

  /** If the event stream never reaches open, still send the request so the run can complete. */
  useEffect(() => {
    if (!activeRequestId || !isProcessing || sendDispatchedRef.current) return;
    const id = window.setTimeout(() => {
      tryDispatchSend();
    }, 4000);
    return () => window.clearTimeout(id);
  }, [activeRequestId, isProcessing, tryDispatchSend]);

  useEffect(() => {
    const errorMessage = sendError?.message;
    if (errorMessage) {
      if (cancelledRef.current) {
        cancelledRef.current = false;
        lastFailedSendRef.current = null;
        return;
      }
      setOperationError(errorMessage);
      const timer = setTimeout(() => setOperationError(null), 8000);
      return () => clearTimeout(timer);
    }
  }, [sendError]);

  const submit = useCallback(
    (
      text: string,
      chainId: string,
      context?: ChatContextPayload,
      inlineAttachments: InlineAttachment[] = [],
    ) => {
      setOperationError(null);
      const trimmed = text.trim();
      if (!trimmed && inlineAttachments.length === 0) return;

      abortRef.current?.abort();
      const controller = new AbortController();
      const requestId = createTaskEventRequestId();
      cancelledRef.current = false;
      abortRef.current = controller;

      setOptimistic({
        requestId,
        content: trimmed,
        attachments: inlineAttachments,
        sentAt: new Date().toISOString(),
      });

      pendingSendRef.current = {
        requestId,
        message: trimmed,
        chainId: chainId.trim(),
        signal: controller.signal,
        context,
      };
      sendDispatchedRef.current = false;
      setHttpDispatched(false);
      activeRequestIdRef.current = requestId;
      setActiveRequestId(requestId);
      setStreamRequestId(requestId);
      setIsProcessing(true);
    },
    [],
  );

  const stop = useCallback(() => {
    cancelledRef.current = true;
    abortRef.current?.abort();
    finishRun();
  }, [finishRun]);

  const retryLastFailed = useCallback(() => {
    const failed = lastFailedSendRef.current;
    if (!failed) return;
    setOperationError(null);
    lastFailedSendRef.current = null;
    submit(failed.text, failed.chainId);
  }, [submit]);

  return {
    runs,
    runOrder,
    activeRun,
    activeRequestId,
    isProcessing,
    httpDispatched,
    sseConnection,
    optimistic,
    clearOptimistic: useCallback(() => setOptimistic(null), []),
    contextUsed,
    contextSize,
    latestState,
    operationError,
    clearOperationError: useCallback(() => setOperationError(null), []),
    canRetry: lastFailedSendRef.current !== null,
    retryLastFailed,
    submit,
    stop,
  };
}

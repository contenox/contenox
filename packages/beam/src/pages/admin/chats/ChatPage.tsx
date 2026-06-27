import { ApprovalCard, Button, Fill, InlineNotice, Page, Section } from '@contenox/ui';
import { t } from 'i18next';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useListChains } from '../../../hooks/useChains';
import { useChatHistory, useCreateChat, useSendMessage } from '../../../hooks/useChats';
import { useListPolicies, useSetActivePolicy } from '../../../hooks/usePolicies';
import { useSetupStatus } from '../../../hooks/useSetupStatus';
import { useTaskEvents } from '../../../hooks/useTaskEvents';
import { api } from '../../../lib/api';
import { ArtifactRegistryProvider, useArtifactRegistry } from '../../../lib/artifacts';
import { artifactsToInlineAttachments } from '../../../lib/inlineAttachments';
import { getBlockingSetupIssue, getSetupIssueFixPath } from '../../../lib/setupHealth';
import {
  SlashCommandRegistryProvider,
  createHelpCommand,
  useSlashCommand,
  useSlashCommandRegistry,
} from '../../../lib/slashCommands';
import { createTaskEventRequestId } from '../../../lib/taskEvents';
import {
  ChatMessage as ApiChatMessage,
  CapturedStateUnit,
  type ChatContextArtifact,
  type ChatContextPayload,
  type ChatModeId,
  type InlineAttachment,
} from '../../../lib/types';
import { buildChatThreadItems } from './chatThreadItems';
import { ChatInterface } from './components/ChatInterface';
import { ChatRunLog } from './components/ChatRunLog';
import { ChatToolbar } from './components/ChatToolbar';
import { MessageInputForm } from './components/MessageInputForm';

const STATE_PANEL_STORAGE_KEY = 'beam_chat_state_panel_open';
const DEFAULT_CHAIN_PATH = 'default-chain.json';

function shouldOpenStatePanelByDefault(): boolean {
  if (typeof window === 'undefined') return true;
  if (!window.matchMedia('(min-width: 768px)').matches) return false;
  return window.localStorage.getItem(STATE_PANEL_STORAGE_KEY) !== '0';
}

function formatChainLabel(path: string): string {
  return path.replace(/\.json$/i, '');
}

type BeamChatLocationState = {
  beamInitialMessage?: string;
  beamInitialChainId?: string;
};

/**
 * Thin wrapper that establishes the ArtifactRegistry for this ChatPage
 * instance, then renders the real component. The registry lives at this
 * boundary so descendants (terminal, composer, etc.) can all register sources
 * whose artifacts the composer collects at send time.
 */
export default function ChatPage() {
  return (
    <ArtifactRegistryProvider>
      <SlashCommandRegistryProvider>
        <ChatPageImpl />
      </SlashCommandRegistryProvider>
    </ArtifactRegistryProvider>
  );
}

/**
 * Build the per-turn context from artifacts contributed by sources registered
 * in the [ArtifactRegistry].
 *
 * Returns undefined when there is nothing to attach — keeping the send path
 * byte-for-byte identical to the pre-registry behaviour when no sources are
 * active.
 */
function buildTurnContext(
  registryArtifacts: ChatContextArtifact[],
): ChatContextPayload | undefined {
  if (registryArtifacts.length === 0) return undefined;
  return { artifacts: [...registryArtifacts] };
}

/**
 * Optimistic user message awaiting echo from the persisted history. Carries
 * inline attachments derived from slash-armed artifact sources so the user
 * can see what they attached on this turn. Cleared once the persisted version
 * arrives (matched by content + sentAt windowing).
 */
type OptimisticUserOutgoing = {
  /** Synthetic id; not the eventual server id. */
  id: string;
  content: string;
  attachments: InlineAttachment[];
  sentAt: string;
};

/**
 * Convert a tool call (function name + JSON arguments string + result content)
 * into an inline attachment for rendering in the chat thread. Returns null for
 * tool calls that have no meaningful visual form (e.g. write_file).
 */
function toolCallToInlineAttachment(
  name: string,
  argsJson: string,
  content: string,
): InlineAttachment | null {
  try {
    const args = JSON.parse(argsJson) as Record<string, unknown>;
    switch (name) {
      case 'read_file':
      case 'read_file_range':
        return {
          kind: 'file_view',
          path: String(args.path ?? args.file ?? ''),
          text: content,
          truncated: false,
        };
      case 'grep':
        return {
          kind: 'terminal_excerpt',
          command: `grep ${String(args.pattern ?? '')} ${String(args.path ?? '')}`.trim(),
          output: content,
        };
      case 'list_dir':
        return {
          kind: 'terminal_excerpt',
          command: `ls ${String(args.path ?? '')}`.trim(),
          output: content,
        };
      case 'stat_file':
      case 'count_stats':
        return {
          kind: 'terminal_excerpt',
          command: `${name} ${String(args.path ?? '')}`.trim(),
          output: content,
        };
      default:
        return null;
    }
  } catch {
    return null;
  }
}

function ChatPageImpl() {
  const { chatId: paramChatId } = useParams<{ chatId: string }>();
  const artifactRegistry = useArtifactRegistry();
  const slashRegistry = useSlashCommandRegistry();
  const [optimisticOutgoing, setOptimisticOutgoing] = useState<OptimisticUserOutgoing | null>(null);
  /**
   * Agent-emitted inline attachments keyed by persisted assistant message id
   * (Phase 5 of the canvas-vision plan). Captured from the live SSE stream
   * once the persisted echo arrives so attachments survive after the live
   * row collapses. Cleared on chat session change.
   */
  const [agentAttachments, setAgentAttachments] = useState<Record<string, InlineAttachment[]>>({});
  const location = useLocation();
  const navigate = useNavigate();
  const chatId = paramChatId ?? null;
  const [message, setMessage] = useState('');
  const [operationError, setOperationError] = useState<string | null>(null);
  const [selectedChainId, setSelectedChainId] = useState('');
  const [selectedMode, setSelectedMode] = useState<ChatModeId>('chat');
  const [latestState, setLatestState] = useState<CapturedStateUnit[]>([]);
  const [isProcessing, setIsProcessing] = useState(false);
  const [activeRequestId, setActiveRequestId] = useState<string | null>(null);
  /** True after POST /api/chats/:id/chat has been dispatched for the current run. */
  const [httpDispatched, setHttpDispatched] = useState(false);
  const abortRef = useRef<AbortController | null>(null);
  const cancelledRef = useRef(false);
  const activeRequestIdRef = useRef<string | null>(null);
  const lastFailedSendRef = useRef<{ text: string; chainId: string; mode: ChatModeId } | null>(
    null,
  );
  const pendingSendRef = useRef<{
    requestId: string;
    message: string;
    chainId: string;
    mode: ChatModeId;
    signal: AbortSignal;
    context?: ChatContextPayload;
  } | null>(null);
  const sendDispatchedRef = useRef(false);
  const landingInitialSendKeyRef = useRef<string | null>(null);
  const [statePanelOpen, setStatePanelOpen] = useState(shouldOpenStatePanelByDefault);

  const toggleStatePanel = () => {
    setStatePanelOpen(open => {
      const next = !open;
      try {
        window.localStorage.setItem(STATE_PANEL_STORAGE_KEY, next ? '1' : '0');
      } catch {
        /* ignore quota / private mode */
      }
      return next;
    });
  };

  useEffect(() => {
    const media = window.matchMedia('(min-width: 768px)');
    const handleChange = () => {
      if (!media.matches) {
        setStatePanelOpen(false);
      }
    };
    handleChange();
    media.addEventListener('change', handleChange);
    return () => media.removeEventListener('change', handleChange);
  }, []);

  const { data: chainPaths = [], isLoading: chainsLoading } = useListChains();
  const sortedChainPaths = useMemo(
    () =>
      [...chainPaths].sort((a, b) => {
        if (a === DEFAULT_CHAIN_PATH) return -1;
        if (b === DEFAULT_CHAIN_PATH) return 1;
        return a.localeCompare(b);
      }),
    [chainPaths],
  );

  // Preselect default-chain.json (or the first available chain) when no chain
  // has been chosen yet. Mirrors what the server does via chain_resolve.go.
  useEffect(() => {
    if (selectedChainId || !sortedChainPaths.length) return;
    const def = sortedChainPaths[0];
    setSelectedChainId(def);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sortedChainPaths]);

  const { data: chatHistory, isLoading: historyLoading, error } = useChatHistory(chatId || '');
  const { mutate: sendMessage, error: sendError } = useSendMessage(chatId || '');
  const { data: policyNames = [] } = useListPolicies();
  const { data: setupStatus } = useSetupStatus(true);
  const blockingSetupIssue = getBlockingSetupIssue(setupStatus);
  const activePolicyName = setupStatus?.hitlPolicyName ?? '';
  const setActivePolicy = useSetActivePolicy();

  const helpCommand = useMemo(() => createHelpCommand(slashRegistry), [slashRegistry]);
  useSlashCommand(helpCommand);

  const {
    mutate: createChat,
    isError,
    error: createError,
    isPending: isCreating,
  } = useCreateChat();

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
        mode: pending.mode,
        signal: pending.signal,
        requestId: pending.requestId,
        context: pending.context,
      },
      {
        onSuccess: response => {
          setLatestState(response.state || []);
          if (response.error) {
            lastFailedSendRef.current = pendingSendRef.current
              ? {
                  text: pendingSendRef.current.message,
                  chainId: pendingSendRef.current.chainId,
                  mode: pendingSendRef.current.mode,
                }
              : null;
            setOperationError(response.error);
          } else {
            lastFailedSendRef.current = null;
          }
          abortRef.current = null;
          setIsProcessing(false);
          setActiveRequestId(null);
          activeRequestIdRef.current = null;
          pendingSendRef.current = null;
          sendDispatchedRef.current = false;
          setHttpDispatched(false);
        },
        onError: (_, variables) => {
          abortRef.current = null;
          if (variables.signal?.aborted) {
            lastFailedSendRef.current = null;
            setOperationError(t('common.cancel', 'Cancel'));
          } else {
            lastFailedSendRef.current = pendingSendRef.current
              ? {
                  text: pendingSendRef.current.message,
                  chainId: pendingSendRef.current.chainId,
                  mode: pendingSendRef.current.mode,
                }
              : null;
          }
          setIsProcessing(false);
          setActiveRequestId(null);
          activeRequestIdRef.current = null;
          pendingSendRef.current = null;
          sendDispatchedRef.current = false;
          setHttpDispatched(false);
        },
      },
    );
  }, [sendMessage, t]);

  const { state: liveTask, connectionState: sseConnection } = useTaskEvents(activeRequestId, {
    enabled: !!activeRequestId && isProcessing,
    onConnectionOpen: () => {
      tryDispatchSend();
    },
  });

  /** If the event stream never reaches open, still send the chat request so the run can complete. */
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

  const processingBarLabel = useMemo(() => {
    if (!isProcessing) return '';
    if (!httpDispatched) {
      if (sseConnection === 'connecting') return t('chat.sse_connecting');
      if (sseConnection === 'open') return t('chat.sse_sending_http');
      if (sseConnection === 'error') return t('chat.sse_degraded');
      return t('chat.sse_queued');
    }
    if (liveTask.error) {
      return liveTask.status || t('chat.task_failed');
    }
    return liveTask.status || t('chat.thinking');
  }, [httpDispatched, isProcessing, liveTask.error, liveTask.status, sseConnection, t]);

  const submitOutgoingMessage = useCallback(
    (text: string, chainIdForSend: string, modeForSend: ChatModeId) => {
      setOperationError(null);
      if (!text.trim()) return;

      abortRef.current?.abort();
      const controller = new AbortController();
      const requestId = createTaskEventRequestId();
      cancelledRef.current = false;
      abortRef.current = controller;

      // Collect the registry exactly once: one-shot slash sources unregister
      // themselves on collect, so a second pass would yield nothing.
      const collected = artifactRegistry.collectWithSources();
      const allArtifacts = collected.map(p => p.artifact);
      // Inline-display attachments come ONLY from explicitly-armed sources
      // (`mention:` from @-mentions, `slash:` legacy). Sticky sources
      // (workspace open_file, terminal armed_output) already have indicators
      // in their owning panels; rendering them on every user message would
      // be visual noise.
      const explicitArtifacts = collected
        .filter(p => p.source.id.startsWith('mention:') || p.source.id.startsWith('slash:'))
        .map(p => p.artifact);
      const inlineAttachments = artifactsToInlineAttachments(explicitArtifacts);

      const context = buildTurnContext(allArtifacts);

      const trimmed = text.trim();
      // Only show optimistic user message when there's something to show.
      if (trimmed || inlineAttachments.length > 0) {
        setOptimisticOutgoing({
          id: `optimistic-${requestId}`,
          content: trimmed,
          attachments: inlineAttachments,
          sentAt: new Date().toISOString(),
        });
      }

      pendingSendRef.current = {
        requestId,
        message: trimmed,
        chainId: chainIdForSend.trim(),
        mode: modeForSend,
        signal: controller.signal,
        context,
      };
      sendDispatchedRef.current = false;
      setHttpDispatched(false);
      activeRequestIdRef.current = requestId;
      setActiveRequestId(requestId);
      setIsProcessing(true);
      setMessage('');
    },
    [artifactRegistry, t],
  );

  const handleSendMessage = (e: React.FormEvent) => {
    e.preventDefault();
    submitOutgoingMessage(message, selectedChainId, selectedMode);
  };

  /** After `/chat` creates a session and navigates here with state, send the first message once. */
  useEffect(() => {
    if (!paramChatId) return;
    const st = location.state as BeamChatLocationState | null;
    if (!st?.beamInitialMessage?.trim() || !st.beamInitialChainId) return;

    const text = st.beamInitialMessage.trim();
    const chain = st.beamInitialChainId;
    const dedupeKey = `${paramChatId}\0${text}\0${chain}`;
    if (landingInitialSendKeyRef.current === dedupeKey) return;
    landingInitialSendKeyRef.current = dedupeKey;

    navigate({ pathname: location.pathname }, { replace: true, state: null });
    setSelectedChainId(chain);
    queueMicrotask(() => {
      submitOutgoingMessage(text, chain, 'chat');
    });
  }, [paramChatId, location.state, location.pathname, navigate, submitOutgoingMessage]);

  const handleStop = useCallback(() => {
    cancelledRef.current = true;
    abortRef.current?.abort();
    abortRef.current = null;
    pendingSendRef.current = null;
    sendDispatchedRef.current = false;
    activeRequestIdRef.current = null;
    setActiveRequestId(null);
    setIsProcessing(false);
    setHttpDispatched(false);
  }, []);

  const handleCreateChat = () => createChat({});

  const chainOptions = [
    { value: '', label: t('chat.no_chain') },
    ...sortedChainPaths.map(p => ({ value: p, label: formatChainLabel(p) })),
  ];
  const modeOptions: { value: ChatModeId; label: string }[] = [
    { value: 'chat', label: t('chat.mode_chat') },
    { value: 'prompt', label: t('chat.mode_prompt') },
  ];
  const displayHistory = useMemo<ApiChatMessage[]>(() => {
    const base = chatHistory ?? [];

    // Build a lookup from toolCallId → function metadata so tool-result
    // messages can be rendered as typed cards rather than raw text blobs.
    const callMap = new Map<string, { name: string; arguments: string }>();
    for (const m of base) {
      if (m.role === 'assistant' && m.callTools) {
        for (const tc of m.callTools) {
          callMap.set(tc.id, tc.function);
        }
      }
    }

    // Augment persisted assistant messages with their agent-emitted
    // attachments, when we captured any during their streaming turn.
    // Augment tool-result messages with typed inline attachments derived
    // from the function name + arguments of their paired call.
    const merged: ApiChatMessage[] = base.map(m => {
      if (m.role === 'assistant' && m.id && agentAttachments[m.id]) {
        return { ...m, attachments: agentAttachments[m.id] };
      }
      if (m.role === 'tool' && m.toolCallId) {
        const fn = callMap.get(m.toolCallId);
        if (fn) {
          const attachment = toolCallToInlineAttachment(fn.name, fn.arguments, m.content);
          if (attachment) {
            return { ...m, attachments: [attachment] };
          }
        }
      }
      return m;
    });

    // Optimistic user message: appended only when its content has not yet
    // appeared in the persisted history. We match by content + a 5-minute
    // sentAt window to tolerate clock skew between client and server.
    if (optimisticOutgoing) {
      const optAt = Date.parse(optimisticOutgoing.sentAt);
      const matched = base.some(m => {
        if (m.role !== 'user') return false;
        if (m.content !== optimisticOutgoing.content) return false;
        const persistedAt = Date.parse(m.sentAt);
        return Math.abs(persistedAt - optAt) < 5 * 60_000;
      });
      if (!matched) {
        merged.push({
          id: optimisticOutgoing.id,
          role: 'user',
          content: optimisticOutgoing.content,
          sentAt: optimisticOutgoing.sentAt,
          isUser: true,
          isLatest: true,
          attachments: optimisticOutgoing.attachments,
        });
      }
    }

    if (!isProcessing || !activeRequestId) {
      return merged;
    }
    merged.push({
      id: `live-${activeRequestId}`,
      role: 'assistant',
      content: liveTask.content,
      sentAt: new Date().toISOString(),
      isUser: false,
      isLatest: true,
      streaming: true,
      error: liveTask.error ?? undefined,
      attachments: liveTask.attachments,
      events: liveTask.events,
    });
    return merged;
  }, [
    activeRequestId,
    agentAttachments,
    chatHistory,
    isProcessing,
    liveTask.attachments,
    liveTask.content,
    liveTask.error,
    liveTask.events,
    optimisticOutgoing,
  ]);

  // Drop the optimistic outgoing once the persisted history echoes it back.
  useEffect(() => {
    if (!optimisticOutgoing) return;
    const persisted = chatHistory ?? [];
    const optAt = Date.parse(optimisticOutgoing.sentAt);
    const matched = persisted.some(
      m =>
        m.role === 'user' &&
        m.content === optimisticOutgoing.content &&
        Math.abs(Date.parse(m.sentAt) - optAt) < 5 * 60_000,
    );
    if (matched) setOptimisticOutgoing(null);
  }, [chatHistory, optimisticOutgoing]);

  // Capture agent-emitted inline attachments onto the persisted assistant
  // message that just landed. Heuristic: while liveTask still holds the
  // streamed attachments, find the most recent persisted assistant message
  // whose id we have not already claimed and bind the attachments to it.
  // This lets the FileView / TerminalExcerpt cards survive after the live
  // streaming row collapses into the persisted thread.
  useEffect(() => {
    if (!liveTask.attachments || liveTask.attachments.length === 0) return;
    const persisted = chatHistory ?? [];
    for (let i = persisted.length - 1; i >= 0; i--) {
      const m = persisted[i];
      if (m.role !== 'assistant' || !m.id) continue;
      if (agentAttachments[m.id]) break; // already claimed; don't double-bind
      const attachments = liveTask.attachments;
      setAgentAttachments(prev => (prev[m.id!] ? prev : { ...prev, [m.id!]: attachments }));
      break;
    }
  }, [chatHistory, liveTask.attachments, agentAttachments]);

  // Clear captured attachments when the active chat session changes — they
  // are keyed by message ids that only make sense within one session.
  useEffect(() => {
    setAgentAttachments({});
    setOptimisticOutgoing(null);
  }, [chatId]);

  const threadItems = useMemo(
    () =>
      buildChatThreadItems({
        displayHistory,
      }),
    [displayHistory],
  );

  const streamScrollSignature = useMemo(
    () =>
      `${liveTask.content.length}\0${liveTask.thinking.length}\0${liveTask.events.length}\0${liveTask.status}\0${threadItems.length}`,
    [
      liveTask.content.length,
      liveTask.events.length,
      liveTask.status,
      liveTask.thinking.length,
      threadItems.length,
    ],
  );

  const chatMainFill = (
    <Fill className="bg-surface-50 dark:bg-dark-surface-100 flex flex-col">
      {chatId ? (
        <>
          <ChatToolbar
            chainOptions={chainOptions}
            selectedChainId={selectedChainId}
            onChainChange={setSelectedChainId}
            chainsLoading={chainsLoading}
            modeOptions={modeOptions}
            selectedMode={selectedMode}
            onModeChange={setSelectedMode}
            isProcessing={isProcessing}
            policyNames={policyNames}
            activePolicyName={activePolicyName}
            onPolicyChange={name => setActivePolicy.mutate(name)}
            policyChangePending={setActivePolicy.isPending}
            policyChangeError={
              setActivePolicy.isError
                ? (setActivePolicy.error?.message ??
                  t('chat.hitl_policy_error', 'Failed to set policy'))
                : null
            }
            statsLabel={t('chat.stats_compact', {
              messages: chatHistory?.length ?? 0,
              state: latestState.length,
            })}
            onEditChain={() =>
              navigate(`/chains?path=${encodeURIComponent(selectedChainId.trim())}`)
            }
          />

          <Fill className="flex flex-col">
            {httpDispatched && sseConnection === 'error' && (
              <InlineNotice variant="warning">{t('chat.sse_stream_lost')}</InlineNotice>
            )}
            {operationError && (
              <InlineNotice
                variant="error"
                onDismiss={() => {
                  setOperationError(null);
                  lastFailedSendRef.current = null;
                }}>
                <span>{operationError}</span>
                {lastFailedSendRef.current && (
                  <Button
                    variant="ghost"
                    size="sm"
                    className="ml-2 h-5 text-xs"
                    type="button"
                    onClick={() => {
                      const failed = lastFailedSendRef.current;
                      if (!failed) return;
                      setOperationError(null);
                      lastFailedSendRef.current = null;
                      submitOutgoingMessage(failed.text, failed.chainId, failed.mode);
                    }}>
                    {t('plan.retry', 'Retry')}
                  </Button>
                )}
              </InlineNotice>
            )}
            {blockingSetupIssue && (
              <InlineNotice variant="error" className="mx-3 mt-3">
                <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                  <div className="space-y-1">
                    <span className="block font-medium">
                      {t('chat.setup_blocked_title', 'Chat setup needs attention')}
                    </span>
                    <span className="block">{blockingSetupIssue.message}</span>
                    {blockingSetupIssue.cliCommand ? (
                      <code className="text-text dark:text-dark-text bg-surface-100 dark:bg-dark-surface-300 block rounded-md px-2 py-1 text-xs">
                        {blockingSetupIssue.cliCommand}
                      </code>
                    ) : null}
                  </div>
                  <Button
                    type="button"
                    variant="outline"
                    palette="neutral"
                    size="sm"
                    className="shrink-0"
                    onClick={() => navigate(getSetupIssueFixPath(blockingSetupIssue))}>
                    {t('chat.setup_blocked_action', 'Open setup')}
                  </Button>
                </div>
              </InlineNotice>
            )}

            <div className="min-h-0 flex-1">
              {chatHistory && Array.isArray(chatHistory) && (
                <ChatInterface
                  threadItems={threadItems}
                  isLoading={historyLoading}
                  error={error}
                  isProcessing={isProcessing}
                  processingBarLabel={processingBarLabel}
                  embedStreamThinkingInThread
                  liveThinking={liveTask.thinking}
                  canStop={isProcessing}
                  onStop={handleStop}
                  streamScrollSignature={streamScrollSignature}
                  approvalContent={
                    liveTask.pendingApproval ? (
                      <ApprovalCard
                        approval={liveTask.pendingApproval}
                        onRespond={async approved => {
                          if (!liveTask.pendingApproval) return;
                          try {
                            await api.respondToApproval(
                              liveTask.pendingApproval.approvalId,
                              approved,
                            );
                          } catch {
                            // Backend will surface the outcome via the SSE stream.
                          }
                        }}
                      />
                    ) : undefined
                  }
                />
              )}
            </div>
          </Fill>

          <div className="bg-surface-50 dark:bg-dark-surface-200 shrink-0">
            <MessageInputForm
              value={message}
              onChange={setMessage}
              onSubmit={handleSendMessage}
              isPending={isProcessing}
              variant="workbench"
              placeholder={t('chat.workbench_placeholder')}
              buttonLabel={t('chat.run_button')}
              canSubmit={!isProcessing && !!message.trim() && !blockingSetupIssue}
              allowEmptyMessage={false}
            />
          </div>
        </>
      ) : (
        <Section
          title={t('chat.no_chat_selected')}
          description={t('chat.start_conversation_prompt')}
          className="text-text dark:text-dark-text min-w-0 flex-1">
          <div className="mt-6 flex gap-3">
            <Button onClick={handleCreateChat} size="lg" isLoading={isCreating}>
              {t('chat.create_chat')}
            </Button>
            <Button variant="outline" size="lg">
              {t('chat.view_examples')}
            </Button>
          </div>
          {isError && (
            <InlineNotice variant="errorSoft" className="mt-4">
              {createError?.message || t('chat.error_create_chat')}
            </InlineNotice>
          )}
        </Section>
      )}
    </Fill>
  );

  return (
    <Page bodyScroll="hidden" className="h-full">
      <Fill className="flex min-h-0">
        <div className="flex min-h-0 min-w-0 flex-1 flex-row">
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">{chatMainFill}</div>
        </div>

        <ChatRunLog
          open={statePanelOpen}
          onToggle={toggleStatePanel}
          isProcessing={isProcessing}
          events={liveTask.events}
          state={latestState}
        />
      </Fill>
    </Page>
  );
}

import {
  AlertTriangle,
  Bot,
  Check,
  Database,
  FileDiff,
  FileText,
  Loader2,
  MessageSquarePlus,
  Package,
  Pencil,
  RefreshCw,
  Search,
  Settings2,
  SlidersHorizontal,
  Trash2,
  Wrench,
  X,
} from 'lucide-react';
import React, { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import {
  Badge,
  Button,
  ChatComposer,
  ChatDateSeparator,
  ChatMessage as ChatMessageUI,
  ChatScrollToLatest,
  ChatThread,
  ChatThreadSkeleton,
  InlineNotice,
  Panel,
  Span,
  chatTranscriptMarkdownComponents,
  useChatScroll,
} from '../../../ui/src/chat';

export type BeamChatSession = {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  lastMessageAt?: string | null;
};

export type BeamChatMessageRole = 'system' | 'user' | 'assistant' | 'tool';

export type BeamChatCitation = {
  title?: string;
  source?: string;
  url?: string;
  path?: string;
};

export type BeamChatMessage = {
  id: string;
  sessionId: string;
  role: BeamChatMessageRole;
  content: string;
  createdAt: string;
  citations?: BeamChatCitation[];
  toolCalls?: BeamChatToolCall[];
  error?: string;
};

export type BeamChatTool = {
  id: string;
  label: string;
  mode: 'read' | 'mutate' | string;
  enabled: boolean;
};

export type BeamChatToolCallDiff = {
  path?: string;
  before?: string;
  after?: string;
};

export type BeamChatToolCall = {
  id: string;
  title?: string;
  status: 'running' | 'completed' | 'error' | string;
  toolName?: string;
  output?: string;
  error?: string;
  diff?: BeamChatToolCallDiff;
};

export type BeamChatApprovalOption = {
  id: string;
  label: string;
  kind: string;
};

export type BeamChatApprovalRequest = {
  approvalId: string;
  title: string;
  toolName?: string;
  details?: string;
  diff?: BeamChatToolCallDiff;
  options: BeamChatApprovalOption[];
};

export type BeamChatTurnHandlers = {
  onDelta?: (chunk: { content?: string; thinking?: string }) => void;
  onToolCall?: (call: BeamChatToolCall) => void;
  onApprovalRequest?: (request: BeamChatApprovalRequest) => Promise<string | undefined>;
};

export type BeamChatReadiness = {
  aiReady?: boolean;
  appCount: number;
  canManage: boolean;
  enabledToolCount?: number;
  searchReady: boolean;
  sourceCount: number;
  syncedSourceCount: number;
};

export type BeamChatSessionResponse = {
  session?: BeamChatSession;
  messages?: BeamChatMessage[];
};

export type BeamChatMessageResponse = {
  session?: BeamChatSession;
  messages?: BeamChatMessage[];
};

export type BeamChatClient = {
  listSessions: () => Promise<BeamChatSession[]>;
  createSession: (input: { title: string }) => Promise<BeamChatSessionResponse>;
  getSession: (id: string) => Promise<BeamChatSessionResponse>;
  renameSession?: (id: string, input: { title: string }) => Promise<BeamChatSessionResponse>;
  deleteSession?: (id: string) => Promise<void>;
  sendMessage: (
    id: string,
    input: { content: string },
    handlers?: BeamChatTurnHandlers,
  ) => Promise<BeamChatMessageResponse>;
  cancelTurn?: (id: string) => void;
  listTools: () => Promise<BeamChatTool[]>;
  openDiff?: (call: BeamChatToolCall) => void;
};

export type BeamChatComposerAction = {
  nonce: string;
  content: string;
  submit: boolean;
};

export type BeamChatRuntimeSummary = {
  provider?: string;
  model?: string;
  think?: string;
  hitlPolicy?: string;
  connected?: boolean;
  // Used context indicator (ACP usage_update / engine tokens vs model ctx len)
  contextUsed?: number;
  contextSize?: number;
};

export type BeamChatLinks = {
  ai?: string;
  apps: string;
  search: string;
  sources: string;
};

type LoadState = 'loading' | 'ready' | 'unavailable';

const defaultLinks: BeamChatLinks = {
  ai: '/ai',
  apps: '/apps',
  search: '/search',
  sources: '/sources',
};

const dateFmt = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
});

function fmtDate(value?: string | null): string {
  return value ? dateFmt.format(new Date(value)) : '';
}

function dateKey(value: string): string {
  return value.slice(0, 10);
}

function formatDateLabel(value: string): string {
  const date = new Date(value);
  const now = new Date();
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const target = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const diffDays = Math.round((today.getTime() - target.getTime()) / 86400000);
  if (diffDays === 0) return 'Today';
  if (diffDays === 1) return 'Yesterday';
  return date.toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' });
}

function attachToolCallsToLastAssistantMessage(
  messages: BeamChatMessage[],
  toolCalls: BeamChatToolCall[],
): BeamChatMessage[] {
  const lastAssistantIndex = [...messages]
    .map((message, index) => ({ message, index }))
    .reverse()
    .find(({ message }) => message.role === 'assistant')?.index;
  if (lastAssistantIndex === undefined) return messages;
  return messages.map((message, index) =>
    index === lastAssistantIndex ? { ...message, toolCalls } : message,
  );
}

function sessionTitle(session: BeamChatSession): string {
  return session.title?.trim() || 'Untitled chat';
}

function composerPlaceholder(
  loadState: LoadState,
  aiReady: boolean,
  selected: BeamChatSession | null,
  productName: string,
  embedded: boolean,
): string {
  if (loadState === 'unavailable') {
    return embedded
      ? `${productName} runtime is not connected.`
      : `${productName} is not connected yet.`;
  }
  if (!aiReady) {
    return embedded
      ? `Run ${productName} setup to start`
      : `Complete ${productName} setup to start`;
  }
  if (selected) {
    return embedded ? 'Ask about this workspace…' : 'Ask about this workspace';
  }
  return 'Create a session to start';
}

function runtimeChipLabel(summary: BeamChatRuntimeSummary | null | undefined): string {
  if (!summary?.provider && !summary?.model) {
    return 'Runtime not configured';
  }
  const provider = summary.provider || 'no provider';
  const model = summary.model || 'no model';
  return `${provider} · ${model}`;
}

function statusSubtitle(
  readiness: BeamChatReadiness,
  productName: string,
  embedded: boolean,
): string {
  if (embedded) {
    return readiness.aiReady ? 'Ready' : 'Setup required';
  }
  if (readiness.aiReady && readiness.searchReady) {
    return `Workspace search is ready for ${productName}.`;
  }
  if (readiness.aiReady) {
    return `${productName} is ready. Workspace search is not prepared yet.`;
  }
  return `${productName} setup is not complete yet.`;
}

function upsertSession(sessions: BeamChatSession[], session?: BeamChatSession): BeamChatSession[] {
  if (!session) return sessions;
  const next = [session, ...sessions.filter(item => item.id !== session.id)];
  return next.sort((a, b) => {
    const av = new Date(a.lastMessageAt ?? a.updatedAt).getTime();
    const bv = new Date(b.lastMessageAt ?? b.updatedAt).getTime();
    return bv - av;
  });
}

type PendingApproval = BeamChatApprovalRequest & {
  resolve: (optionId: string | undefined) => void;
};

export function BeamChat({
  client,
  links = defaultLinks,
  readiness,
  embedded = false,
  productName = 'Beam',
  productIcon,
  composerAction,
  onComposerActionHandled,
  selectSessionId,
  confirmDeleteSession,
  promptRenameSession,
  runtimeSummary,
  onOpenRuntimeSettings,
}: {
  client: BeamChatClient;
  links?: BeamChatLinks;
  readiness: BeamChatReadiness;
  embedded?: boolean;
  productName?: string;
  /** Custom icon to use for product/assistant branding (e.g. Contenox logo). Falls back to generic Bot. */
  productIcon?: React.ReactNode;
  composerAction?: BeamChatComposerAction | null;
  onComposerActionHandled?: () => void;
  selectSessionId?: string | null;
  confirmDeleteSession?: (session: BeamChatSession) => Promise<boolean>;
  promptRenameSession?: (
    session: BeamChatSession,
    currentTitle: string,
  ) => Promise<string | undefined>;
  runtimeSummary?: BeamChatRuntimeSummary | null;
  onOpenRuntimeSettings?: () => void;
}) {
  const [loadState, setLoadState] = useState<LoadState>('loading');
  const [sessions, setSessions] = useState<BeamChatSession[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [messages, setMessages] = useState<BeamChatMessage[]>([]);
  const [tools, setTools] = useState<BeamChatTool[]>([]);
  const [input, setInput] = useState('');
  const [pending, setPending] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [streaming, setStreaming] = useState<{ content: string; thinking: string } | null>(null);
  const [liveToolCalls, setLiveToolCalls] = useState<BeamChatToolCall[]>([]);
  const [pendingApproval, setPendingApproval] = useState<PendingApproval | null>(null);
  const activeSessionRef = useRef<string | null>(null);

  const selected = useMemo(
    () => sessions.find(session => session.id === selectedId) ?? null,
    [selectedId, sessions],
  );
  const aiReady = Boolean(readiness.aiReady);
  const composerDisabled = loadState !== 'ready' || pending || !selected || !aiReady;

  const selectSession = useCallback(
    async (id: string) => {
      setPending(true);
      setErr(null);
      try {
        const result = await client.getSession(id);
        if (result.session) {
          setSessions(current => upsertSession(current, result.session));
        }
        setSelectedId(id);
        setMessages(result.messages ?? []);
      } catch (error) {
        setErr(error instanceof Error ? error.message : String(error));
      } finally {
        setPending(false);
      }
    },
    [client],
  );

  const lastRequestedSessionId = useRef<string | null>(null);
  useEffect(() => {
    if (
      !selectSessionId ||
      selectSessionId === lastRequestedSessionId.current ||
      selectSessionId === selectedId
    ) {
      return;
    }
    lastRequestedSessionId.current = selectSessionId;
    void selectSession(selectSessionId);
  }, [selectSessionId, selectedId, selectSession]);

  const loadInitial = useCallback(async () => {
    setLoadState('loading');
    setErr(null);
    try {
      const [nextSessions, nextTools] = await Promise.all([
        client.listSessions(),
        client.listTools().catch(() => [] as BeamChatTool[]),
      ]);
      setLoadState('ready');
      setSessions(nextSessions);
      setTools(nextTools);

      const first = nextSessions[0] ?? null;
      if (first) {
        await selectSession(first.id);
      } else {
        setSelectedId(null);
        setMessages([]);
      }
    } catch (error) {
      setLoadState('unavailable');
      setSessions([]);
      setTools([]);
      setSelectedId(null);
      setMessages([]);
      setErr(error instanceof Error ? error.message : String(error));
    }
  }, [client, selectSession]);

  const createSession = useCallback(async () => {
    if (!aiReady) return;
    setPending(true);
    setErr(null);
    try {
      const result = await client.createSession({ title: '' });
      if (!result.session) {
        await loadInitial();
        return;
      }
      setSessions(current => upsertSession(current, result.session));
      setSelectedId(result.session.id);
      setMessages(result.messages ?? []);
    } catch (error) {
      setErr(error instanceof Error ? error.message : String(error));
    } finally {
      setPending(false);
    }
  }, [aiReady, client, loadInitial]);

  const deleteSession = useCallback(
    async (session: BeamChatSession) => {
      if (!client.deleteSession) return;
      const confirmed = confirmDeleteSession
        ? await confirmDeleteSession(session)
        : window.confirm(`Delete "${sessionTitle(session)}"?`);
      if (!confirmed) return;
      setPending(true);
      setErr(null);
      try {
        await client.deleteSession(session.id);
        const next = sessions.filter(item => item.id !== session.id);
        setSessions(next);
        if (selectedId === session.id) {
          const nextSelected = next[0] ?? null;
          if (nextSelected) {
            await selectSession(nextSelected.id);
          } else {
            setSelectedId(null);
            setMessages([]);
          }
        }
      } catch (error) {
        setErr(error instanceof Error ? error.message : String(error));
      } finally {
        setPending(false);
      }
    },
    [client, confirmDeleteSession, selectedId, selectSession, sessions],
  );

  const renameSession = useCallback(
    async (session: BeamChatSession) => {
      if (!client.renameSession) return;
      const current = sessionTitle(session);
      const title = (
        promptRenameSession
          ? await promptRenameSession(session, current)
          : window.prompt('Session name', current)
      )?.trim();
      if (!title || title === current) return;
      setPending(true);
      setErr(null);
      try {
        const result = await client.renameSession(session.id, { title });
        if (result.session) {
          setSessions(currentSessions => upsertSession(currentSessions, result.session));
        } else {
          await loadInitial();
        }
        if (result.messages && selectedId === session.id) {
          setMessages(result.messages);
        }
      } catch (error) {
        setErr(error instanceof Error ? error.message : String(error));
      } finally {
        setPending(false);
      }
    },
    [client, loadInitial, promptRenameSession, selectedId],
  );

  const sendMessage = useCallback(
    async (event?: FormEvent, overrideContent?: string, overrideSessionId?: string) => {
      event?.preventDefault();
      const content = (overrideContent ?? input).trim();
      const sessionId = overrideSessionId ?? selected?.id;
      if (!aiReady || !sessionId || !content) return;

      activeSessionRef.current = sessionId;
      setPending(true);
      setErr(null);
      setStreaming({ content: '', thinking: '' });
      setLiveToolCalls([]);
      const collectedToolCalls: BeamChatToolCall[] = [];

      const handlers: BeamChatTurnHandlers = {
        onDelta: chunk => {
          setStreaming(current => ({
            content: (current?.content ?? '') + (chunk.content ?? ''),
            thinking: (current?.thinking ?? '') + (chunk.thinking ?? ''),
          }));
        },
        onToolCall: call => {
          const index = collectedToolCalls.findIndex(item => item.id === call.id);
          if (index >= 0) {
            collectedToolCalls[index] = call;
          } else {
            collectedToolCalls.push(call);
          }
          setLiveToolCalls([...collectedToolCalls]);
        },
        onApprovalRequest: request =>
          new Promise<string | undefined>(resolve => {
            setPendingApproval({ ...request, resolve });
          }),
      };

      try {
        const result = await client.sendMessage(sessionId, { content }, handlers);
        setInput('');
        if (result.session) {
          setSessions(current => upsertSession(current, result.session));
        }
        if (result.messages) {
          const withToolCalls = collectedToolCalls.length
            ? attachToolCallsToLastAssistantMessage(result.messages, collectedToolCalls)
            : result.messages;
          setMessages(withToolCalls);
        } else {
          await selectSession(sessionId);
        }
      } catch (error) {
        setErr(error instanceof Error ? error.message : String(error));
      } finally {
        setPending(false);
        setStreaming(null);
        setLiveToolCalls([]);
        setPendingApproval(null);
        activeSessionRef.current = null;
      }
    },
    [aiReady, client, input, selectSession, selected],
  );

  const cancelTurn = useCallback(() => {
    if (!client.cancelTurn || !activeSessionRef.current) return;
    client.cancelTurn(activeSessionRef.current);
  }, [client]);

  const resolveApproval = useCallback(
    (optionId: string | undefined) => {
      setPendingApproval(current => {
        current?.resolve(optionId);
        return null;
      });
    },
    [],
  );

  useEffect(() => {
    void loadInitial();
  }, [loadInitial]);

  const lastComposerActionNonce = useRef<string | null>(null);
  useEffect(() => {
    if (!composerAction || composerAction.nonce === lastComposerActionNonce.current) return;
    lastComposerActionNonce.current = composerAction.nonce;
    if (!composerAction.submit) {
      setInput(composerAction.content);
      onComposerActionHandled?.();
      return;
    }
    void (async () => {
      let sessionId = selected?.id;
      if (!sessionId && aiReady) {
        const result = await client.createSession({ title: '' }).catch(() => undefined);
        if (result?.session) {
          setSessions(current => upsertSession(current, result.session));
          setSelectedId(result.session.id);
          setMessages(result.messages ?? []);
          sessionId = result.session.id;
        }
      }
      if (sessionId) {
        await sendMessage(undefined, composerAction.content, sessionId);
      }
      onComposerActionHandled?.();
    })();
  }, [aiReady, client, composerAction, onComposerActionHandled, selected, sendMessage]);

  const statusBadge =
    loadState === 'ready' ? 'Ready' : loadState === 'loading' ? 'Loading' : 'Not connected';
  const composerPlaceholderText = composerPlaceholder(
    loadState,
    aiReady,
    selected,
    productName,
    embedded,
  );
  const subtitle = statusSubtitle(readiness, productName, embedded);

  const errorNotice = err ? (
    <InlineNotice
      variant={loadState === 'unavailable' ? 'warning' : 'error'}
      className={embedded ? 'shrink-0' : 'm-4 mb-0'}>
      <div className="flex items-start gap-2">
        <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
        <span>{err}</span>
      </div>
    </InlineNotice>
  ) : null;

  const conversation = (
    <ConversationPane
      loadState={loadState}
      messages={messages}
      onCreate={createSession}
      readiness={readiness}
      selected={selected}
      links={links}
      streaming={streaming}
      liveToolCalls={liveToolCalls}
      pendingApproval={pendingApproval}
      onResolveApproval={resolveApproval}
      onOpenDiff={client.openDiff}
      productName={productName}
      embedded={embedded}
    />
  );

  const composerBlock = (
    <div className="border-surface-200 dark:border-dark-surface-700 shrink-0 border-t">
      <ChatComposer
        value={input}
        onChange={setInput}
        onSubmit={sendMessage}
        disabled={composerDisabled}
        isPending={pending}
        shell="plain"
        variant={embedded ? 'compact' : 'default'}
        title=""
        placeholder={composerPlaceholderText}
        submitLabel="Send"
        pendingLabel="Sending"
        showCharCount={!embedded}
        className={embedded ? 'px-3 py-2' : undefined}
        textareaProps={{ 'aria-label': 'Message' }}
      />
      {pending && client.cancelTurn ? (
        <div className={embedded ? 'flex justify-end px-3 pb-2' : 'flex justify-end px-4 pb-3'}>
          <Button onClick={cancelTurn} size="sm" type="button" variant="outline">
            <X className="mr-2 h-4 w-4" />
            Cancel
          </Button>
        </div>
      ) : null}
    </div>
  );

  if (embedded) {
    const runtimeLabel = runtimeChipLabel(runtimeSummary);
    const runtimeTitle = runtimeSummary
      ? [
          runtimeSummary.provider ? `Provider: ${runtimeSummary.provider}` : undefined,
          runtimeSummary.model ? `Model: ${runtimeSummary.model}` : undefined,
          runtimeSummary.think ? `Thinking: ${runtimeSummary.think}` : undefined,
          runtimeSummary.hitlPolicy ? `HITL: ${runtimeSummary.hitlPolicy}` : undefined,
        ]
          .filter(Boolean)
          .join('\n')
      : 'Runtime settings';

    return (
      <div className="flex h-full min-h-0 min-w-0 flex-col">
        <header className="border-surface-200 dark:border-dark-surface-700 shrink-0 space-y-1.5 border-b px-3 py-2">
          <div className="flex min-w-0 items-center gap-1.5">
            <label className="sr-only" htmlFor="beam-embedded-session">
              Active session
            </label>
            <select
              className="border-surface-200 bg-surface-50 text-text hover:border-primary-500/40 focus:border-primary-500 focus:ring-primary-500/30 dark:border-dark-surface-700 dark:bg-dark-surface-200 dark:text-dark-text min-w-0 flex-1 truncate rounded-md border px-2 py-1.5 text-sm focus:ring-2 focus:outline-none"
              disabled={pending || loadState !== 'ready' || sessions.length === 0}
              id="beam-embedded-session"
              onChange={event => void selectSession(event.target.value)}
              value={selectedId ?? ''}>
              {sessions.length === 0 ? (
                <option value="">No sessions yet</option>
              ) : (
                sessions.map(session => (
                  <option key={session.id} value={session.id}>
                    {sessionTitle(session)}
                  </option>
                ))
              )}
            </select>
            <Button
              aria-label={`New ${productName} session`}
              disabled={loadState !== 'ready' || pending || !aiReady}
              onClick={() => void createSession()}
              size="icon"
              type="button"
              variant="outline">
              <MessageSquarePlus className="h-4 w-4" />
            </Button>
            {selected && client.deleteSession ? (
              <Button
                aria-label={`Delete ${sessionTitle(selected)}`}
                disabled={pending}
                onClick={() => void deleteSession(selected)}
                size="icon"
                type="button"
                variant="ghost">
                <Trash2 className="h-4 w-4" />
              </Button>
            ) : null}
          </div>
          <div className="flex min-w-0 items-center gap-2">
            {onOpenRuntimeSettings ? (
              <button
                className="border-surface-200 bg-surface-50 text-text hover:bg-surface-100 dark:border-dark-surface-700 dark:bg-dark-surface-200 dark:hover:bg-dark-surface-300 dark:text-dark-text min-w-0 flex-1 truncate rounded-md border px-2 py-1 text-left text-xs"
                onClick={onOpenRuntimeSettings}
                title={runtimeTitle}
                type="button">
                {runtimeLabel}
              </button>
            ) : (
              <Span variant="muted" className="min-w-0 flex-1 truncate text-xs">
                {productName}
              </Span>
            )}
            {runtimeSummary && runtimeSummary.contextSize && runtimeSummary.contextSize > 0 ? (() => {
              const used = runtimeSummary.contextUsed || 0;
              const size = runtimeSummary.contextSize;
              const pct = Math.round((used / size) * 100);
              const cls = pct > 90 ? 'text-red-500' : pct > 70 ? 'text-yellow-500' : 'text-text-muted dark:text-dark-text-muted';
              return (
                <span className={`ml-1 text-[10px] tabular-nums ${cls}`} title={`Context: ${used.toLocaleString()} / ${size.toLocaleString()} tokens (${pct}%)`}>
                  {used.toLocaleString()}/{size.toLocaleString()} ({pct}%)
                </span>
              );
            })() : null}
            <Badge variant={loadState === 'ready' ? 'outline' : 'secondary'} size="sm">
              {subtitle}
            </Badge>
          </div>
        </header>

        {errorNotice}
        {conversation}
        {composerBlock}
      </div>
    );
  }

  return (
    <div className="grid min-h-[42rem] min-w-0 grid-cols-1 gap-4 lg:grid-cols-[19rem_1fr]">
      <Panel variant="surface" className="flex min-h-0 min-w-0 flex-col p-0">
        <div className="border-surface-200 dark:border-dark-surface-700 flex items-center justify-between border-b p-3">
          <div>
            <h2 className="text-sm font-semibold">Sessions</h2>
            <Span variant="muted" className="text-xs">
              {sessions.length} sessions
            </Span>
          </div>
          <Button
            aria-label={`New ${productName} session`}
            disabled={loadState !== 'ready' || pending || !aiReady}
            onClick={() => void createSession()}
            size="icon"
            type="button">
            <MessageSquarePlus className="h-4 w-4" />
          </Button>
        </div>

        <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2" aria-label={`${productName} sessions`}>
          {loadState === 'loading' ? <ChatThreadSkeleton rows={3} /> : null}
          {loadState === 'ready' && sessions.length === 0 ? (
            <Panel
              variant="empty"
              className="border-surface-200 dark:border-dark-surface-700 rounded-md border border-dashed p-4">
              <Span variant="muted" className="text-sm">
                No sessions yet.
              </Span>
            </Panel>
          ) : null}
          {sessions.map(session => (
            <div
              className={[
                'group flex items-center gap-1 rounded-lg border p-1',
                session.id === selectedId
                  ? 'border-primary-500/60 bg-surface-100 text-text dark:border-dark-primary-500/50 dark:bg-dark-surface-300 dark:text-dark-text'
                  : 'text-text hover:bg-surface-100 dark:text-dark-text dark:hover:bg-dark-surface-200 border-transparent',
              ].join(' ')}
              key={session.id}>
              <button
                className="min-w-0 flex-1 rounded px-2 py-2 text-left"
                disabled={pending}
                onClick={() => void selectSession(session.id)}
                type="button">
                <div className="truncate text-sm font-medium">{sessionTitle(session)}</div>
                <div className="truncate text-xs opacity-70">
                  {fmtDate(session.lastMessageAt ?? session.updatedAt)}
                </div>
              </button>
              {client.renameSession ? (
                <Button
                  aria-label={`Rename ${sessionTitle(session)}`}
                  className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
                  disabled={pending}
                  onClick={() => void renameSession(session)}
                  size="icon"
                  type="button"
                  variant="ghost">
                  <Pencil className="h-4 w-4" />
                </Button>
              ) : null}
              {client.deleteSession ? (
                <Button
                  aria-label={`Delete ${sessionTitle(session)}`}
                  className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
                  disabled={pending}
                  onClick={() => void deleteSession(session)}
                  size="icon"
                  type="button"
                  variant="ghost">
                  <Trash2 className="h-4 w-4" />
                </Button>
              ) : null}
            </div>
          ))}
        </nav>

        <div className="border-surface-200 dark:border-dark-surface-700 border-t p-3">
          <ContextReadiness links={links} readiness={readiness} productName={productName} runtimeSummary={runtimeSummary} />
          <ToolSummary tools={tools} unavailable={loadState === 'unavailable'} />
        </div>
      </Panel>

      <Panel variant="surface" className="flex min-h-0 min-w-0 flex-col p-0">
        <div className="border-surface-200 dark:border-dark-surface-700 flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              {productIcon ?? <Bot className="h-5 w-5 opacity-70" />}
              <h2 className="truncate text-base font-semibold">
                {selected ? sessionTitle(selected) : productName}
              </h2>
              <Badge variant={loadState === 'ready' ? 'outline' : 'secondary'} size="sm">
                {statusBadge}
              </Badge>
              {runtimeSummary && runtimeSummary.contextSize && runtimeSummary.contextSize > 0 ? (() => {
                const used = runtimeSummary.contextUsed || 0;
                const size = runtimeSummary.contextSize;
                const pct = Math.round((used / size) * 100);
                const cls = pct > 90 ? 'text-red-500' : pct > 70 ? 'text-yellow-500' : 'text-text-muted dark:text-dark-text-muted';
                return (
                  <span className={`ml-1 text-[10px] tabular-nums ${cls}`} title={`Context: ${used.toLocaleString()} / ${size.toLocaleString()} tokens (${pct}%)`}>
                    {used.toLocaleString()}/{size.toLocaleString()} ({pct}%)
                  </span>
                );
              })() : null}
            </div>
            <Span variant="muted" className="mt-1 block text-sm">
              {subtitle}
            </Span>
          </div>
          <Button
            disabled={loadState === 'loading'}
            onClick={() => void loadInitial()}
            size="sm"
            type="button"
            variant="outline">
            <RefreshCw className="mr-2 h-4 w-4" />
            Refresh
          </Button>
        </div>

        {errorNotice}
        {conversation}
        {composerBlock}
      </Panel>
    </div>
  );
}

function ConversationPane({
  links,
  loadState,
  messages,
  onCreate,
  readiness,
  selected,
  streaming,
  liveToolCalls,
  pendingApproval,
  onResolveApproval,
  onOpenDiff,
  productName,
  embedded = false,
}: {
  links: BeamChatLinks;
  loadState: LoadState;
  messages: BeamChatMessage[];
  onCreate: () => Promise<void>;
  readiness: BeamChatReadiness;
  selected: BeamChatSession | null;
  streaming: { content: string; thinking: string } | null;
  liveToolCalls: BeamChatToolCall[];
  pendingApproval: PendingApproval | null;
  onResolveApproval: (optionId: string | undefined) => void;
  onOpenDiff?: (call: BeamChatToolCall) => void;
  productName: string;
  embedded?: boolean;
}) {
  const { containerRef, endRef, scrollToEnd, isNearBottom } = useChatScroll({
    deps: [messages, loadState, streaming, liveToolCalls, pendingApproval],
  });

  if (loadState === 'loading') {
    return (
      <div className="min-h-0 flex-1 p-3 sm:p-4">
        <ChatThreadSkeleton />
      </div>
    );
  }

  if (loadState === 'unavailable') {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center p-4 sm:p-6">
        <UnavailableState
          links={links}
          readiness={readiness}
          productName={productName}
          embedded={embedded}
        />
      </div>
    );
  }

  if (!selected) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center p-4 sm:p-6">
        <Panel variant="empty" className="max-w-lg text-center">
          <div className="bg-surface-100 dark:bg-dark-surface-200 mx-auto mb-4 w-fit rounded-full p-4">
            <MessageSquarePlus className="h-8 w-8 opacity-70" />
          </div>
          <h3 className="text-lg font-semibold">No session selected</h3>
          <Span variant="muted" className="mt-2 block text-sm">
            Start a new session to chat with {productName}.
          </Span>
          <Button
            className="mt-5"
            disabled={!readiness.aiReady}
            onClick={() => void onCreate()}
            type="button">
            <MessageSquarePlus className="mr-2 h-4 w-4" />
            New session
          </Button>
        </Panel>
      </div>
    );
  }

  return (
    <div className="relative min-h-0 flex-1">
      <ChatThread
        containerRef={containerRef}
        endRef={endRef}
        className="h-full"
        scrollClassName={
          embedded
            ? 'flex-1 space-y-3 overflow-auto px-3 py-3'
            : 'flex-1 space-y-4 overflow-auto px-4 py-4 sm:px-5'
        }>
        {messages.map((message, index) => {
          const prev = index > 0 ? messages[index - 1] : null;
          const showSeparator = !prev || dateKey(prev.createdAt) !== dateKey(message.createdAt);
          return (
            <div key={message.id} className="animate-in fade-in-0 space-y-4 duration-150">
              {showSeparator ? (
                <ChatDateSeparator label={formatDateLabel(message.createdAt)} />
              ) : null}
              <ChatMessageView
                message={message}
                isLatest={index === messages.length - 1 && !streaming}
                onOpenDiff={onOpenDiff}
                productName={productName}
              />
            </div>
          );
        })}
        {streaming ? (
          <StreamingMessageView
            streaming={streaming}
            toolCalls={liveToolCalls}
            productName={productName}
          />
        ) : null}
        {pendingApproval ? (
          <ApprovalCard request={pendingApproval} onResolve={onResolveApproval} />
        ) : null}
      </ChatThread>
      <ChatScrollToLatest visible={!isNearBottom} onClick={scrollToEnd} label="Scroll to latest" />
    </div>
  );
}

function ChatMessageView({
  message,
  isLatest,
  onOpenDiff,
  productName,
}: {
  message: BeamChatMessage;
  isLatest: boolean;
  onOpenDiff?: (call: BeamChatToolCall) => void;
  productName: string;
}) {
  const roleLabel =
    message.role === 'user'
      ? 'You'
      : message.role === 'system'
        ? 'System'
        : message.role === 'tool'
          ? 'Tool'
          : productName;

  return (
    <ChatMessageUI
      appearance="transcript"
      role={message.role}
      roleLabel={roleLabel}
      timestamp={new Date(message.createdAt).toLocaleTimeString()}
      timestampTooltip={new Date(message.createdAt).toLocaleString()}
      isLatest={isLatest}
      latestLabel={isLatest ? 'Latest' : undefined}
      copyText={message.content}
      copyLabel="Copy"
      copiedLabel="Copied"
      error={message.error}
      collapseToggleLabel={{ open: 'Hide', closed: 'Show' }}>
      {message.content ? (
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={chatTranscriptMarkdownComponents}>
          {message.content}
        </ReactMarkdown>
      ) : null}
      {message.citations?.length ? (
        <div className="mt-3 flex flex-wrap gap-2">
          {message.citations.map((citation, index) => (
            <span
              className="border-surface-300 dark:border-dark-surface-600 inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-1 text-xs"
              key={`${citation.title ?? citation.url ?? citation.path ?? 'citation'}-${index}`}>
              <FileText className="h-3 w-3 shrink-0" />
              <span className="truncate">
                {citation.title || citation.path || citation.source || citation.url}
              </span>
            </span>
          ))}
        </div>
      ) : null}
      {message.toolCalls?.length ? (
        <div className="mt-3 space-y-2">
          {message.toolCalls.map(call => (
            <ToolCallCard call={call} key={call.id} onOpenDiff={onOpenDiff} />
          ))}
        </div>
      ) : null}
    </ChatMessageUI>
  );
}

function StreamingMessageView({
  streaming,
  toolCalls,
  productName,
}: {
  streaming: { content: string; thinking: string };
  toolCalls: BeamChatToolCall[];
  productName: string;
}) {
  return (
    <ChatMessageUI
      appearance="transcript"
      role="assistant"
      roleLabel={productName}
      isLatest
      latestLabel="Latest"
      collapseToggleLabel={{ open: 'Hide', closed: 'Show' }}>
      {streaming.content ? (
        <ReactMarkdown remarkPlugins={[remarkGfm]} components={chatTranscriptMarkdownComponents}>
          {streaming.content}
        </ReactMarkdown>
      ) : (
        <span className="text-text-muted dark:text-dark-text-muted flex items-center gap-2 text-sm">
          <Loader2 className="h-3.5 w-3.5 animate-spin" />
          {streaming.thinking ? 'Thinking…' : 'Working…'}
        </span>
      )}
      {toolCalls.length ? (
        <div className="mt-3 space-y-2">
          {toolCalls.map(call => (
            <ToolCallCard call={call} key={call.id} />
          ))}
        </div>
      ) : null}
    </ChatMessageUI>
  );
}

function ToolCallCard({
  call,
  onOpenDiff,
}: {
  call: BeamChatToolCall;
  onOpenDiff?: (call: BeamChatToolCall) => void;
}) {
  return (
    <div className="border-surface-200 dark:border-dark-surface-700 rounded-md border px-3 py-2 text-sm">
      <div className="flex items-center justify-between gap-2">
        <span className="flex min-w-0 items-center gap-2">
          {call.status === 'running' ? (
            <Loader2 className="h-3.5 w-3.5 shrink-0 animate-spin opacity-70" />
          ) : call.status === 'error' ? (
            <AlertTriangle className="text-error-500 h-3.5 w-3.5 shrink-0" />
          ) : (
            <Wrench className="h-3.5 w-3.5 shrink-0 opacity-70" />
          )}
          <span className="truncate font-medium">
            {call.title ?? call.toolName ?? 'Tool call'}
          </span>
        </span>
        <Badge variant={call.status === 'error' ? 'secondary' : 'outline'} size="sm">
          {call.status}
        </Badge>
      </div>
      {call.output ? (
        <pre className="bg-surface-100 dark:bg-dark-surface-200 mt-2 max-h-40 overflow-auto rounded p-2 text-xs whitespace-pre-wrap">
          {call.output}
        </pre>
      ) : null}
      {call.error ? <p className="text-error-500 mt-2 text-xs">{call.error}</p> : null}
      {call.diff && onOpenDiff ? (
        <Button
          className="mt-2"
          onClick={() => onOpenDiff(call)}
          size="sm"
          type="button"
          variant="outline">
          <FileDiff className="mr-2 h-3.5 w-3.5" />
          Open Diff
        </Button>
      ) : null}
    </div>
  );
}

function ApprovalCard({
  request,
  onResolve,
}: {
  request: BeamChatApprovalRequest;
  onResolve: (optionId: string | undefined) => void;
}) {
  return (
    <div className="border-warning-200 bg-warning-50 text-warning-900 dark:border-dark-surface-500 dark:bg-dark-surface-300/40 dark:text-dark-text rounded-md border p-3 text-sm">
      <div className="flex items-center gap-2 font-medium">
        <AlertTriangle className="text-warning-900 dark:text-dark-text h-4 w-4 shrink-0" />
        {request.title}
      </div>
      {request.details ? (
        <p className="text-text-muted dark:text-dark-text-muted mt-1 text-xs">
          {request.details}
        </p>
      ) : null}
      {request.diff ? (
        <div className="mt-2 grid gap-2 sm:grid-cols-2">
          {request.diff.before ? (
            <pre className="bg-surface-100 dark:bg-dark-surface-200 max-h-32 overflow-auto rounded p-2 text-xs whitespace-pre-wrap">
              {request.diff.before}
            </pre>
          ) : null}
          {request.diff.after ? (
            <pre className="bg-surface-100 dark:bg-dark-surface-200 max-h-32 overflow-auto rounded p-2 text-xs whitespace-pre-wrap">
              {request.diff.after}
            </pre>
          ) : null}
        </div>
      ) : null}
      <div className="mt-3 flex flex-wrap gap-2">
        {request.options.map(option => (
          <Button
            key={option.id}
            onClick={() => onResolve(option.id)}
            size="sm"
            type="button"
            variant={option.kind.startsWith('allow') ? 'primary' : 'outline'}>
            {option.kind.startsWith('allow') ? (
              <Check className="mr-2 h-3.5 w-3.5" />
            ) : (
              <X className="mr-2 h-3.5 w-3.5" />
            )}
            {option.label}
          </Button>
        ))}
      </div>
    </div>
  );
}

function ContextReadiness({
  links,
  readiness,
  productName,
  runtimeSummary,
}: {
  links: BeamChatLinks;
  readiness: BeamChatReadiness;
  productName: string;
  runtimeSummary?: BeamChatRuntimeSummary | null;
}) {
  const items = [
    {
      icon: SlidersHorizontal,
      label: `${productName} setup`,
      ready: Boolean(readiness.aiReady),
      value: readiness.aiReady ? 'Ready' : 'Needs setup',
      href: links.ai ?? defaultLinks.ai,
      disabled: !readiness.canManage,
    },
    {
      icon: Search,
      label: 'Search',
      ready: readiness.searchReady,
      value: readiness.searchReady ? 'Ready' : 'Needs setup',
      href: links.search,
    },
    {
      icon: Database,
      label: 'Sources',
      ready: readiness.syncedSourceCount > 0,
      value:
        readiness.sourceCount > 0
          ? `${readiness.syncedSourceCount}/${readiness.sourceCount} synced`
          : 'None',
      href: links.sources,
      disabled: !readiness.canManage,
    },
    {
      icon: Package,
      label: 'Apps',
      ready: readiness.appCount > 0,
      value: readiness.appCount > 0 ? String(readiness.appCount) : 'None',
      href: links.apps,
    },
  ];
  if (runtimeSummary && runtimeSummary.contextSize && runtimeSummary.contextSize > 0) {
    const used = runtimeSummary.contextUsed || 0;
    const size = runtimeSummary.contextSize;
    const pct = Math.round((used / size) * 100);
    items.push({
      icon: SlidersHorizontal, // reuse or import a new
      label: 'Context',
      ready: pct < 90,
      value: `${used.toLocaleString()}/${size.toLocaleString()} (${pct}%)`,
      href: '#',
      disabled: true,
    } as any);
  }

  return (
    <div className="space-y-2">
      <div className="text-text-muted dark:text-dark-text-muted text-xs font-medium uppercase">
        Context
      </div>
      {items.map(item => {
        const Icon = item.icon;
        const content = (
          <>
            <span className="flex min-w-0 items-center gap-2">
              <Icon className="h-4 w-4 shrink-0 opacity-70" />
              <span className="truncate">{item.label}</span>
            </span>
            <Badge variant={item.ready ? 'success' : 'outline'} size="sm">
              {item.value}
            </Badge>
          </>
        );

        if (item.disabled) {
          return (
            <div
              className="border-surface-200 text-text-muted dark:border-dark-surface-700 dark:text-dark-text-muted flex items-center justify-between rounded-md border px-2 py-2 text-sm"
              key={item.label}>
              {content}
            </div>
          );
        }

        return (
          <a
            className="border-surface-200 dark:border-dark-surface-700 hover:bg-surface-50 dark:hover:bg-dark-surface-200 flex items-center justify-between rounded-md border px-2 py-2 text-sm"
            href={item.href}
            key={item.label}>
            {content}
          </a>
        );
      })}
    </div>
  );
}

function ToolSummary({ tools, unavailable }: { tools: BeamChatTool[]; unavailable: boolean }) {
  return (
    <div className="mt-4 space-y-2">
      <div className="text-text-muted dark:text-dark-text-muted text-xs font-medium uppercase">
        Tools
      </div>
      {unavailable ? (
        <Panel
          variant="empty"
          className="border-surface-200 dark:border-dark-surface-700 rounded-md border border-dashed px-2 py-2">
          <Span variant="muted" className="text-sm">
            Runtime tools unavailable.
          </Span>
        </Panel>
      ) : tools.length > 0 ? (
        tools.slice(0, 4).map(tool => (
          <div
            className="border-surface-200 dark:border-dark-surface-700 flex items-center justify-between rounded-md border px-2 py-2 text-sm"
            key={tool.id}>
            <span className="flex min-w-0 items-center gap-2">
              <Wrench className="h-4 w-4 shrink-0 opacity-70" />
              <span className="truncate">{tool.label}</span>
            </span>
            <Badge variant={tool.enabled ? 'outline' : 'secondary'} size="sm">
              {tool.mode}
            </Badge>
          </div>
        ))
      ) : (
        <Panel
          variant="empty"
          className="border-surface-200 dark:border-dark-surface-700 rounded-md border border-dashed px-2 py-2">
          <Span variant="muted" className="text-sm">
            No tools enabled.
          </Span>
        </Panel>
      )}
    </div>
  );
}

function UnavailableState({
  links,
  readiness,
  productName,
  embedded = false,
}: {
  links: BeamChatLinks;
  readiness: BeamChatReadiness;
  productName: string;
  embedded?: boolean;
}) {
  return (
    <Panel variant="empty" className="max-w-xl text-center">
      <div className="bg-surface-100 dark:bg-dark-surface-200 mx-auto mb-4 w-fit rounded-full p-4">
        {productIcon ?? <Bot className="h-8 w-8 opacity-70" />}
      </div>
      <h3 className="text-lg font-semibold">{productName} is not connected yet</h3>
      <Span variant="muted" className="mt-2 block text-sm">
        {embedded
          ? `The ${productName} runtime is not responding. Use the Runtime view or run setup, then refresh.`
          : `The ${productName} API is not responding. Refresh the page or check the workspace setup.`}
      </Span>
      {embedded ? null : (
        <div className="mt-6 flex flex-wrap justify-center gap-2">
          {readiness.canManage ? (
            <a className="text-primary hover:underline" href={links.ai ?? defaultLinks.ai}>
              {productName} setup
            </a>
          ) : null}
          <a className="text-primary hover:underline" href={links.search}>
            {readiness.searchReady ? 'Open search' : 'Prepare search'}
          </a>
          {readiness.canManage ? (
            <a className="text-primary hover:underline" href={links.sources}>
              Sources
            </a>
          ) : null}
          <a className="text-primary hover:underline" href={links.apps}>
            Apps
          </a>
        </div>
      )}
    </Panel>
  );
}

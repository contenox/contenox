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
import {
  AlertTriangle,
  Bot,
  Database,
  FileText,
  MessageSquarePlus,
  Package,
  RefreshCw,
  Search,
  SlidersHorizontal,
  Trash2,
  Wrench,
} from 'lucide-react';
import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

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
  error?: string;
};

export type BeamChatTool = {
  id: string;
  label: string;
  mode: 'read' | 'mutate' | string;
  enabled: boolean;
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
  deleteSession?: (id: string) => Promise<void>;
  sendMessage: (id: string, input: { content: string }) => Promise<BeamChatMessageResponse>;
  listTools: () => Promise<BeamChatTool[]>;
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

function sessionTitle(session: BeamChatSession): string {
  return session.title?.trim() || 'New session';
}

function upsertSession(
  sessions: BeamChatSession[],
  session?: BeamChatSession,
): BeamChatSession[] {
  if (!session) return sessions;
  const next = [session, ...sessions.filter(item => item.id !== session.id)];
  return next.sort((a, b) => {
    const av = new Date(a.lastMessageAt ?? a.updatedAt).getTime();
    const bv = new Date(b.lastMessageAt ?? b.updatedAt).getTime();
    return bv - av;
  });
}

export function BeamChat({
  client,
  links = defaultLinks,
  readiness,
}: {
  client: BeamChatClient;
  links?: BeamChatLinks;
  readiness: BeamChatReadiness;
}) {
  const [loadState, setLoadState] = useState<LoadState>('loading');
  const [sessions, setSessions] = useState<BeamChatSession[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [messages, setMessages] = useState<BeamChatMessage[]>([]);
  const [tools, setTools] = useState<BeamChatTool[]>([]);
  const [input, setInput] = useState('');
  const [pending, setPending] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const selected = useMemo(
    () => sessions.find(session => session.id === selectedId) ?? null,
    [selectedId, sessions],
  );
  const composerDisabled = loadState !== 'ready' || pending || !selected;

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
    setPending(true);
    setErr(null);
    try {
      const result = await client.createSession({ title: 'New session' });
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
  }, [client, loadInitial]);

  const deleteSession = useCallback(
    async (session: BeamChatSession) => {
      if (!client.deleteSession) return;
      if (!window.confirm(`Delete "${sessionTitle(session)}"?`)) return;
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
    [client, selectedId, selectSession, sessions],
  );

  const sendMessage = useCallback(
    async (event?: FormEvent) => {
      event?.preventDefault();
      const content = input.trim();
      if (!selected || !content) return;

      setPending(true);
      setErr(null);
      try {
        const result = await client.sendMessage(selected.id, { content });
        setInput('');
        if (result.session) {
          setSessions(current => upsertSession(current, result.session));
        }
        if (result.messages) {
          setMessages(result.messages);
        } else {
          await selectSession(selected.id);
        }
      } catch (error) {
        setErr(error instanceof Error ? error.message : String(error));
      } finally {
        setPending(false);
      }
    },
    [client, input, selectSession, selected],
  );

  useEffect(() => {
    void loadInitial();
  }, [loadInitial]);

  return (
    <div className="grid min-h-[42rem] gap-4 lg:grid-cols-[19rem_1fr]">
      <Panel variant="surface" className="flex min-h-0 flex-col p-0">
        <div className="border-surface-200 dark:border-dark-surface-700 flex items-center justify-between border-b p-3">
          <div>
            <h2 className="text-sm font-semibold">Sessions</h2>
            <Span variant="muted" className="text-xs">{sessions.length} sessions</Span>
          </div>
          <Button
            aria-label="New Beam session"
            disabled={loadState !== 'ready' || pending}
            onClick={() => void createSession()}
            size="icon"
            type="button">
            <MessageSquarePlus className="h-4 w-4" />
          </Button>
        </div>

        <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-2" aria-label="Beam sessions">
          {loadState === 'loading' ? <ChatThreadSkeleton rows={3} /> : null}
          {loadState === 'ready' && sessions.length === 0 ? (
            <Panel variant="empty" className="border-surface-200 dark:border-dark-surface-700 rounded-md border border-dashed p-4">
              <Span variant="muted" className="text-sm">No sessions yet.</Span>
            </Panel>
          ) : null}
          {sessions.map(session => (
            <div
              className={[
                'group flex items-center gap-1 rounded-lg p-1',
                session.id === selectedId
                  ? 'bg-primary-600 text-white dark:bg-dark-primary-600'
                  : 'text-text hover:bg-surface-100 dark:text-dark-text dark:hover:bg-dark-surface-200',
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
              {client.deleteSession ? (
                <Button
                  aria-label={`Delete ${sessionTitle(session)}`}
                  className="opacity-0 group-hover:opacity-100"
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
          <ContextReadiness links={links} readiness={readiness} />
          <ToolSummary tools={tools} unavailable={loadState === 'unavailable'} />
        </div>
      </Panel>

      <Panel variant="surface" className="flex min-h-0 flex-col p-0">
        <div className="border-surface-200 dark:border-dark-surface-700 flex flex-col gap-3 border-b p-4 sm:flex-row sm:items-center sm:justify-between">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <Bot className="h-5 w-5 opacity-70" />
              <h2 className="truncate text-base font-semibold">{selected ? sessionTitle(selected) : 'Beam'}</h2>
              <Badge variant={loadState === 'ready' ? 'outline' : 'secondary'} size="sm">
                {loadState === 'ready' ? 'Ready' : loadState === 'loading' ? 'Loading' : 'Not connected'}
              </Badge>
            </div>
            <Span variant="muted" className="mt-1 block text-sm">
              {readiness.aiReady && readiness.searchReady
                ? 'Workspace search is ready for Beam.'
                : readiness.aiReady
                  ? 'Beam is ready. Workspace search is not prepared yet.'
                  : 'Beam setup is not complete yet.'}
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

        {err ? (
          <InlineNotice variant={loadState === 'unavailable' ? 'warning' : 'error'} className="m-4 mb-0">
            <div className="flex items-start gap-2">
              <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" />
              <span>{err}</span>
            </div>
          </InlineNotice>
        ) : null}

        <ConversationPane
          loadState={loadState}
          messages={messages}
          onCreate={createSession}
          readiness={readiness}
          selected={selected}
          links={links}
        />

        <div className="border-surface-200 dark:border-dark-surface-700 border-t">
          <ChatComposer
            value={input}
            onChange={setInput}
            onSubmit={sendMessage}
            disabled={composerDisabled}
            isPending={pending}
            shell="plain"
            variant="default"
            title=""
            placeholder={
              loadState === 'unavailable'
                ? 'Beam is not connected yet.'
                : selected
                  ? 'Ask about this workspace'
                  : 'Create a session to start'
            }
            submitLabel="Send"
            pendingLabel="Sending"
            textareaProps={{ 'aria-label': 'Message' }}
          />
        </div>
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
}: {
  links: BeamChatLinks;
  loadState: LoadState;
  messages: BeamChatMessage[];
  onCreate: () => Promise<void>;
  readiness: BeamChatReadiness;
  selected: BeamChatSession | null;
}) {
  const { containerRef, endRef, scrollToEnd, isNearBottom } = useChatScroll({ deps: [messages, loadState] });

  if (loadState === 'loading') {
    return (
      <div className="min-h-0 flex-1 p-4">
        <ChatThreadSkeleton />
      </div>
    );
  }

  if (loadState === 'unavailable') {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center p-6">
        <UnavailableState links={links} readiness={readiness} />
      </div>
    );
  }

  if (!selected) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center p-6">
        <Panel variant="empty" className="max-w-lg text-center">
          <div className="bg-surface-100 dark:bg-dark-surface-200 mx-auto mb-4 w-fit rounded-full p-4">
            <MessageSquarePlus className="h-8 w-8 opacity-70" />
          </div>
          <h3 className="text-lg font-semibold">No session selected</h3>
          <Button className="mt-5" onClick={() => void onCreate()} type="button">
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
        scrollClassName="flex-1 space-y-4 overflow-auto px-4 py-4 sm:px-5">
        {messages.length === 0 ? (
          <div className="flex h-full items-center justify-center">
            <Panel variant="empty" className="max-w-lg text-center">
              <div className="bg-surface-100 dark:bg-dark-surface-200 mx-auto mb-4 w-fit rounded-full p-4">
                <Bot className="h-8 w-8 opacity-70" />
              </div>
              <h3 className="text-lg font-semibold">New session</h3>
              <Span variant="muted" className="mt-2 block text-sm">
                Ask a question once the workspace has searchable context.
              </Span>
            </Panel>
          </div>
        ) : (
          messages.map((message, index) => {
            const prev = index > 0 ? messages[index - 1] : null;
            const showSeparator = !prev || dateKey(prev.createdAt) !== dateKey(message.createdAt);
            return (
              <div key={message.id} className="animate-in fade-in-0 space-y-4 duration-150">
                {showSeparator ? <ChatDateSeparator label={formatDateLabel(message.createdAt)} /> : null}
                <ChatMessageView message={message} isLatest={index === messages.length - 1} />
              </div>
            );
          })
        )}
      </ChatThread>
      <ChatScrollToLatest visible={!isNearBottom} onClick={scrollToEnd} label="Scroll to latest" />
    </div>
  );
}

function ChatMessageView({
  message,
  isLatest,
}: {
  message: BeamChatMessage;
  isLatest: boolean;
}) {
  const roleLabel =
    message.role === 'user'
      ? 'You'
      : message.role === 'system'
        ? 'System'
        : message.role === 'tool'
          ? 'Tool'
          : 'Beam';

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
    </ChatMessageUI>
  );
}

function ContextReadiness({
  links,
  readiness,
}: {
  links: BeamChatLinks;
  readiness: BeamChatReadiness;
}) {
  const items = [
    {
      icon: SlidersHorizontal,
      label: 'Beam setup',
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
      value: readiness.sourceCount > 0 ? `${readiness.syncedSourceCount}/${readiness.sourceCount} synced` : 'None',
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

  return (
    <div className="space-y-2">
      <div className="text-text-muted dark:text-dark-text-muted text-xs font-medium uppercase">Context</div>
      {items.map(item => {
        const Icon = item.icon;
        const content = (
          <>
            <span className="flex min-w-0 items-center gap-2">
              <Icon className="h-4 w-4 shrink-0 opacity-70" />
              <span className="truncate">{item.label}</span>
            </span>
            <Badge variant={item.ready ? 'success' : 'outline'} size="sm">{item.value}</Badge>
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

function ToolSummary({
  tools,
  unavailable,
}: {
  tools: BeamChatTool[];
  unavailable: boolean;
}) {
  return (
    <div className="mt-4 space-y-2">
      <div className="text-text-muted dark:text-dark-text-muted text-xs font-medium uppercase">Tools</div>
      {unavailable ? (
        <Panel variant="empty" className="border-surface-200 dark:border-dark-surface-700 rounded-md border border-dashed px-2 py-2">
          <Span variant="muted" className="text-sm">Runtime tools unavailable.</Span>
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
            <Badge variant={tool.enabled ? 'outline' : 'secondary'} size="sm">{tool.mode}</Badge>
          </div>
        ))
      ) : (
        <Panel variant="empty" className="border-surface-200 dark:border-dark-surface-700 rounded-md border border-dashed px-2 py-2">
          <Span variant="muted" className="text-sm">No tools enabled.</Span>
        </Panel>
      )}
    </div>
  );
}

function UnavailableState({
  links,
  readiness,
}: {
  links: BeamChatLinks;
  readiness: BeamChatReadiness;
}) {
  return (
    <Panel variant="empty" className="max-w-xl text-center">
      <div className="bg-surface-100 dark:bg-dark-surface-200 mx-auto mb-4 w-fit rounded-full p-4">
        <Bot className="h-8 w-8 opacity-70" />
      </div>
      <h3 className="text-lg font-semibold">Beam is not connected yet</h3>
      <Span variant="muted" className="mt-2 block text-sm">
        The workbench is in the Dashboard, but message execution still needs the tenant Beam API behind it.
      </Span>
      <div className="mt-6 flex flex-wrap justify-center gap-2">
        {readiness.canManage ? (
          <a className="text-primary hover:underline" href={links.ai ?? defaultLinks.ai}>
            Beam setup
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
    </Panel>
  );
}

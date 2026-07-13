import { Button, Fill, InlineNotice, Page, Spinner } from '@contenox/ui';
import { lazy, Suspense, useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useAgentTurn } from '../../../hooks/useAgentTurn';
import { useChatHistory, useChats, useCreateChat } from '../../../hooks/useChats';
import { useTerminalAvailable } from '../../../hooks/useTerminal';
import { useListChains } from '../../../hooks/useChains';
import { useListPolicies, useSetActivePolicy } from '../../../hooks/usePolicies';
import { useSetupStatus } from '../../../hooks/useSetupStatus';
import { api } from '../../../lib/api';
import { ArtifactRegistryProvider, useArtifactRegistry } from '../../../lib/artifacts';
import { artifactsToInlineAttachments } from '../../../lib/inlineAttachments';
import { isOptimisticEcho } from '../../../lib/optimisticEcho';
import { createHelpCommand } from '../../../lib/slashCommands/builtins';
import {
  SlashCommandRegistryProvider,
  useSlashCommand,
  useSlashCommandRegistry,
} from '../../../lib/slashCommands/registry';
import type { ChatContextArtifact, ChatContextPayload } from '../../../lib/types';
import { MessageInputForm } from '../chats/components/MessageInputForm';
import { useConsoleCommands } from './consoleCommands';
import { buildConsoleTurns } from './consoleTurns';
import { StatusLine } from './components/StatusLine';
import { TurnBlock } from './components/TurnBlock';

const TerminalPanel = lazy(() =>
  import('../chats/components/TerminalPanel').then(m => ({ default: m.TerminalPanel })),
);

const DEFAULT_CHAIN_PATH = 'default-chain.json';

/**
 * The console: Beam's terminal-agent surface. One stream of turns
 * (command → work → result), a command-line composer with slash commands,
 * and a status line. Scrollback retains every run's work log this session.
 */
export default function ConsolePage() {
  return (
    <ArtifactRegistryProvider>
      <SlashCommandRegistryProvider>
        <ConsolePageImpl />
      </SlashCommandRegistryProvider>
    </ArtifactRegistryProvider>
  );
}

function buildTurnContext(artifacts: ChatContextArtifact[]): ChatContextPayload | undefined {
  if (artifacts.length === 0) return undefined;
  return { artifacts: [...artifacts] };
}

function ConsolePageImpl() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { chatId: paramChatId } = useParams<{ chatId: string }>();
  const chatId = paramChatId ?? '';

  const [message, setMessage] = useState('');
  const [selectedChainId, setSelectedChainId] = useState('');
  const [terminalOpen, setTerminalOpen] = useState(false);

  const artifactRegistry = useArtifactRegistry();
  const slashRegistry = useSlashCommandRegistry();
  const helpCommand = useMemo(() => createHelpCommand(slashRegistry), [slashRegistry]);
  useSlashCommand(helpCommand);

  const { data: sessions } = useChats();
  const { data: chatHistory, isLoading: historyLoading } = useChatHistory(chatId, {
    enabled: !!chatId,
  });
  const { data: chainPaths = [] } = useListChains();
  const { data: policyNames = [] } = useListPolicies();
  const { data: setupStatus } = useSetupStatus(true);
  const { isError: terminalUnavailable } = useTerminalAvailable();
  const activePolicyName = setupStatus?.hitlPolicyName ?? '';
  const setActivePolicy = useSetActivePolicy();
  const { mutate: createChat, isPending: isCreating } = useCreateChat();

  const agent = useAgentTurn(chatId);

  const sortedChainPaths = useMemo(
    () =>
      [...chainPaths].sort((a, b) => {
        if (a === DEFAULT_CHAIN_PATH) return -1;
        if (b === DEFAULT_CHAIN_PATH) return 1;
        return a.localeCompare(b);
      }),
    [chainPaths],
  );

  useEffect(() => {
    if (selectedChainId || !sortedChainPaths.length) return;
    setSelectedChainId(sortedChainPaths[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [sortedChainPaths]);

  const newSession = useCallback(() => {
    createChat(
      {},
      {
        onSuccess: created => {
          if (created.id) navigate(`/console/${created.id}`);
        },
      },
    );
  }, [createChat, navigate]);

  useConsoleCommands({
    chainPaths: sortedChainPaths,
    selectedChainId,
    setSelectedChainId,
    policyNames,
    activePolicyName,
    setActivePolicy: name => setActivePolicy.mutate(name),
    newSession,
  });

  // /console without a session: open the most recent one or create the first.
  const autoRoutedRef = useRef(false);
  useEffect(() => {
    if (chatId || autoRoutedRef.current || !sessions) return;
    autoRoutedRef.current = true;
    const latest = sessions[0];
    if (latest?.id) {
      navigate(`/chat/${latest.id}`, { replace: true });
    } else {
      newSession();
    }
  }, [chatId, sessions, navigate, newSession]);

  // After `/chat` (landing) creates a session and navigates here with state,
  // send the first message exactly once. Ported from ChatPage.
  const landingInitialSendKeyRef = useRef<string | null>(null);
  const location = useLocation();
  useEffect(() => {
    if (!paramChatId) return;
    const st = location.state as {
      beamInitialMessage?: string;
      beamInitialChainId?: string;
    } | null;
    if (!st?.beamInitialMessage?.trim() || !st.beamInitialChainId) return;

    const text = st.beamInitialMessage.trim();
    const chain = st.beamInitialChainId;
    const dedupeKey = `${paramChatId}\0${text}\0${chain}`;
    if (landingInitialSendKeyRef.current === dedupeKey) return;
    landingInitialSendKeyRef.current = dedupeKey;

    navigate({ pathname: location.pathname }, { replace: true, state: null });
    setSelectedChainId(chain);
    queueMicrotask(() => {
      agent.submit(text, chain);
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [paramChatId, location.state]);

  // Drop the optimistic turn once the history echoes it (by provenance or content).
  useEffect(() => {
    if (!agent.optimistic) return;
    const persisted = chatHistory ?? [];
    const echoed = persisted.some(
      m =>
        m.role === 'user' &&
        (m.requestId === agent.optimistic?.requestId ||
          isOptimisticEcho(m.content, agent.optimistic?.content ?? '')),
    );
    if (echoed) agent.clearOptimistic();
  }, [chatHistory, agent]);

  const turns = useMemo(
    () => buildConsoleTurns(chatHistory ?? [], agent.optimistic, agent.activeRequestId),
    [chatHistory, agent.optimistic, agent.activeRequestId],
  );

  const pendingApproval = agent.activeRun?.pendingApproval ?? null;

  // Pin scroll to the bottom as turns stream in (and to the approval card).
  const endRef = useRef<HTMLDivElement | null>(null);
  const scrollSignature = `${turns.length}:${agent.activeRun?.events.length ?? 0}:${pendingApproval?.approvalId ?? ''}`;
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }, [scrollSignature]);

  const handleSubmit = useCallback(
    (e: React.FormEvent) => {
      e.preventDefault();
      if (!message.trim()) return;
      const collected = artifactRegistry.collectWithSources();
      const allArtifacts = collected.map(p => p.artifact);
      const explicitArtifacts = collected
        .filter(p => p.source.id.startsWith('mention:') || p.source.id.startsWith('slash:'))
        .map(p => p.artifact);
      agent.submit(
        message,
        selectedChainId,
        buildTurnContext(allArtifacts),
        artifactsToInlineAttachments(explicitArtifacts),
      );
      setMessage('');
    },
    [agent, artifactRegistry, message, selectedChainId],
  );

  const respondApproval = useCallback((approvalId: string, approved: boolean) => {
    api.respondToApproval(approvalId, approved).catch(() => {
      // Outcome re-arrives via the SSE stream either way.
    });
  }, []);

  // Terminal-genre keyboard: Esc stops the run; while an approval is pending
  // and the composer is unfocused, y approves and n denies.
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && agent.isProcessing) {
        e.preventDefault();
        agent.stop();
        return;
      }
      if (!pendingApproval) return;
      const target = e.target as HTMLElement | null;
      const typing =
        target instanceof HTMLTextAreaElement ||
        target instanceof HTMLInputElement ||
        target?.isContentEditable;
      if (typing) return;
      if (e.key === 'y' || e.key === 'Y') {
        e.preventDefault();
        respondApproval(pendingApproval.approvalId, true);
      } else if (e.key === 'n' || e.key === 'N') {
        e.preventDefault();
        respondApproval(pendingApproval.approvalId, false);
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [agent, pendingApproval, respondApproval]);

  return (
    <Page bodyScroll="hidden" className="h-full">
      <Fill className="flex min-h-0 flex-col">
        <StatusLine
          selectedChainId={selectedChainId}
          activePolicyName={activePolicyName}
          contextUsed={agent.contextUsed}
          contextSize={agent.contextSize}
          sseConnection={agent.sseConnection}
          isProcessing={agent.isProcessing}
          terminalAvailable={!terminalUnavailable}
          terminalOpen={terminalOpen}
          onToggleTerminal={() => setTerminalOpen(open => !open)}
        />

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-2">
          {historyLoading || isCreating ? (
            <div className="flex h-full items-center justify-center">
              <Spinner size="md" />
            </div>
          ) : turns.length === 0 ? (
            <div className="text-text-muted flex h-full items-center justify-center font-mono text-sm">
              {t('console.empty', 'Type a task below — the agent works here, visibly.')}
            </div>
          ) : (
            turns.map(turn => (
              <TurnBlock
                key={turn.key}
                turn={turn}
                run={turn.requestId ? (agent.runs[turn.requestId] ?? null) : null}
                onRespondApproval={respondApproval}
              />
            ))
          )}
          <div ref={endRef} />
        </div>

        {terminalOpen && (
          <div className="border-surface-300 dark:border-dark-surface-400 h-72 shrink-0 border-t">
            <Suspense
              fallback={
                <div className="flex h-full items-center justify-center">
                  <Spinner size="md" />
                </div>
              }>
              <TerminalPanel className="h-full min-h-0 min-w-0" />
            </Suspense>
          </div>
        )}

        {agent.operationError && (
          <InlineNotice
            variant="error"
            className="mx-4 mb-1"
            onDismiss={agent.clearOperationError}>
            <span className="break-words">{agent.operationError}</span>
            {agent.canRetry && (
              <Button variant="ghost" size="sm" className="ml-2" onClick={agent.retryLastFailed}>
                {t('plan.retry', 'Retry')}
              </Button>
            )}
          </InlineNotice>
        )}

        <div className="bg-surface-50 dark:bg-dark-surface-200 shrink-0 border-t border-surface-300 dark:border-dark-surface-400">
          <MessageInputForm
            value={message}
            onChange={setMessage}
            onSubmit={handleSubmit}
            placeholder={
              pendingApproval
                ? t(
                    'console.awaiting_approval',
                    'Waiting for approval — y approve · n deny · Esc stop',
                  )
                : t('console.placeholder', 'Task, question, or /command — Esc stops a run')
            }
            isPending={agent.isProcessing}
            canSubmit={!pendingApproval}
            variant="workbench"
          />
        </div>
      </Fill>
    </Page>
  );
}

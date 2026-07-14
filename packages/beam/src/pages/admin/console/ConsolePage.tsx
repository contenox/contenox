import { Fill, Page, Spinner } from '@contenox/ui';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate, useParams } from 'react-router-dom';
import { useAgentTurn } from '../../../hooks/useAgentTurn';
import { useChatHistory, useChats, useCreateChat } from '../../../hooks/useChats';
import { useListChains } from '../../../hooks/useChains';
import { useListPolicies, useSetActivePolicy } from '../../../hooks/usePolicies';
import { useSetupStatus } from '../../../hooks/useSetupStatus';
import { api } from '../../../lib/api';
import { ArtifactRegistryProvider, useArtifactRegistry } from '../../../lib/artifacts';
import { artifactsToInlineAttachments } from '../../../lib/inlineAttachments';
import { matchesOptimisticEcho } from '../../../lib/optimisticEcho';
import { createHelpCommand } from '../../../lib/slashCommands/builtins';
import {
  SlashCommandRegistryProvider,
  useSlashCommand,
  useSlashCommandRegistry,
} from '../../../lib/slashCommands/registry';
import type { ChatContextArtifact, ChatContextPayload } from '../../../lib/types';
import { useConsoleCommands } from './consoleCommands';
import { useConsoleShell, type ShellEntry } from './useConsoleShell';
import { buildConsoleTurns } from './consoleTurns';
import { ConsoleComposer } from './components/ConsoleComposer';
import { StatusLine } from './components/StatusLine';
import { TurnBlock } from './components/TurnBlock';
import { TERM } from './term';

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

/** Raw PTY scrollback from a `!command` — the PTY's own echo is the header. */
function ShellBlock({ entry }: { entry: ShellEntry }) {
  return (
    <pre className={`whitespace-pre-wrap break-words py-1 ${TERM.font} ${TERM.text}`}>
      {entry.text || <span className={TERM.dim}>…</span>}
    </pre>
  );
}

function ConsolePageImpl() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { chatId: paramChatId } = useParams<{ chatId: string }>();
  const chatId = paramChatId ?? '';

  const [message, setMessage] = useState('');
  const [selectedChainId, setSelectedChainId] = useState('');

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

  // Drop the optimistic turn once the history echoes it — by requestId when
  // the persisted row is stamped, else by windowed content match, so an older
  // persisted copy of a repeated command never clears the fresh turn.
  useEffect(() => {
    const optimistic = agent.optimistic;
    if (!optimistic) return;
    const persisted = chatHistory ?? [];
    const echoed = persisted.some(m => m.role === 'user' && matchesOptimisticEcho(m, optimistic));
    if (echoed) agent.clearOptimistic();
  }, [chatHistory, agent]);

  const turns = useMemo(
    () => buildConsoleTurns(chatHistory ?? [], agent.optimistic, agent.activeRequestId),
    [chatHistory, agent.optimistic, agent.activeRequestId],
  );

  const shell = useConsoleShell();

  // Shell entries interleave after the turn they were anchored to at exec
  // time; entries whose anchor vanished (optimistic echo) trail at the end.
  const shellByAnchor = useMemo(() => {
    const known = new Set(turns.map(turn => turn.key));
    const map = new Map<string, ShellEntry[]>();
    for (const entry of shell.entries) {
      const slot =
        entry.anchorKey === null ? '<head>' : known.has(entry.anchorKey) ? entry.anchorKey : '<tail>';
      const list = map.get(slot);
      if (list) list.push(entry);
      else map.set(slot, [entry]);
    }
    return map;
  }, [turns, shell.entries]);

  const pendingApproval = agent.activeRun?.pendingApproval ?? null;

  // Pin scroll to the bottom as turns stream in (and to the approval card).
  const endRef = useRef<HTMLDivElement | null>(null);
  const shellChars = shell.entries.reduce((n, e) => n + e.text.length, 0);
  const scrollSignature = `${turns.length}:${agent.activeRun?.events.length ?? 0}:${pendingApproval?.approvalId ?? ''}:${shell.entries.length}:${shellChars}`;
  useEffect(() => {
    endRef.current?.scrollIntoView({ behavior: 'smooth', block: 'end' });
  }, [scrollSignature]);

  const sendCurrent = useCallback(() => {
    if (!message.trim()) return;
    // Bang-escape: `!cmd` runs in the console's inline PTY, TUI-style.
    if (message.trimStart().startsWith('!')) {
      const command = message.trimStart().slice(1).trim();
      if (command) {
        shell.exec(command, turns.length > 0 ? turns[turns.length - 1].key : null);
      }
      setMessage('');
      return;
    }
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
  }, [agent, artifactRegistry, message, selectedChainId, shell, turns]);

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
      <Fill className={`flex min-h-0 flex-col ${TERM.surface} ${TERM.text}`}>
        <StatusLine
          selectedChainId={selectedChainId}
          activePolicyName={activePolicyName}
          model={setupStatus?.defaultModel}
          contextUsed={agent.contextUsed}
          contextSize={agent.contextSize}
          sseConnection={agent.sseConnection}
          isProcessing={agent.isProcessing}
        />

        <div className={`min-h-0 flex-1 overflow-y-auto px-3 py-2 ${TERM.font}`}>
          {historyLoading || isCreating ? (
            <div className="flex h-full items-center justify-center">
              <Spinner size="md" />
            </div>
          ) : turns.length === 0 && shell.entries.length === 0 ? (
            <div className={`flex h-full items-center justify-center ${TERM.dim}`}>
              {t('console.empty', 'Type a task below — the agent works here, visibly.')}
            </div>
          ) : (
            <div className={`divide-y divide-surface-200/60 dark:divide-dark-surface-300/50`}>
              {shellByAnchor.get('<head>')?.map(entry => (
                <ShellBlock key={`shell-${entry.id}`} entry={entry} />
              ))}
              {turns.map(turn => (
                <div key={turn.key}>
                  <TurnBlock
                    turn={turn}
                    run={turn.requestId ? (agent.runs[turn.requestId] ?? null) : null}
                    onRespondApproval={respondApproval}
                  />
                  {shellByAnchor.get(turn.key)?.map(entry => (
                    <ShellBlock key={`shell-${entry.id}`} entry={entry} />
                  ))}
                </div>
              ))}
              {shellByAnchor.get('<tail>')?.map(entry => (
                <ShellBlock key={`shell-${entry.id}`} entry={entry} />
              ))}
            </div>
          )}
          <div ref={endRef} />
        </div>

        {agent.operationError && (
          <div className={`flex items-center gap-2 border-t ${TERM.border} ${TERM.surface} px-3 py-1 ${TERM.small} ${TERM.err}`}>
            <span className="min-w-0 flex-1 break-words">{agent.operationError}</span>
            {agent.canRetry && (
              <button type="button" className={`${TERM.dim} hover:${TERM.text}`} onClick={agent.retryLastFailed}>
                [retry]
              </button>
            )}
            <button type="button" className={`${TERM.dim} hover:${TERM.text}`} onClick={agent.clearOperationError}>
              [dismiss]
            </button>
          </div>
        )}

        <ConsoleComposer
          value={message}
          onChange={setMessage}
          onSend={sendCurrent}
          disabled={!!pendingApproval}
          hint={
            pendingApproval
              ? t('console.awaiting_approval', 'awaiting approval — y approve · n deny · esc stop')
              : t('console.hint', 'type a task · ! shell · / commands · @ mentions · enter to run · esc stops')
          }
        />
      </Fill>
    </Page>
  );
}

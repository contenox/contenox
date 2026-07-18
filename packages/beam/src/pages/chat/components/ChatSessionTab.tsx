import {
  Button,
  ChatComposer,
  ChatScrollToLatest,
  ChatThread,
  EmptyState,
  InlineNotice,
  useChatScroll,
} from '@contenox/ui';
import { PanelLeft, PanelLeftClose } from 'lucide-react';
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
  type MouseEvent,
} from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useAcpWorkspace } from '../../../hooks/useAcpWorkspace';
import { useWorkspaceFiles } from '../../../hooks/useWorkspaceFiles';
import { initialAcpSessionState } from '../../../hooks/acpSessionState';
import { EMPTY_SESSION_KEY } from '../../../hooks/acpWorkspaceState';
import { externalAgentFromMeta, type SessionConfigOption, type SessionConfigOptionValue } from '../../../lib/acp';
import { useStagedAgent } from '../../../lib/stagedAgent';
import { useSetupStatus } from '../../../hooks/useSetupStatus';
import { usePersistentToggle } from '../../../hooks/usePersistentToggle';
import { classifyAcpExecutionError } from '../../../lib/acpFailureKind';
import { activeMentions, mentionPreviewPath, type WorkspaceFileRef } from '../lib/mentions';
import { resolveWorkspaceRoot } from '../lib/workspaceRoot';
import { useFilePreview } from '../../../hooks/useFilePreview';
import { parseTerminalPassthrough } from '../lib/terminalPassthrough';
import { ChatSessionToolbar } from './ChatSessionToolbar';
import { ExecutionErrorBanner, ResumedBanner } from './SessionBanners';
import { MentionMenu, useMentionMenu } from './MentionMenu';
import { PlanPanel } from './PlanPanel';
import { SlashCommandMenu, useSlashCommandMenu } from './SlashCommandMenu';
import { TranscriptItems } from './TranscriptItems';
import { WorkspacePanel } from './WorkspacePanel';
import { CanvasRegion } from './CanvasRegion';
import { useCanvasTabs } from '../../../hooks/useCanvasTabs';
import { fileCanvasTab, TERMINAL_CANVAS_TAB } from '../lib/canvasTabs';

// Shared workspace-wide UI preference (localStorage key). The file panel toggle
// is workspace-scoped, not per-session, so every open tab reads/writes one
// shared store (see usePersistentToggle) rather than drifting apart. The
// terminal is NOT a shared toggle: it is a per-session CANVAS tab (see
// useCanvasTabs / CanvasRegion).
const WORKSPACE_PANEL_TOGGLE_KEY = 'beam_workspace_panel_open';

// Mirrors acpsvc's workspace-root config option id. The chosen root becomes the
// session's cwd at creation time and is immutable afterward, so it is handled
// specially (fed to newSession, filtered out of the live-session controls).
const WORKSPACE_ROOT_CONFIG_ID = 'workspace-root';

// Mirrors acpsvc's HITL-policy config option id. Its currentValue is the
// session's chosen approval policy (a concrete name, or the "Default" sentinel);
// the workspace file tree's agent-view evaluates against it (see useWorkspaceFiles).
const HITL_POLICY_CONFIG_ID = 'hitl-policy';

function configOptionCurrentValue(options: SessionConfigOption[], id: string): string | undefined {
  return options.find(o => o.id === id)?.currentValue;
}

/**
 * Overlays locally-staged values onto the workspace-level config options so the
 * empty-chat controls reflect the user's pending picks before any session
 * exists. `currentValue` is a string on the wire (booleans as "true"/"false" —
 * see configOptionMapping), so a staged value is stringified onto it.
 */
function overlayStagedValues(
  options: SessionConfigOption[],
  staged: Record<string, SessionConfigOptionValue>,
): SessionConfigOption[] {
  if (options.length === 0 || Object.keys(staged).length === 0) return options;
  return options.map(opt => {
    const value = staged[opt.id];
    return value === undefined ? opt : { ...opt, currentValue: String(value) };
  });
}

export interface ChatSessionTabProps {
  /** The session this tab renders, or `null` for the empty/new-chat surface (lazy creation). */
  sessionId: string | null;
  /** Called with the freshly-minted session id when the empty surface lazily creates a session on first submit. */
  onSessionCreated: (sessionId: string) => void;
  /** Opens a fresh empty/new-chat surface — the narrow-viewport "new session" affordance. */
  onNewSession: () => void;
}

/**
 * ONE chat session rendered as a workspace tab (workspace-tabs Slice 2). This
 * is the per-session chat body extracted verbatim from the old single-view
 * `AcpChatPage`: the transcript, the composer with its @-mention menu + live
 * file preview, the per-session config controls (Model/HITL/Think/Token-Limit),
 * usage meter, plan panel, workspace/terminal panels, permission gate, and the
 * error/reconnect banners.
 *
 * It reads ITS OWN session slice from `sessions.slices[sessionId]` (not the
 * global focused accessor), so several `ChatSessionTab`s can be mounted at once
 * — one per open tab — each rendering its own live session. Only the active tab
 * is visible+interactive (its `sessionId` equals the controller's focused
 * session, kept in lockstep by `useWorkspaceTabs`), so the controller ops that
 * act on the focused session (`sendPrompt`, `setConfigOption`, `cancel`, …)
 * always target this tab when the user drives it.
 *
 * `sessionId === null` is the lazy-creation surface: no `session/new` until the
 * first submit, at which point it mints a session and reports it up via
 * `onSessionCreated` so the tab-model promotes the surface to a real tab.
 */
export function ChatSessionTab({ sessionId, onSessionCreated, onNewSession }: ChatSessionTabProps) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const {
    workspace,
    sessions,
    newSession,
    sendPrompt,
    runTerminal,
    respondPermission,
    cancel,
    setConfigOption,
    applyConfigOptions,
  } = useAcpWorkspace();

  const sessionKey = sessionId ?? EMPTY_SESSION_KEY;
  const session = sessions.slices[sessionKey] ?? initialAcpSessionState;

  const onEmptyChat = sessionId == null;

  // The external agent this chat talks to. On the empty chat it is the staged
  // pick (seeded by the sidebar's "new chat with an agent", changeable in the
  // toolbar); on a live session it is read back from the roster's `_meta` echo.
  // `null` means the native "contenox" chain.
  const { stagedAgent, setStagedAgent } = useStagedAgent();
  const sessionInfo = workspace.sessions.find(s => s.sessionId === sessionId);
  const sessionAgentName = externalAgentFromMeta(sessionInfo?._meta);
  const stagedExternalAgent = onEmptyChat ? stagedAgent : null;
  // Attribution for the transcript's assistant label: an external session shows
  // its agent's name instead of the generic workspace agent (init.agentInfo).
  const attributionAgentName = sessionAgentName ?? workspace.agentName;

  const { refetch: refetchSetupStatus } = useSetupStatus(true);

  const [draft, setDraft] = useState('');
  // Staged config choices for the empty chat (no session yet) — see the
  // equivalent block in the old AcpChatPage. This tab is remounted with a fresh
  // key by WorkspaceTabs once it spawns a session, so the staging clears itself.
  const [stagedConfig, setStagedConfig] = useState<Record<string, SessionConfigOptionValue>>({});
  const { containerRef, endRef, isNearBottom, scrollToEnd } = useChatScroll({
    deps: [session.items, session.pendingPermission],
  });

  // The session's workspace root: an active session's persisted cwd, or (on the
  // empty chat) the staged/default workspace-root pick. This is contenox
  // runtime-side data — the `/files` REST is rooted here and the `@`-mention list
  // comes from it — so it is INDEPENDENT of which agent drives the session: an
  // external-agent session resolves its root exactly like a native one (its
  // `@`-mentions ride as reference-only `resource_link` blocks the downstream
  // agent reads under the same cwd). See lib/workspaceRoot.ts for the full rules;
  // notably an external session exposes no root PICKER, so it falls back to the
  // default root — the cwd the runtime creates it under.
  const stagedRoot = stagedConfig[WORKSPACE_ROOT_CONFIG_ID];
  const activeSessionCwd = sessionInfo?.cwd ?? null;
  const defaultRoot = configOptionCurrentValue(workspace.workspaceConfigOptions, WORKSPACE_ROOT_CONFIG_ID);
  const workspaceRoot = resolveWorkspaceRoot({
    onEmptyChat,
    stagedRoot: typeof stagedRoot === 'string' ? stagedRoot : undefined,
    defaultRoot,
    activeSessionCwd,
  });

  // The session's active HITL policy drives the workspace tree's agent-view: the
  // file explorer evaluates each path against the SAME policy the live agent gates
  // under, so switching strict/dev/etc. re-colors the verdicts. A live session
  // reads its own `session.configOptions`; the empty chat reads the workspace
  // options overlaid with staged picks (mirrors `headerConfigOptions` below).
  const currentHitlPolicy = onEmptyChat
    ? configOptionCurrentValue(overlayStagedValues(workspace.workspaceConfigOptions, stagedConfig), HITL_POLICY_CONFIG_ID)
    : configOptionCurrentValue(session.configOptions, HITL_POLICY_CONFIG_ID);

  const files = useWorkspaceFiles(workspaceRoot, currentHitlPolicy);

  const { open: panelOpen, toggle: togglePanel } = usePersistentToggle(WORKSPACE_PANEL_TOGGLE_KEY);

  // The canvas tab-model (per-session): the terminal now lives here as a
  // side-by-side canvas tab rather than a cramped right sidebar.
  const canvas = useCanvasTabs();
  const { open: openCanvasTab } = canvas;
  const openTerminal = useCallback(() => openCanvasTab(TERMINAL_CANVAS_TAB), [openCanvasTab]);
  // Clicking a file in the workspace sidebar opens it as a read-only canvas tab
  // (dedup by path), rather than previewing inline in the sidebar.
  const openFile = useCallback((path: string) => openCanvasTab(fileCanvasTab(path)), [openCanvasTab]);
  const activeFileTab = canvas.tabs.find(tab => tab.id === canvas.activeId && tab.kind === 'file');
  const selectedFilePath = activeFileTab?.path ?? null;

  // Surface the terminal canvas tab automatically the first time THIS session
  // produces shell output. The canvas is per-session, so the `!`-passthrough on
  // the empty/new-chat surface (which issues the command on one ChatSessionTab
  // and then promotes the freshly-created session to a *different*, now-active
  // tab) would otherwise leave the visible tab's canvas empty. Also covers the
  // agent running a shell mid-turn. Guarded by a ref so a user who closes the
  // terminal keeps it closed.
  const terminalHasOutput = (session.terminal?.text?.length ?? 0) > 0;
  const autoSurfacedTerminalRef = useRef(false);
  useEffect(() => {
    if (terminalHasOutput && !autoSurfacedTerminalRef.current) {
      autoSurfacedTerminalRef.current = true;
      openCanvasTab(TERMINAL_CANVAS_TAB);
    }
  }, [terminalHasOutput, openCanvasTab]);

  // @-mention state.
  const [mentions, setMentions] = useState<WorkspaceFileRef[]>([]);
  const [caret, setCaret] = useState(0);
  const mentionMenu = useMentionMenu({
    draft,
    caret,
    cache: files.cache,
    ensureLoaded: files.ensureLoaded,
    isLoading: files.isLoading,
    onInsert: (nextDraft, nextCaret, file) => {
      setDraft(nextDraft);
      setCaret(nextCaret);
      setMentions(prev => (prev.some(m => m.path === file.path) ? prev : [...prev, file]));
    },
    onDrill: (nextDraft, nextCaret) => {
      setDraft(nextDraft);
      setCaret(nextCaret);
    },
  });
  const trackCaret = useCallback((e: KeyboardEvent<HTMLTextAreaElement> | MouseEvent<HTMLTextAreaElement>) => {
    setCaret(e.currentTarget.selectionStart ?? 0);
  }, []);

  // Live file preview while the @-menu is open and a FILE is highlighted.
  const previewPath = mentionMenu.open ? mentionPreviewPath(mentionMenu.entries, mentionMenu.activeIndex) : null;
  const filePreview = useFilePreview(previewPath, files.readFile);

  // "modeld down" double-error-surface dedup — see the old AcpChatPage comment.
  const lastRefetchedErrorRef = useRef<string | null>(null);
  useEffect(() => {
    if (!session.error) {
      lastRefetchedErrorRef.current = null;
      return;
    }
    if (lastRefetchedErrorRef.current === session.error) return;
    const kind = classifyAcpExecutionError(session.error);
    if (kind === 'generic') return;
    lastRefetchedErrorRef.current = session.error;
    void refetchSetupStatus();
  }, [session.error, refetchSetupStatus]);

  const handleOpenSettings = useCallback(() => navigate('/settings'), [navigate]);

  const slashMenu = useSlashCommandMenu({ draft, onDraftChange: setDraft, availableCommands: session.availableCommands });

  const composerKeyDown = useCallback(
    (e: KeyboardEvent<HTMLTextAreaElement>) => {
      slashMenu.handleKeyDown(e);
      mentionMenu.handleKeyDown(e);
    },
    [slashMenu, mentionMenu],
  );

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      if (slashMenu.open || mentionMenu.open) return;

      if (session.isPrompting) {
        // D6: Enter must NOT submit/cancel while prompting — only an explicit
        // click on the (now "Stop"-labelled) submit button does.
        const submitter = (e.nativeEvent as SubmitEvent).submitter ?? null;
        if (submitter) cancel();
        return;
      }

      const text = draft.trim();
      if (!text) return;

      // `!` passthrough: run the rest as a shell line (no LLM turn).
      const shellCommand = parseTerminalPassthrough(draft);
      if (shellCommand) {
        setDraft('');
        setMentions([]);
        openTerminal();
        let shellSid = sessionId;
        if (!shellSid) {
          const rootPick = typeof stagedRoot === 'string' && stagedRoot.trim() !== '' ? stagedRoot : undefined;
          try {
            shellSid = await newSession(rootPick, stagedExternalAgent);
          } catch {
            setDraft(draft);
            return;
          }
          if (stagedExternalAgent) setStagedAgent(null);
          onSessionCreated(shellSid);
        }
        void runTerminal(shellCommand);
        return;
      }

      const turnMentions = activeMentions(draft, mentions);
      setDraft('');
      setMentions([]);

      let sid = sessionId;
      if (!sid) {
        // Lazy creation (D5): no session/new until the first submit. A staged
        // external agent binds the session via `_meta` (see AGENT_META_KEY).
        const rootPick = typeof stagedRoot === 'string' && stagedRoot.trim() !== '' ? stagedRoot : undefined;
        try {
          sid = await newSession(rootPick, stagedExternalAgent);
        } catch {
          // newSession() already surfaced the failure; restore the draft.
          setDraft(text);
          setMentions(turnMentions);
          return;
        }
        // Flush the empty-chat's staged native config choices onto the just-minted
        // session BEFORE the prompt runs (so they win over server defaults). An
        // external session has NO config options (set_config_option would fail),
        // so this is native-only.
        if (!stagedExternalAgent) {
          const staged = Object.entries(stagedConfig)
            .filter(([configId]) => configId !== WORKSPACE_ROOT_CONFIG_ID)
            .map(([configId, value]) => ({ configId, value }));
          if (staged.length > 0) {
            try {
              await applyConfigOptions(staged);
            } catch {
              setDraft(text);
              setMentions(turnMentions);
              return;
            }
          }
        }
        // The staged agent is one-shot — consumed by this session, so the next
        // fresh empty chat starts native again.
        if (stagedExternalAgent) setStagedAgent(null);
        // Promote the empty surface to a real tab (URL follows the active tab).
        onSessionCreated(sid);
      }
      sendPrompt(text, turnMentions);
    },
    [
      slashMenu.open,
      mentionMenu.open,
      session.isPrompting,
      draft,
      mentions,
      stagedRoot,
      stagedExternalAgent,
      setStagedAgent,
      sessionId,
      stagedConfig,
      newSession,
      applyConfigOptions,
      onSessionCreated,
      sendPrompt,
      runTerminal,
      openTerminal,
      cancel,
    ],
  );

  // Config-control change handler. On an empty chat (no session) it stages the
  // pick locally; once a session exists it pushes straight through
  // set_config_option (which targets the focused session — this active tab).
  const handleConfigChange = useCallback(
    (configId: string, value: SessionConfigOptionValue) => {
      if (sessionId == null) {
        setStagedConfig(prev => ({ ...prev, [configId]: value }));
      } else {
        setConfigOption(configId, value);
      }
    },
    [sessionId, setConfigOption],
  );

  const hasContent = session.items.length > 0;
  const composerDisabled = session.pendingPermission != null || (!session.isPrompting && workspace.status !== 'ready');

  // Config controls source: a live session's own `session.configOptions`, or (on
  // the empty chat) the workspace-level options overlaid with staged picks. The
  // workspace-root option is a pre-session picker only. A staged external agent
  // exposes NO native config options (model/think/policy don't apply — the live
  // external session likewise returns none), so they are hidden pre-session too.
  const headerConfigOptions = onEmptyChat
    ? stagedExternalAgent
      ? []
      : overlayStagedValues(workspace.workspaceConfigOptions, stagedConfig)
    : session.configOptions.filter(o => o.id !== WORKSPACE_ROOT_CONFIG_ID);

  return (
    <div className="bg-surface dark:bg-dark-surface flex h-full min-h-0 flex-col">
      <ChatSessionToolbar
        usage={session.usage}
        configOptions={headerConfigOptions}
        onConfigChange={handleConfigChange}
        showNewSession={!onEmptyChat}
        onNewSession={onNewSession}
      />

      {workspace.status === 'reconnecting' && <InlineNotice variant="warning">{t('acp_chat.banner_reconnecting')}</InlineNotice>}
      {session.connectionBanner === 'disconnected' && (
        <InlineNotice variant="warning">{t('acp_chat.banner_disconnected')}</InlineNotice>
      )}
      {session.connectionBanner === 'resumed' && <ResumedBanner />}
      {session.error && <ExecutionErrorBanner message={session.error} onOpenSettings={handleOpenSettings} />}

      <PlanPanel entries={session.plan} />

      <div className="flex min-h-0 flex-1">
        {workspaceRoot && (
          // Slim left rail mirroring the terminal's right rail in CanvasRegion:
          // a stable, edge-anchored toggle on the same side the file panel opens,
          // instead of a switch buried in the top config strip. Desktop-only
          // because the file panel itself is (`hidden sm:flex`) — the terminal,
          // which works on narrow viewports via full-width takeover, has no such
          // gate, so this asymmetry follows the panels' own responsiveness.
          <div className="border-surface-200 dark:border-dark-surface-600 hidden shrink-0 flex-col items-center gap-1 border-r px-1 py-2 sm:flex">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-pressed={panelOpen}
              aria-label={t('workspace.toggle_label')}
              title={t('workspace.show_files')}
              onClick={togglePanel}>
              {panelOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeft className="h-4 w-4" />}
            </Button>
          </div>
        )}
        {panelOpen && workspaceRoot && (
          <div className="hidden sm:flex">
            <WorkspacePanel
              root={workspaceRoot}
              files={files}
              onOpenFile={openFile}
              selectedFilePath={selectedFilePath}
            />
          </div>
        )}
        <CanvasRegion
          sessionId={sessionId}
          canvas={canvas}
          readFile={files.readFile}
          className="flex-1">
          <div className="flex min-h-0 min-w-0 flex-1 flex-col">
          {!hasContent && !session.pendingPermission ? (
            <div className="m-auto">
              <EmptyState
                title={t('acp_chat.empty_title')}
                description={
                  stagedExternalAgent
                    ? t('acp_chat.empty_description_agent', { name: stagedExternalAgent })
                    : t('acp_chat.empty_description')
                }
              />
            </div>
          ) : (
            <div className="relative flex min-h-0 flex-1 flex-col">
              <ChatThread containerRef={containerRef} endRef={endRef}>
                <TranscriptItems
                  session={session}
                  agentName={attributionAgentName}
                  onRespondPermission={respondPermission}
                />
              </ChatThread>
              <ChatScrollToLatest
                visible={!isNearBottom}
                onClick={scrollToEnd}
                label={t('acp_chat.scroll_to_latest')}
              />
            </div>
          )}

          <div className="relative flex min-h-0 flex-col px-3 pb-3 sm:px-4">
            {slashMenu.open ? (
              <SlashCommandMenu
                entries={slashMenu.entries}
                activeIndex={slashMenu.activeIndex}
                onPick={slashMenu.pick}
                onHoverIndex={slashMenu.setActiveIndex}
              />
            ) : mentionMenu.open ? (
              <MentionMenu
                entries={mentionMenu.entries}
                scope={mentionMenu.scope}
                loading={mentionMenu.loading}
                activeIndex={mentionMenu.activeIndex}
                onPick={mentionMenu.pick}
                onHoverIndex={mentionMenu.setActiveIndex}
                preview={filePreview}
              />
            ) : null}
            <div className="shrink-0">
              <ChatComposer
                value={draft}
                onChange={setDraft}
                onSubmit={handleSubmit}
                isPending={false}
                disabled={composerDisabled}
                canSubmit={workspace.status === 'ready' || session.isPrompting}
                allowEmptyMessage={session.isPrompting}
                submitLabel={session.isPrompting ? t('acp_chat.composer_stop') : t('acp_chat.composer_send')}
                placeholder={workspace.status === 'ready' ? t('acp_chat.composer_placeholder') : t('acp_chat.composer_placeholder_connecting')}
                textareaProps={{ onKeyDown: composerKeyDown, onKeyUp: trackCaret, onClick: trackCaret, onSelect: trackCaret }}
              />
            </div>
          </div>
          </div>
        </CanvasRegion>
      </div>
    </div>
  );
}

import {
  Badge,
  Button,
  ChatComposer,
  ChatScrollToLatest,
  ChatThread,
  ChatThreadSkeleton,
  Collapsible,
  EmptyState,
  H2,
  InlineNotice,
  Span,
  useChatScroll,
} from '@contenox/ui';
import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { useAcpWorkspace } from '../../hooks/useAcpWorkspace';
import type { AcpWorkspaceStatus } from '../../hooks/acpWorkspaceState';
import type { SessionConfigOption, SessionConfigOptionValue } from '../../lib/acp';
import { useSetupStatus } from '../../hooks/useSetupStatus';
import { classifyAcpExecutionError, classifySetupIssueCode } from '../../lib/acpFailureKind';
import { getBlockingSetupIssue, getSetupIssueFixPath } from '../../lib/setupHealth';
import type { SetupIssue } from '../../lib/types';
import { SlashCommandRegistryProvider } from '../../lib/slashCommands';
import { ConfigOptionControls } from './components/ConfigOptionControls';
import { PermissionGate } from './components/PermissionGate';
import { PlanPanel } from './components/PlanPanel';
import { SlashCommandMenu, useSlashCommandMenu } from './components/SlashCommandMenu';
import { TranscriptItems } from './components/TranscriptItems';
import { UsageMeter } from './components/UsageMeter';

/**
 * THE chat surface, mounted at `/chat` and `/chat/:sessionId` (Stage 4 — see
 * `App.tsx`, which hoists `AcpWorkspaceProvider` into the authenticated app
 * shell so this page, the sessions rail, and any future ACP-backed surface
 * all share one connection instead of each owning/remounting their own).
 *
 * Its only job is to render whatever the connected ACP agent exposes — the
 * way Zed renders any ACP agent — using beam's standard chat styling
 * (ChatThread/ChatMessage/ChatComposer, not the console's terminal look).
 */
export default function AcpChatPage() {
  return (
    <SlashCommandRegistryProvider>
      <AcpChatRoute />
    </SlashCommandRegistryProvider>
  );
}

function AcpChatRoute() {
  const { sessionId } = useParams<{ sessionId?: string }>();
  const paramSessionId = sessionId ?? null;
  // Keyed by the route param: switching sessions (or bare -> deep link) gets
  // a fresh component instance, which resets all local composer/menu/attempt
  // state for free instead of needing to reconcile it across navigations.
  return <AcpChatWorkspace key={paramSessionId ?? '__new__'} paramSessionId={paramSessionId} />;
}

function statusBadgeVariant(status: AcpWorkspaceStatus): 'success' | 'warning' | 'error' | 'secondary' {
  if (status === 'ready') return 'success';
  if (status === 'reconnecting') return 'warning';
  if (status === 'error' || status === 'disconnected' || status === 'setup_required') return 'error';
  return 'secondary';
}

function ResumedBanner() {
  const { t } = useTranslation();
  const [visible, setVisible] = useState(true);
  useEffect(() => {
    const id = setTimeout(() => setVisible(false), 4000);
    return () => clearTimeout(id);
  }, []);
  if (!visible) return null;
  return <InlineNotice variant="info">{t('acp_chat.banner_resumed')}</InlineNotice>;
}

/**
 * The chain-failure banner (BUG: previously a raw wall of `session.error`
 * text, then a single generic headline for every failure). Now classifies
 * `message` via `classifyAcpExecutionError` (see lib/acpFailureKind.ts) so a
 * runtime-backend-unreachable failure, a not-servable-default-model failure,
 * and an unrelated chain failure each get their own headline/description
 * instead of one indistinguishable "Execution failed" — this is the same
 * three-way taxonomy `SetupRequiredState` below uses for a `/setup-status`
 * blocking issue, so the two detection paths read as ONE consistent state
 * for the same root cause rather than two different-looking error surfaces.
 * The full raw error text stays behind a collapsed-by-default disclosure so
 * the transcript isn't dominated by a stack of provider/runtime prose.
 */
function ExecutionErrorBanner({ message, onOpenSettings }: { message: string; onOpenSettings: () => void }) {
  const { t } = useTranslation();
  const kind = classifyAcpExecutionError(message);

  const headline =
    kind === 'backend_unreachable'
      ? t('acp_recovery.backend_unreachable_title')
      : kind === 'model_unavailable'
        ? t('acp_recovery.model_unavailable_title')
        : t('acp_chat.error_banner_headline');
  const hint =
    kind === 'backend_unreachable'
      ? t('acp_recovery.backend_unreachable_description')
      : kind === 'model_unavailable'
        ? t('acp_recovery.model_unavailable_description')
        : null;

  return (
    <InlineNotice variant="error">
      <div className="flex flex-col gap-1.5">
        <Span className="font-medium">{headline}</Span>
        {hint && <Span className="text-sm">{hint}</Span>}
        {kind === 'model_unavailable' && (
          <div>
            <Button type="button" variant="secondary" size="sm" onClick={onOpenSettings}>
              {t('acp_recovery.model_unavailable_action')}
            </Button>
          </div>
        )}
        <Collapsible defaultOpen={false} title={t('acp_chat.error_details_toggle')}>
          <p className="mt-1 text-xs whitespace-pre-wrap">{message}</p>
        </Collapsible>
      </div>
    </InlineNotice>
  );
}

/**
 * Full-page "setup required" state, shown when `/setup-status` reports a
 * blocking issue OR the workspace connection itself needs auth. `issue.code`
 * is run through the SAME `classifySetupIssueCode` taxonomy
 * `ExecutionErrorBanner` uses for a live `session.error` — a modeld/backend
 * outage and a broken-default-model misconfiguration each get their own
 * title/copy instead of one generic "ACP setup needs attention", and the fix
 * path (see `getSetupIssueFixPath` / `setupcheck.go`'s `FixPath`) already
 * routes a broken default model to Settings, not Backends.
 */
function SetupRequiredState({
  issue,
  fallbackMessage,
  onRetryBackend,
}: {
  issue: SetupIssue | null;
  fallbackMessage: string | null;
  onRetryBackend: () => void;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const fixPath = issue ? getSetupIssueFixPath(issue) : '/settings';
  const kind = classifySetupIssueCode(issue?.code);

  const title =
    kind === 'backend_unreachable'
      ? t('acp_recovery.backend_unreachable_title')
      : kind === 'model_unavailable'
        ? t('acp_recovery.model_unavailable_title')
        : t('acp_chat.setup_required_title');
  const actionLabel =
    kind === 'model_unavailable' ? t('acp_recovery.model_unavailable_action') : t('acp_chat.setup_required_action');

  return (
    <div className="m-auto max-w-md p-6">
      <EmptyState
        variant="warning"
        title={title}
        description={issue?.message ?? fallbackMessage ?? t('acp_chat.setup_required_generic')}
      />
      <div className="mt-4 flex justify-center gap-2">
        {kind === 'backend_unreachable' && (
          <Button type="button" variant="secondary" onClick={onRetryBackend}>
            {t('acp_recovery.backend_unreachable_retry')}
          </Button>
        )}
        <Button type="button" variant="primary" onClick={() => navigate(fixPath)}>
          {actionLabel}
        </Button>
      </div>
    </div>
  );
}

function DisconnectedState({ onRetry }: { onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="m-auto max-w-md p-6">
      <EmptyState variant="error" title={t('acp_chat.disconnected_title')} description={t('acp_chat.disconnected_description')} />
      <div className="mt-4 flex justify-center">
        <Button type="button" variant="primary" onClick={onRetry}>
          {t('acp_chat.disconnected_retry')}
        </Button>
      </div>
    </div>
  );
}

function ErrorState({ message, onRetry }: { message: string | null; onRetry: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="m-auto max-w-md p-6">
      <EmptyState variant="error" title={t('acp_chat.error_title')} description={message ?? t('acp_chat.error_generic')} />
      <div className="mt-4 flex justify-center">
        <Button type="button" variant="primary" onClick={onRetry}>
          {t('acp_chat.disconnected_retry')}
        </Button>
      </div>
    </div>
  );
}

function NotFoundState({ onNewSession }: { onNewSession: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="m-auto max-w-md p-6">
      <EmptyState title={t('acp_chat.not_found_title')} description={t('acp_chat.not_found_description')} />
      <div className="mt-4 flex justify-center">
        <Button type="button" variant="primary" onClick={onNewSession}>
          {t('acp_chat.not_found_action')}
        </Button>
      </div>
    </div>
  );
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

function AcpChatWorkspace({ paramSessionId }: { paramSessionId: string | null }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const {
    workspace,
    session,
    newSession,
    openSession,
    clearActiveSession,
    sendPrompt,
    respondPermission,
    cancel,
    setConfigOption,
    applyConfigOptions,
    reconnect,
  } = useAcpWorkspace();
  const { data: setupStatus, refetch: refetchSetupStatus } = useSetupStatus(true);
  const blockingSetupIssue = getBlockingSetupIssue(setupStatus);

  const [draft, setDraft] = useState('');
  // Staged config choices for the empty chat (no session yet). Sessions are
  // minted lazily on first submit, so there's no session to push a
  // set_config_option to yet — the user's picks (model/think/HITL/token-limit)
  // are held here and flushed to the freshly-minted session in handleSubmit,
  // BEFORE the first prompt runs, so they win over the server's per-session
  // defaults (critical when the configured default model is broken). This
  // component is keyed by the route param (see AcpChatRoute), so navigating to
  // a real session remounts and clears the staging for free.
  const [stagedConfig, setStagedConfig] = useState<Record<string, SessionConfigOptionValue>>({});
  const { containerRef, endRef, isNearBottom, scrollToEnd } = useChatScroll({
    deps: [session.items, session.pendingPermission],
  });

  // Deduplication for the "modeld down" double-error-surface bug: a failed
  // `session/prompt` classifies as backend_unreachable/model_unavailable
  // (see lib/acpFailureKind.ts) far sooner than the next `/setup-status`
  // poll (staleTime 30s) would otherwise notice — without this, the user
  // sees the inline ExecutionErrorBanner sit for up to 30s before the page
  // *separately* flips to SetupRequiredState with what reads as a second,
  // differently-worded error. Forcing an immediate refetch here converges
  // both detection paths onto the SAME state almost immediately instead of
  // leaving two successive surfaces on screen. Keyed by message identity so
  // it fires once per distinct failure, not on every re-render.
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

  const handleRetryBackend = useCallback(() => {
    void refetchSetupStatus();
    void reconnect();
  }, [refetchSetupStatus, reconnect]);

  const handleOpenSettings = useCallback(() => navigate('/settings'), [navigate]);

  // Deep-link open: attempt exactly once per (mount, target id) pair. Gated
  // on 'ready' OR 'error' (not 'connecting'/'disconnected'/'setup_required')
  // so a stale error from something unrelated doesn't block a legitimate
  // open, but we don't race the initial connect either.
  const openAttemptRef = useRef<string | null>(null);
  useEffect(() => {
    if (!paramSessionId) return;
    if (openAttemptRef.current === paramSessionId) return;
    if (workspace.activeSessionId === paramSessionId) {
      // Already open — the lazy-create flow (handleSubmit: newSession() then
      // navigate('/chat/<sid>')) lands here on this mount's first effect run.
      // Mark the attempt anyway: this effect re-runs whenever
      // workspace.activeSessionId changes, and react-router v7 wraps
      // navigation in startTransition, so a "new session" click's
      // clearActiveSession() commits (activeSessionId -> null) BEFORE this
      // instance unmounts. Without the mark, that re-run would fall through
      // to openSession(paramSessionId) below and re-open — re-subscribe,
      // re-activate, replay — the very session the user just cleared.
      openAttemptRef.current = paramSessionId;
      return;
    }
    if (workspace.status !== 'ready' && workspace.status !== 'error') return;
    openAttemptRef.current = paramSessionId;
    openSession(paramSessionId);
  }, [paramSessionId, workspace.status, workspace.activeSessionId, openSession]);

  // `workspace.sessionLoadState` is the controller's explicit, authoritative
  // outcome of the most recent `openSession()` call (see
  // acpWorkspaceController.ts / acpWorkspaceState.ts) — no more inferring
  // "not found" from a combination of connection status + empty transcript.
  // The `openAttemptRef.current === paramSessionId` guard matters because
  // `sessionLoadState` is workspace-wide, not scoped to this mount: without
  // it, navigating from a session that just failed to open straight to a
  // *different* deep link would render NotFoundState for one stale tick
  // before this page's own openSession() call for the new id has even fired.
  const notFound =
    paramSessionId != null && openAttemptRef.current === paramSessionId && workspace.sessionLoadState === 'not_found';

  const slashMenu = useSlashCommandMenu({ draft, onDraftChange: setDraft, availableCommands: session.availableCommands });

  // Canonical "new session" affordance (BUG 1 / BUG 2): clears the
  // workspace-global activeSessionId at CLICK time (not via an effect keyed
  // on it — that would race handleSubmit's lazy newSession(), which sets
  // activeSessionId while still on bare /chat, right before navigating —
  // see acpWorkspaceController.ts's clearActiveSession() doc comment) and
  // only then navigates to bare /chat, so the remounted page starts with an
  // empty composer and no stale transcript.
  const handleNewSession = useCallback(() => {
    clearActiveSession();
    navigate('/chat');
  }, [clearActiveSession, navigate]);

  const handleSubmit = useCallback(
    async (e: FormEvent) => {
      e.preventDefault();
      // The slash menu intercepts Enter to accept a completion while it's
      // open (see SlashCommandMenu.tsx) — a submit event firing while it's
      // still open means the browser raced us; treat it as consumed.
      if (slashMenu.open) return;

      if (session.isPrompting) {
        // D6: Enter must NOT submit/cancel while prompting — only an
        // explicit click on the (now "Stop"-labelled) submit button does.
        // `requestSubmit()` called from Enter has no `submitter`; a real
        // click always does.
        const submitter = (e.nativeEvent as SubmitEvent).submitter ?? null;
        if (submitter) cancel();
        return;
      }

      const text = draft.trim();
      if (!text) return;
      setDraft('');

      let sid = workspace.activeSessionId;
      if (!sid) {
        // Lazy creation (D5): no session/new until the first submit.
        try {
          sid = await newSession();
        } catch {
          // newSession() itself already surfaced the failure (setup_required
          // for auth errors, session.error otherwise — see
          // acpWorkspaceController.ts's newSession()). Restore the draft so
          // the user doesn't lose what they typed and can just retry.
          setDraft(text);
          return;
        }
        // Flush the empty-chat's staged config choices onto the just-minted
        // session BEFORE the prompt runs, so they win over the server's
        // per-session defaults for this first turn — the turn that fails when
        // the configured default model is broken and the user picked a working
        // one on the empty chat.
        const staged = Object.entries(stagedConfig).map(([configId, value]) => ({ configId, value }));
        if (staged.length > 0) {
          try {
            await applyConfigOptions(staged);
          } catch {
            // applyConfigOptions already surfaced the failure on the session
            // error banner. Hold the turn back and restore the draft so the
            // user can adjust their pick and retry rather than silently running
            // against the broken default.
            setDraft(text);
            return;
          }
        }
        navigate(`/chat/${sid}`, { replace: true });
      }
      sendPrompt(text);
    },
    [
      slashMenu.open,
      session.isPrompting,
      draft,
      workspace.activeSessionId,
      stagedConfig,
      newSession,
      applyConfigOptions,
      navigate,
      sendPrompt,
      cancel,
    ],
  );

  // Config-control change handler. Declared before the early returns below so
  // the hook order stays stable across renders (Rules of Hooks). On an empty
  // chat (no session) it stages the pick locally; once a session exists it
  // pushes straight through set_config_option — see the render site.
  const handleConfigChange = useCallback(
    (configId: string, value: SessionConfigOptionValue) => {
      if (workspace.activeSessionId == null) {
        setStagedConfig(prev => ({ ...prev, [configId]: value }));
      } else {
        setConfigOption(configId, value);
      }
    },
    [workspace.activeSessionId, setConfigOption],
  );

  if (blockingSetupIssue || workspace.status === 'setup_required') {
    return (
      <SetupRequiredState
        issue={blockingSetupIssue}
        fallbackMessage={workspace.error}
        onRetryBackend={handleRetryBackend}
      />
    );
  }
  if (notFound) {
    return <NotFoundState onNewSession={handleNewSession} />;
  }
  if (workspace.status === 'disconnected') {
    return <DisconnectedState onRetry={reconnect} />;
  }
  if (workspace.status === 'error') {
    return <ErrorState message={workspace.error} onRetry={reconnect} />;
  }
  if (workspace.status === 'connecting' && session.items.length === 0) {
    return (
      <div
        className="flex h-full min-h-0 flex-1 flex-col"
        role="status"
        aria-label={t('acp_chat.connecting_label')}>
        <ChatThreadSkeleton className="flex-1" />
      </div>
    );
  }

  const hasContent = session.items.length > 0;
  const composerDisabled = session.pendingPermission != null || (!session.isPrompting && workspace.status !== 'ready');

  // Config controls source: once a session exists, its live `session.configOptions`
  // drive them and changes push straight through `setConfigOption`. On an empty
  // chat (no session yet) they come from the workspace-level options advertised
  // at initialize (see acpWorkspaceState), and changes are staged locally to be
  // flushed on the first submit — see `stagedConfig` / handleSubmit.
  const onEmptyChat = workspace.activeSessionId == null;
  const headerConfigOptions = onEmptyChat
    ? overlayStagedValues(workspace.workspaceConfigOptions, stagedConfig)
    : session.configOptions;

  return (
    <div className="bg-surface dark:bg-dark-surface flex h-full min-h-0 flex-col">
      <header className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-wrap items-center justify-between gap-3 border-b px-3 py-3 sm:px-4">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <H2>{t('acp_chat.title')}</H2>
          <Badge variant={statusBadgeVariant(workspace.status)} size="sm">
            {t(`acp_chat.status_${workspace.status}`)}
          </Badge>
          {workspace.agentName && (
            <Span variant="muted" className="text-sm">
              {workspace.agentName}
            </Span>
          )}
        </div>
        <div className="flex flex-wrap items-center gap-3">
          <UsageMeter usage={session.usage} />
          <ConfigOptionControls configOptions={headerConfigOptions} onChange={handleConfigChange} />
          {paramSessionId && (
            // The sidebar's "new session" button (AcpSessionSidebar.tsx) is
            // canonical and always present at sm+ viewports (Layout.tsx's
            // DesktopSidebar) — this one is only needed on narrow viewports,
            // where the sidebar is a closeable drawer (BUG 2: exactly one
            // visible affordance per viewport state).
            <Button
              type="button"
              variant="outline"
              palette="neutral"
              size="sm"
              className="sm:hidden"
              onClick={handleNewSession}>
              {t('acp_chat.new_session')}
            </Button>
          )}
        </div>
      </header>

      {workspace.status === 'reconnecting' && <InlineNotice variant="warning">{t('acp_chat.banner_reconnecting')}</InlineNotice>}
      {session.connectionBanner === 'disconnected' && (
        <InlineNotice variant="warning">{t('acp_chat.banner_disconnected')}</InlineNotice>
      )}
      {session.connectionBanner === 'resumed' && <ResumedBanner />}
      {session.error && <ExecutionErrorBanner message={session.error} onOpenSettings={handleOpenSettings} />}

      <PlanPanel entries={session.plan} />

      {!hasContent ? (
        <div className="m-auto">
          <EmptyState title={t('acp_chat.empty_title')} description={t('acp_chat.empty_description')} />
        </div>
      ) : (
        <div className="relative flex min-h-0 flex-1 flex-col">
          {/* Wrapper must be a flex column: ChatThread sizes itself with
              flex-1/min-h-0, which is inert inside a plain block wrapper — the
              scroll region then grows unbounded and the thread stops scrolling. */}
          <ChatThread containerRef={containerRef} endRef={endRef}>
            <TranscriptItems session={session} agentName={workspace.agentName} />
          </ChatThread>
          <ChatScrollToLatest
            visible={!isNearBottom}
            onClick={scrollToEnd}
            label={t('acp_chat.scroll_to_latest')}
          />
        </div>
      )}

      <div className="relative shrink-0 px-3 pb-3 sm:px-4">
        {slashMenu.open && (
          <SlashCommandMenu
            entries={slashMenu.entries}
            activeIndex={slashMenu.activeIndex}
            onPick={slashMenu.pick}
            onHoverIndex={slashMenu.setActiveIndex}
          />
        )}
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
          textareaProps={{ onKeyDown: slashMenu.handleKeyDown }}
        />
      </div>

      <PermissionGate permission={session.pendingPermission} onRespond={respondPermission} />
    </div>
  );
}

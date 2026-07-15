import {
  Badge,
  Button,
  ChatComposer,
  ChatThread,
  EmptyState,
  H2,
  InlineNotice,
  Span,
  Spinner,
  useChatScroll,
} from '@contenox/ui';
import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { useAcpWorkspace } from '../../hooks/useAcpWorkspace';
import type { AcpWorkspaceStatus } from '../../hooks/acpWorkspaceState';
import { useSetupStatus } from '../../hooks/useSetupStatus';
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

function SetupRequiredState({
  issue,
  fallbackMessage,
}: {
  issue: SetupIssue | null;
  fallbackMessage: string | null;
}) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const fixPath = issue ? getSetupIssueFixPath(issue) : '/settings';

  return (
    <div className="m-auto max-w-md p-6">
      <EmptyState
        variant="warning"
        title={t('acp_chat.setup_required_title')}
        description={issue?.message ?? fallbackMessage ?? t('acp_chat.setup_required_generic')}
      />
      <div className="mt-4 flex justify-center">
        <Button type="button" variant="primary" onClick={() => navigate(fixPath)}>
          {t('acp_chat.setup_required_action')}
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

function AcpChatWorkspace({ paramSessionId }: { paramSessionId: string | null }) {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const {
    workspace,
    session,
    newSession,
    openSession,
    sendPrompt,
    respondPermission,
    cancel,
    setConfigOption,
    reconnect,
  } = useAcpWorkspace();
  const { data: setupStatus } = useSetupStatus(true);
  const blockingSetupIssue = getBlockingSetupIssue(setupStatus);

  const [draft, setDraft] = useState('');
  const { containerRef, endRef } = useChatScroll({ deps: [session.items, session.pendingPermission] });

  // Deep-link open: attempt exactly once per (mount, target id) pair. Gated
  // on 'ready' OR 'error' (not 'connecting'/'disconnected'/'setup_required')
  // so a stale error from something unrelated doesn't block a legitimate
  // open, but we don't race the initial connect either.
  const openAttemptRef = useRef<string | null>(null);
  useEffect(() => {
    if (!paramSessionId) return;
    if (workspace.activeSessionId === paramSessionId) return;
    if (openAttemptRef.current === paramSessionId) return;
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
        sid = await newSession();
        navigate(`/chat/${sid}`, { replace: true });
      }
      sendPrompt(text);
    },
    [slashMenu.open, session.isPrompting, draft, workspace.activeSessionId, newSession, navigate, sendPrompt, cancel],
  );

  if (blockingSetupIssue || workspace.status === 'setup_required') {
    return <SetupRequiredState issue={blockingSetupIssue} fallbackMessage={workspace.error} />;
  }
  if (notFound) {
    return <NotFoundState onNewSession={() => navigate('/chat')} />;
  }
  if (workspace.status === 'disconnected') {
    return <DisconnectedState onRetry={reconnect} />;
  }
  if (workspace.status === 'error') {
    return <ErrorState message={workspace.error} onRetry={reconnect} />;
  }
  if (workspace.status === 'connecting' && session.items.length === 0) {
    return (
      <div className="flex h-full min-h-0 flex-1 items-center justify-center">
        <Spinner size="lg" aria-label={t('acp_chat.connecting_label')} />
      </div>
    );
  }

  const hasContent = session.items.length > 0;
  const composerDisabled = session.pendingPermission != null || (!session.isPrompting && workspace.status !== 'ready');

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
          <ConfigOptionControls configOptions={session.configOptions} onChange={setConfigOption} />
          {paramSessionId && (
            <Button type="button" variant="outline" palette="neutral" size="sm" onClick={() => navigate('/chat')}>
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
      {session.error && <InlineNotice variant="error">{session.error}</InlineNotice>}

      <PlanPanel entries={session.plan} />

      {!hasContent ? (
        <div className="m-auto">
          <EmptyState title={t('acp_chat.empty_title')} description={t('acp_chat.empty_description')} />
        </div>
      ) : (
        <ChatThread containerRef={containerRef} endRef={endRef}>
          <TranscriptItems session={session} agentName={workspace.agentName} />
        </ChatThread>
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

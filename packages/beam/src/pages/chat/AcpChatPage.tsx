import { Button, ChatThreadSkeleton, EmptyState } from '@contenox/ui';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { useNavbarSlot } from '../../components/NavbarSlot';
import { useAcpWorkspace } from '../../hooks/useAcpWorkspace';
import { useSetupStatus } from '../../hooks/useSetupStatus';
import { classifySetupIssueCode } from '../../lib/acpFailureKind';
import { getBlockingSetupIssue, getSetupIssueFixPath } from '../../lib/setupHealth';
import type { SetupIssue } from '../../lib/types';
import { SlashCommandRegistryProvider } from '../../lib/slashCommands';
import { ChatConnectionBadge } from './components/ChatConnectionBadge';
import { WorkspaceTabs } from './components/WorkspaceTabs';

/**
 * THE chat surface, mounted at `/chat` and `/chat/:sessionId` (see `App.tsx`,
 * which hoists `AcpWorkspaceProvider` into the authenticated app shell so this
 * page, the sessions rail, and any future ACP-backed surface share one
 * connection).
 *
 * Workspace-tabs Slice 2: the page is now the OUTER SHELL only — connection
 * status/gating and a thin header — hosting `WorkspaceTabs` as its body. The
 * per-session chat body lives in `ChatSessionTab`, one per open tab. The page
 * is deliberately NOT keyed on the route param anymore (the old single-view
 * page was): keeping the shell stable across `/chat/:sessionId` navigation is
 * what lets background tabs stay mounted, subscribed, and streaming.
 */
export default function AcpChatPage() {
  return (
    <SlashCommandRegistryProvider>
      <AcpChatWorkspace />
    </SlashCommandRegistryProvider>
  );
}

/**
 * Full-page "setup required" state — see the taxonomy note on
 * `classifySetupIssueCode` / `SessionBanners`' `ExecutionErrorBanner`.
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

function AcpChatWorkspace() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { sessionId: paramSessionId } = useParams<{ sessionId?: string }>();
  const { workspace, session, openSessionIds, reconnect } = useAcpWorkspace();

  // Connection status + the attributed agent name now live in the global navbar
  // (reclaiming the vertical space of the old in-body header strip) via the
  // navbar slot. `ChatConnectionBadge` reads the workspace/staged-agent contexts
  // itself, so it stays correct rendered up in the shell's navbar. Injected here
  // unconditionally (before the early returns below) so the badge is present for
  // every workspace status, not only the ready one.
  useNavbarSlot(<ChatConnectionBadge />);

  const { data: setupStatus, refetch: refetchSetupStatus } = useSetupStatus(true);
  const blockingSetupIssue = getBlockingSetupIssue(setupStatus);

  const handleRetryBackend = useCallback(() => {
    void refetchSetupStatus();
    void reconnect();
  }, [refetchSetupStatus, reconnect]);

  // A full-page "session not found" only when a LONE deep link failed (no other
  // tabs open). When other tabs are open, a bad deep link simply doesn't get a
  // tab — the shell stays put so the live tabs are preserved.
  const notFound =
    paramSessionId != null &&
    openSessionIds.length === 0 &&
    workspace.sessionLoadState === 'not_found' &&
    !openSessionIds.includes(paramSessionId);

  const handleNewSession = useCallback(() => navigate('/chat'), [navigate]);

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
      <div className="flex h-full min-h-0 flex-1 flex-col" role="status" aria-label={t('acp_chat.connecting_label')}>
        <ChatThreadSkeleton className="flex-1" />
      </div>
    );
  }

  // The old header strip (title + status + agent) is gone — its content moved
  // into the navbar via `useNavbarSlot(<ChatConnectionBadge/>)` above, so the tab
  // strip + per-session config toolbar (WorkspaceTabs → ChatSessionTab) is the
  // first row of the chat body, directly under the navbar.
  return (
    <div className="bg-surface dark:bg-dark-surface flex h-full min-h-0 flex-col">
      <WorkspaceTabs />
    </div>
  );
}

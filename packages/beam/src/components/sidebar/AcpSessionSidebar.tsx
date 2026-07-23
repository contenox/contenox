import { Button, Span, Spinner } from '@contenox/ui';
import { ChevronDown, Folder, Trash2 } from 'lucide-react';
import { useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { Link, useMatch, useNavigate } from 'react-router-dom';
import { useAcpWorkspace } from '../../hooks/useAcpWorkspace';
import { useWorkspaceRoots } from '../../hooks/useWorkspaceRoots';
import { externalAgentFromMeta, type SessionInfo } from '../../lib/acp';
import { adoptResultFromMeta } from '../../lib/adoptMeta';
import { relativeTime } from '../../lib/relativeTime';
import { useStagedAgent } from '../../lib/stagedAgent';
import { useStagedRoot } from '../../lib/stagedRoot';
import { workspaceNameForCwd } from '../../lib/workspaceRoots';
import { AgentPicker } from '../AgentPicker';
import { meaningfulTitle, workspaceLabel } from '../../pages/chat/lib/sessionLabel';
import { startNewChat } from './newChatIntent';

/**
 * App-shell rail for the ACP workspace (Stage 4 replaced the console/REST
 * chat-history-backed rail with this ACP-native one — see `App.tsx`, which
 * mounts both this and `pages/chat/AcpChatPage.tsx` under the same hoisted
 * `AcpWorkspaceProvider` so they share one connection and roster). The old
 * rail and the console/legacy chat pages were deleted in Stage 5.
 *
 * Workspace-tabs Slice 2: clicking a session no longer just navigates a
 * single-view page — the `/chat/:sessionId` route now opens/focuses that
 * session's TAB (the tab-model reconciles the param; see `WorkspaceTabs`), so
 * the rail keeps driving via `<Link to="/chat/:id">` and several sessions stay
 * open at once.
 */

export function AcpSessionSidebar({ setIsOpen }: { setIsOpen: (open: boolean) => void }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const match = useMatch('/chat/:sessionId');
  const activeSessionId = match?.params.sessionId;
  const { workspace, sessions, deleteSession, focusEmptyTab } = useAcpWorkspace();
  const { roots } = useWorkspaceRoots();
  const { setStagedAgent } = useStagedAgent();
  const { setStagedRoot } = useStagedRoot();

  // ONE path for starting a fresh chat: the plain "New session" button is just
  // `startNew(null)` (native contenox — clears any staged agent), and the agent
  // picker's `onSelect` is `startNew(name)`. Sharing `startNewChat` guarantees
  // the two affordances behave identically (stage the pick, drive the tab-model
  // to the empty surface via `focusEmptyTab`, route to `/chat`, collapse the
  // mobile rail) and can never drift. Driving `focusEmptyTab` — the same
  // controller action `useWorkspaceTabs` uses — is what makes the transition
  // deterministic from a focused session tab: a bare navigate alone was reverted
  // by the tab↔route sync. The empty surface reads the staged agent reactively,
  // so a pick made while already on `/chat` (where the navigate is a no-op) still
  // shows and binds. See `newChatIntent.ts`.
  const startNew = useCallback(
    (agent: string | null) =>
      startNewChat(agent, {
        setStagedAgent,
        setStagedRoot,
        focusEmptyTab,
        navigate,
        closeSidebar: () => setIsOpen(false),
      }),
    [setStagedAgent, setStagedRoot, focusEmptyTab, navigate, setIsOpen],
  );

  const handleDelete = (session: SessionInfo) => {
    // An ADOPTED session is never deletable from here: session/delete STOPS the
    // running dispatch (including for any other viewer of it — see
    // acpsvc/adopt.go's teardown asymmetry), so the delete affordance is hidden
    // for adopted sessions and this is a defensive second gate. Leaving/closing
    // the tab detaches; ending the run is a fleet-board Stop, not a chat delete.
    if (adoptResultFromMeta(session._meta)) return;
    const label = meaningfulTitle(session) ?? t('acp_sidebar.session_fallback_label', { shortId: session.sessionId.slice(0, 8) });
    if (!window.confirm(t('acp_sidebar.confirm_delete', { name: label }))) return;
    deleteSession(session.sessionId);
    if (session.sessionId === activeSessionId) {
      navigate('/chat');
    }
  };

  const isInitialLoad = workspace.status === 'connecting' && workspace.sessions.length === 0;

  // The PROJECT a session belongs to — the explicit name of the registered root
  // that contains its cwd, else the cwd basename (see workspaceNameForCwd). This
  // is BOTH the per-row label and the grouping key, so a row and its group header
  // always read the same name.
  const projectLabelFor = (session: SessionInfo): string =>
    workspaceNameForCwd(session.cwd, roots) ?? workspaceLabel(session.cwd);

  // One session row. `showProjectLabel` is false when the row sits under a project
  // group header (which already names the project), so the per-row folder line is
  // dropped as redundant; it stays true in the flat, single-project layout.
  const renderSessionRow = (session: SessionInfo, showProjectLabel: boolean) => {
    const isActive = activeSessionId === session.sessionId;
    const label = meaningfulTitle(session) ?? t('acp_sidebar.session_fallback_label', { shortId: session.sessionId.slice(0, 8) });
    // relativeTime is total (non-optional string in, string out); a session
    // with no updatedAt yet is this call site's own absent-timestamp case, so
    // it is handled here rather than by widening the shared function.
    const relative = session.updatedAt
      ? relativeTime(session.updatedAt, i18n.language, t('common.just_now'))
      : null;
    const agentName = externalAgentFromMeta(session._meta);
    const workspaceName = showProjectLabel ? projectLabelFor(session) : null;
    // Adopted sessions expose NO delete affordance here: deleting one
    // stops the running dispatch (see handleDelete). Detach is via close.
    const adopted = adoptResultFromMeta(session._meta) != null;
    // A background (non-focused) session with a permission request waiting
    // on the user surfaces a subtle dot here so it is discoverable while
    // its tab is out of view. Only OPEN (subscribed) sessions have a live
    // slice, so this is naturally scoped to sessions that can actually have
    // a pending request.
    const hasPendingPermission = sessions.slices[session.sessionId]?.pendingPermission != null;
    return (
      <div
        key={session.sessionId}
        className={`group flex items-center gap-1 rounded-lg border p-1 pl-3 transition-colors duration-150 ${
          isActive
            ? 'bg-surface-200 dark:bg-dark-surface-200 border-surface-400 dark:border-dark-surface-600'
            : 'bg-surface-100 dark:bg-dark-surface-100 border-surface-200 dark:border-dark-surface-700 hover:bg-surface-200 dark:hover:bg-dark-surface-200'
        }`}>
        <Link
          to={`/chat/${session.sessionId}`}
          onClick={() => setIsOpen(false)}
          className="min-w-0 flex-1 py-2 text-left">
          <Span className="text-text dark:text-dark-text line-clamp-2 text-xs">{label}</Span>
          {hasPendingPermission && (
            <Span
              className="text-warning-800 dark:text-warning-300 mt-1 flex items-center gap-1.5 text-xs font-medium"
              title={t('acp_sidebar.pending_permission')}>
              <span
                aria-hidden
                className="bg-warning-500 h-1.5 w-1.5 shrink-0 rounded-full"
              />
              <span className="truncate">{t('acp_sidebar.pending_permission')}</span>
            </Span>
          )}
          {agentName && (
            <Span
              className="text-text-muted dark:text-dark-text-muted mt-1 block truncate text-xs"
              title={t('acp_chat.agent_label', { name: agentName })}>
              {agentName}
            </Span>
          )}
          {(workspaceName || relative) && (
            <div className="text-text-muted dark:text-dark-text-muted mt-1 flex items-center justify-between gap-2 text-xs">
              {workspaceName ? (
                <span
                  className="flex min-w-0 items-center gap-1"
                  title={t('acp_sidebar.workspace_label', { path: session.cwd ?? '' })}>
                  <Folder className="h-3 w-3 shrink-0" aria-hidden />
                  <span className="truncate">{workspaceName}</span>
                </span>
              ) : (
                <span />
              )}
              {relative && <span className="shrink-0">{relative}</span>}
            </div>
          )}
        </Link>
        {!adopted && (
          <Button
            aria-label={t('acp_sidebar.delete_label', { name: label })}
            className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
            onClick={() => handleDelete(session)}
            size="icon"
            type="button"
            variant="ghost">
            <Trash2 className="h-4 w-4" aria-hidden />
          </Button>
        )}
      </div>
    );
  };

  // Group the (already newest-first) sessions by project, preserving that order
  // both across groups (a group appears when its most-recent session does) and
  // within each group. One project → render flat (a lone header would be noise,
  // and the per-row label already disambiguates); two or more → header per group.
  const sessionGroups: { name: string; items: SessionInfo[] }[] = [];
  const groupIndexByName = new Map<string, number>();
  for (const session of workspace.sessions) {
    const name = projectLabelFor(session);
    let gi = groupIndexByName.get(name);
    if (gi === undefined) {
      gi = sessionGroups.length;
      groupIndexByName.set(name, gi);
      sessionGroups.push({ name, items: [] });
    }
    sessionGroups[gi].items.push(session);
  }
  const useGroups = sessionGroups.length > 1;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-surface-300 dark:border-dark-surface-700 flex shrink-0 items-center gap-1 border-b p-3">
        <Button variant="primary" size="sm" className="min-w-0 flex-1" onClick={() => startNew(null)}>
          <span className="truncate">{t('acp_sidebar.new_session')}</span>
        </Button>
        {/* Start a new chat with a specific registered agent (native contenox at
            top). Hidden when no enabled agents are registered. */}
        <AgentPicker
          value={null}
          onSelect={startNew}
          trigger={
            <Button
              type="button"
              variant="outline"
              palette="neutral"
              size="icon"
              aria-label={t('acp_sidebar.new_session_with_agent')}
              title={t('acp_sidebar.new_session_with_agent')}
            >
              <ChevronDown className="h-4 w-4" aria-hidden />
            </Button>
          }
        />
      </div>
      <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-3" aria-label={t('acp_sidebar.title')}>
        {isInitialLoad ? (
          <div className="flex items-center justify-center gap-2 py-8">
            <Spinner size="md" />
            <Span className="text-text-muted text-sm">{t('acp_sidebar.loading')}</Span>
          </div>
        ) : workspace.sessions.length === 0 ? (
          <Span className="text-text-muted text-sm">{t('acp_sidebar.empty_hint')}</Span>
        ) : useGroups ? (
          sessionGroups.map(group => (
            <div key={group.name} className="space-y-1">
              <div className="text-text-muted dark:text-dark-text-muted flex items-center gap-1.5 px-1 pt-2 pb-0.5 text-xs font-medium">
                <Folder className="h-3 w-3 shrink-0" aria-hidden />
                <span className="min-w-0 truncate">{group.name}</span>
                <span className="text-text-muted/70 dark:text-dark-text-muted/70 shrink-0">
                  {group.items.length}
                </span>
              </div>
              {group.items.map(session => renderSessionRow(session, false))}
            </div>
          ))
        ) : (
          workspace.sessions.map(session => renderSessionRow(session, true))
        )}
      </nav>
    </div>
  );
}

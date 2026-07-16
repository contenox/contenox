import { Button, Span, Spinner } from '@contenox/ui';
import { Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Link, useMatch, useNavigate } from 'react-router-dom';
import { useAcpWorkspace } from '../../hooks/useAcpWorkspace';
import type { SessionInfo } from '../../lib/acp';

/**
 * App-shell rail for the ACP workspace (Stage 4 replaced the console/REST
 * chat-history-backed rail with this ACP-native one — see `App.tsx`, which
 * mounts both this and `pages/chat/AcpChatPage.tsx` under the same hoisted
 * `AcpWorkspaceProvider` so they share one connection and roster). The old
 * rail and the console/legacy chat pages were deleted in Stage 5.
 */

/**
 * `session/list` currently reports `Title` as a copy of the session id
 * itself (see `acpsvc/session.go`'s `ListSessions` — ACP sessions have no
 * distinct display name yet), so a title that merely echoes the id carries no
 * information; fall back to a short-id label in that case exactly as if no
 * title had been sent at all. Forward-compatible: a future backend that sends
 * a real friendly title will just work. Returns `null` when there is no
 * meaningful title — the caller renders the i18n'd short-id fallback itself
 * (kept out of this function so it stays free of the `t()` dependency).
 */
function meaningfulTitle(session: SessionInfo): string | null {
  const title = session.title?.trim();
  if (title && title !== session.sessionId) return title;
  return null;
}

function relativeTimeLabel(updatedAt: string | undefined, locale: string, justNowLabel: string): string | null {
  if (!updatedAt) return null;
  const then = Date.parse(updatedAt);
  if (Number.isNaN(then)) return null;
  const diffSec = Math.round((Date.now() - then) / 1000);
  if (diffSec < 45) return justNowLabel;

  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });
  const diffMin = Math.round(diffSec / 60);
  if (diffMin < 60) return rtf.format(-diffMin, 'minute');
  const diffHour = Math.round(diffMin / 60);
  if (diffHour < 24) return rtf.format(-diffHour, 'hour');
  const diffDay = Math.round(diffHour / 24);
  if (diffDay < 30) return rtf.format(-diffDay, 'day');
  const diffMonth = Math.round(diffDay / 30);
  if (diffMonth < 12) return rtf.format(-diffMonth, 'month');
  return rtf.format(-Math.round(diffMonth / 12), 'year');
}

export function AcpSessionSidebar({ setIsOpen }: { setIsOpen: (open: boolean) => void }) {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const match = useMatch('/chat/:sessionId');
  const activeSessionId = match?.params.sessionId;
  const { workspace, deleteSession, clearActiveSession } = useAcpWorkspace();

  const handleNewSession = () => {
    // Clear at CLICK time, before navigating — see
    // acpWorkspaceController.ts's clearActiveSession() doc comment (BUG 1).
    clearActiveSession();
    navigate('/chat');
    setIsOpen(false);
  };

  const handleDelete = (session: SessionInfo) => {
    const label = meaningfulTitle(session) ?? t('acp_sidebar.session_fallback_label', { shortId: session.sessionId.slice(0, 8) });
    if (!window.confirm(t('acp_sidebar.confirm_delete', { name: label }))) return;
    deleteSession(session.sessionId);
    if (session.sessionId === activeSessionId) {
      navigate('/chat');
    }
  };

  const isInitialLoad = workspace.status === 'connecting' && workspace.sessions.length === 0;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-surface-300 dark:border-dark-surface-700 shrink-0 border-b p-3">
        <Button variant="primary" size="sm" className="w-full" onClick={handleNewSession}>
          <span className="truncate">{t('acp_sidebar.new_session')}</span>
        </Button>
      </div>
      <nav className="min-h-0 flex-1 space-y-1 overflow-y-auto p-3" aria-label={t('acp_sidebar.title')}>
        {isInitialLoad ? (
          <div className="flex items-center justify-center gap-2 py-8">
            <Spinner size="md" />
            <Span className="text-text-muted text-sm">{t('acp_sidebar.loading')}</Span>
          </div>
        ) : workspace.sessions.length === 0 ? (
          <Span className="text-text-muted text-sm">{t('acp_sidebar.empty_hint')}</Span>
        ) : (
          workspace.sessions.map(session => {
            const isActive = activeSessionId === session.sessionId;
            const label = meaningfulTitle(session) ?? t('acp_sidebar.session_fallback_label', { shortId: session.sessionId.slice(0, 8) });
            const relative = relativeTimeLabel(session.updatedAt, i18n.language, t('acp_sidebar.just_now'));
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
                  {relative && (
                    <Span className="text-text-muted dark:text-dark-text-muted mt-1 block text-xs">{relative}</Span>
                  )}
                </Link>
                <Button
                  aria-label={t('acp_sidebar.delete_label', { name: label })}
                  className="opacity-0 group-hover:opacity-100 group-focus-within:opacity-100"
                  onClick={() => handleDelete(session)}
                  size="icon"
                  type="button"
                  variant="ghost">
                  <Trash2 className="h-4 w-4" aria-hidden />
                </Button>
              </div>
            );
          })
        )}
      </nav>
    </div>
  );
}

import { Button, Tabs, TabPanel, TabPanels, type Tab } from '@contenox/ui';
import { Plus } from 'lucide-react';
import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { useAcpWorkspace } from '../../../hooks/useAcpWorkspace';
import { useWorkspaceTabs } from '../../../hooks/useWorkspaceTabs';
import { meaningfulTitle } from '../lib/sessionLabel';
import { ChatSessionTab } from './ChatSessionTab';

/** TabPanel id for the empty/new-chat surface — cannot collide with a session id. */
const EMPTY_PANEL_ID = '__empty__';

/**
 * The chat workspace's in-app tab area (workspace-tabs Slice 2): a closeable
 * tab strip (reusing `@contenox/ui` `Tabs`) plus one `TabPanel` per open chat
 * tab, each hosting a `ChatSessionTab` for its own session. Inactive panels
 * stay MOUNTED (`hidden`, not unmounted), so a backgrounded session keeps its
 * live subscription + streaming state and its composer draft while another tab
 * is shown — the whole point of the multi-session foundation.
 *
 * The tab-model (`useWorkspaceTabs`) is the single owner of which tabs exist and
 * which is active; it drives the controller's focus. Routing is reconciled both
 * ways here: the `/chat/:sessionId` param opens/focuses that session's tab
 * (deep-linking, sidebar clicks, browser back), and any change to the active
 * tab pushes the matching URL.
 *
 * The empty state (no open tabs) is today's empty chat: a single full-height
 * `ChatSessionTab` bound to no session that lazily creates one — and promotes
 * itself to a real tab — on first submit.
 */
export function WorkspaceTabs() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { sessionId: paramSessionId } = useParams<{ sessionId?: string }>();
  const { workspace } = useAcpWorkspace();
  const tabModel = useWorkspaceTabs();
  const { tabs, activeId, open, close, focus, openEmpty } = tabModel;

  // Remounts the empty surface after it spawns a session, so its staged config /
  // draft start fresh for the next new chat (the old single-view page got this
  // for free by being keyed on the route param).
  const emptyKeyRef = useRef(0);

  // Distinguishes an INTENTIONAL move to the empty surface (new session / closed
  // the last tab) from the transient window during deep-link adoption, where
  // `activeId` is briefly null before the param's tab is opened — see the URL
  // effect below.
  const intentToEmptyRef = useRef(false);

  // URL (source of truth on entry) -> tab model. Fires on mount and whenever the
  // param changes (sidebar click, browser back, a fresh deep link). `open`
  // dedups (focus if already open); both `open`/`openEmpty` are stable.
  useEffect(() => {
    if (paramSessionId) open(paramSessionId);
    else openEmpty();
  }, [paramSessionId, open, openEmpty]);

  // Active tab -> URL. A real active tab always reflects its id; the empty
  // surface only pushes `/chat` when the move there was intentional (guarding
  // the deep-link adoption window, where activeId is transiently null).
  useEffect(() => {
    if (activeId !== null) {
      navigate(`/chat/${activeId}`, { replace: true });
      return;
    }
    if (intentToEmptyRef.current) {
      intentToEmptyRef.current = false;
      navigate('/chat', { replace: true });
    }
  }, [activeId, navigate]);

  const handleSessionCreated = useCallback(
    (sid: string) => {
      emptyKeyRef.current += 1;
      open(sid);
    },
    [open],
  );

  const handleNewSession = useCallback(() => {
    intentToEmptyRef.current = true;
    openEmpty();
  }, [openEmpty]);

  const handleClose = useCallback(
    (id: string) => {
      if (tabs.length === 1 && tabs[0] === id) intentToEmptyRef.current = true;
      close(id);
    },
    [tabs, close],
  );

  const sessionLabelFor = useCallback(
    (id: string): string => {
      const info = workspace.sessions.find(s => s.sessionId === id);
      const title = info ? meaningfulTitle(info) : null;
      return title ?? t('acp_sidebar.session_fallback_label', { shortId: id.slice(0, 8) });
    },
    [workspace.sessions, t],
  );

  // No open tabs: today's empty chat, full-height, no strip.
  if (tabs.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <ChatSessionTab
          key={`empty-${emptyKeyRef.current}`}
          sessionId={null}
          onSessionCreated={handleSessionCreated}
          onNewSession={handleNewSession}
        />
      </div>
    );
  }

  const stripTabs: Tab[] = tabs.map(id => {
    const label = sessionLabelFor(id);
    return {
      id,
      label: <span className="max-w-[12rem] truncate">{label}</span>,
      closable: true,
      closeLabel: t('acp_chat.tab_close_label', { name: label }),
    };
  });

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center gap-1 border-b px-2">
        <Tabs
          tabs={stripTabs}
          activeTab={activeId ?? EMPTY_PANEL_ID}
          onTabChange={id => focus(id)}
          onClose={handleClose}
          className="min-w-0 flex-1 flex-nowrap"
        />
        <Button
          type="button"
          variant={activeId === null ? 'primary' : 'ghost'}
          palette="neutral"
          size="sm"
          className="shrink-0"
          aria-label={t('acp_chat.new_tab_label')}
          onClick={handleNewSession}>
          <Plus className="h-4 w-4" />
          <span className="ml-1 hidden sm:inline">{t('acp_sidebar.new_session')}</span>
        </Button>
      </div>

      <TabPanels>
        {tabs.map(id => (
          <TabPanel key={id} tabId={id} activeTab={activeId ?? EMPTY_PANEL_ID}>
            <ChatSessionTab
              sessionId={id}
              onSessionCreated={handleSessionCreated}
              onNewSession={handleNewSession}
            />
          </TabPanel>
        ))}
        <TabPanel key={`empty-${emptyKeyRef.current}`} tabId={EMPTY_PANEL_ID} activeTab={activeId ?? EMPTY_PANEL_ID}>
          <ChatSessionTab
            sessionId={null}
            onSessionCreated={handleSessionCreated}
            onNewSession={handleNewSession}
          />
        </TabPanel>
      </TabPanels>
    </div>
  );
}

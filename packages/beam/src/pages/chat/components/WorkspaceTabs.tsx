import { Tabs, TabPanel, TabPanels, type Tab } from '@contenox/ui';
import { useCallback, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate, useParams } from 'react-router-dom';
import { useAcpWorkspace } from '../../../hooks/useAcpWorkspace';
import { useWorkspaceTabs } from '../../../hooks/useWorkspaceTabs';
import { useAdoptIntent } from '../../../lib/adoptIntent';
import { meaningfulTitle } from '../lib/sessionLabel';
import { routeForActiveTab } from '../lib/workspaceTabs';
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
  const { workspace, adoptSession } = useAcpWorkspace();
  const tabModel = useWorkspaceTabs();
  const { tabs, activeId, open, close, focus, openEmpty } = tabModel;

  // Eager fleet ADOPTION: an entry point (fleet board / mission detail) staged an
  // adopt intent and routed here (see `adoptIntent.ts`). Unlike a staged agent —
  // consumed lazily on the first prompt — the operator wants to watch the running
  // dispatch immediately, so we adopt the moment the connection is ready and
  // promote the resulting upstream session to a tab (its URL then follows). The
  // intent is consumed one-shot BEFORE the await (a ready-state re-render must not
  // re-fire it), and `adoptSession` is additive so any tab already open stays.
  const { adoptIntent, setAdoptIntent } = useAdoptIntent();
  const adoptingRef = useRef(false);
  useEffect(() => {
    if (!adoptIntent || workspace.status !== 'ready' || adoptingRef.current) return;
    adoptingRef.current = true;
    const intent = adoptIntent;
    setAdoptIntent(null);
    void adoptSession({ instanceId: intent.instanceId, sessionId: intent.sessionId }, intent.cwd)
      .then(sid => {
        open(sid);
      })
      .catch(() => {
        // A failed adopt (instance gone, or an old serve rejecting it) is already
        // surfaced as an inline error in the focused slice by adoptSession.
      })
      .finally(() => {
        adoptingRef.current = false;
      });
  }, [adoptIntent, workspace.status, adoptSession, open, setAdoptIntent]);

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

  // react-router's `useNavigate` (this app uses HashRouter → the non-data-router
  // `useNavigateUnstable`) hands back a NEW `navigate` identity on every location
  // change — its callback closes over `locationPathname`. We deliberately hold it
  // in a ref and depend ONLY on `activeId` below, so the tab→URL effect fires when
  // the ACTIVE TAB changes, never merely because a navigation just changed
  // `navigate`'s identity. The previous `[activeId, navigate]` deps re-ran this
  // effect on that identity change while `activeId` was still the OUTGOING session
  // id, so a sidebar "new session" (`navigate('/chat')`) was instantly reverted
  // back to `/chat/:id` (replace) before the tab model reached the empty surface.
  // Every target here is an absolute path, so a ref-held (latest) navigate is safe.
  const navigateRef = useRef(navigate);
  navigateRef.current = navigate;

  // Active tab -> URL. The target is decided by the pure `routeForActiveTab` (see
  // workspaceTabs.ts): a real tab -> its `/chat/:id`; the empty surface -> bare
  // `/chat` only when the move there was intentional (a one-shot flag, consumed
  // here), else `null` = "leave the URL alone" (the transient null of deep-link
  // adoption).
  useEffect(() => {
    const intentionalEmpty = intentToEmptyRef.current;
    if (activeId === null) intentToEmptyRef.current = false;
    const target = routeForActiveTab(activeId, intentionalEmpty);
    if (target !== null) navigateRef.current(target, { replace: true });
  }, [activeId]);

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
      {/* No "new session" affordance here: the session sidebar owns the single
          creation entry point (and the narrow-viewport chat toolbar mirrors it).
          A second one in the tab strip was redundant. */}
      <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center gap-1 border-b px-2">
        <Tabs
          tabs={stripTabs}
          activeTab={activeId ?? EMPTY_PANEL_ID}
          onTabChange={id => focus(id)}
          onClose={handleClose}
          className="min-w-0 flex-1 flex-nowrap"
        />
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

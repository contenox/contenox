import { Cpu, Database, Settings, type LucideIcon } from 'lucide-react';
import { lazy } from 'react';
import { Navigate, useParams } from 'react-router-dom';
import i18n, { type TranslationKey } from '../i18n';
import HomeRedirect from '../pages/HomeRedirect.tsx';
import { LOGIN_ROUTE } from './routeConstants.ts';

const AcpChatPage = lazy(() => import('../pages/chat/AcpChatPage.tsx'));
const BackendsPage = lazy(() => import('../pages/admin/backends/BackendPage.tsx'));
const ControlPlanePage = lazy(() => import('../pages/admin/control/ControlPlanePage.tsx'));
const SettingsPage = lazy(() => import('../pages/admin/settings/SettingsPage.tsx'));
const ByePage = lazy(() => import('../pages/public/bye/Bye.tsx'));
const AuthPage = lazy(() => import('../pages/public/login/AuthPage.tsx'));

// pages/admin/chats/{ChatPage,ChatLandingPage}.tsx and
// pages/admin/console/ConsolePage.tsx are no longer imported here — every
// route that used to point at them now redirects to /chat (below). The files
// themselves are untouched; Stage 5 deletes them.
const LegacyChatsRedirect = () => <Navigate to="/chat" replace />;
const LegacyConsoleRedirect = () => <Navigate to="/chat" replace />;
/** `/chat-acp/:sessionId` -> `/chat/:sessionId`, preserving the id (unlike the other legacy redirects, which all just land bare on /chat). */
const AcpChatSessionRedirect = () => {
  const { sessionId } = useParams<{ sessionId: string }>();
  return <Navigate to={`/chat/${sessionId}`} replace />;
};

interface RouteConfig {
  path: string;
  element: React.ComponentType;
  label: string;
  icon?: React.ReactNode;
  showInNav?: boolean;
  system?: boolean;
  protected: boolean;
  showInShelf?: boolean;
}

export type AdminNavItem = {
  path: string;
  label: string;
  icon?: React.ReactNode;
};

type AdminRouteDefinition = {
  path: string;
  element: React.ComponentType;
  labelKey: TranslationKey;
  Icon: LucideIcon;
};

const adminRouteDefinitions: AdminRouteDefinition[] = [
  { path: '/backends', element: BackendsPage, labelKey: 'navbar.backends', Icon: Database },
  { path: '/settings', element: SettingsPage, labelKey: 'navbar.settings', Icon: Settings },
];

/** Admin destinations for the control-plane menu and hub; route paths unchanged. */
export const adminNavItems: AdminNavItem[] = adminRouteDefinitions
  .map(({ path, labelKey, Icon }) => {
    const item = {
      path,
      label: i18n.t(labelKey),
      icon: <Icon className="h-[1em] w-[1em]" />,
    };
    if (path !== '/backends') {
      return [item];
    }
    return [
      item,
      {
        path: '/backends?tab=local-runtime',
        label: i18n.t('navbar.local_runtime_modeld'),
        icon: <Cpu className="h-[1em] w-[1em]" />,
      },
    ];
  })
  .flat();

const adminRoutes: RouteConfig[] = adminRouteDefinitions.map(def => ({
  path: def.path,
  element: def.element,
  label: i18n.t(def.labelKey),
  icon: <def.Icon className="h-[1em] w-[1em]" />,
  showInNav: false,
  protected: true,
  showInShelf: false,
}));

export const routes: RouteConfig[] = [
  {
    path: '/',
    element: HomeRedirect,
    label: '',
    showInNav: false,
    protected: false,
    showInShelf: false,
  },
  {
    // THE chat surface (Stage 4 cutover) — bare /chat lazy-creates a session
    // on first submit (D5); see pages/chat/AcpChatPage.tsx.
    path: '/chat',
    element: AcpChatPage,
    label: i18n.t('navbar.chat'),
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    path: '/chat/:sessionId',
    element: AcpChatPage,
    label: i18n.t('navbar.chat'),
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    path: '/chats',
    element: LegacyChatsRedirect,
    label: '',
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    // Pre-Stage-4 opt-in ACP workspace surface — now just an alias for /chat.
    path: '/chat-acp',
    element: LegacyChatsRedirect,
    label: '',
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    path: '/chat-acp/:sessionId',
    element: AcpChatSessionRedirect,
    label: '',
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    // Pre-ACP chat surface, kept reachable (as a redirect) during the transition.
    path: '/chat-legacy/:chatId',
    element: LegacyConsoleRedirect,
    label: '',
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    path: '/console',
    element: LegacyConsoleRedirect,
    label: '',
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    path: '/console/:chatId',
    element: LegacyConsoleRedirect,
    label: '',
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  {
    path: '/control',
    element: ControlPlanePage,
    label: '',
    showInNav: false,
    protected: true,
    showInShelf: false,
  },
  ...adminRoutes,
  {
    path: LOGIN_ROUTE,
    element: AuthPage,
    label: i18n.t('login.title'),
    showInNav: false,
    protected: false,
    showInShelf: false,
  },
  {
    path: '/bye',
    element: ByePage,
    label: i18n.t('navbar.bye'),
    showInNav: false,
    system: true,
    protected: false,
    showInShelf: false,
  },
  {
    path: '*',
    element: () => i18n.t('pages.not_found'),
    label: i18n.t('pages.not_found'),
    showInNav: false,
    system: true,
    protected: false,
    showInShelf: false,
  },
];

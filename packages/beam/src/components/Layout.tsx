import { Button, LoadingState, Panel, SidebarToggle } from '@contenox/ui';
import React, { useContext, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useLocation, useNavigate } from 'react-router-dom';
import logoMarkLightUrl from '../assets/logo-mark-light.svg?url';
import logoMarkDarkUrl from '../assets/logo-mark.svg?url';
import { useSetupStatus } from '../hooks/useSetupStatus';
import { useWizardDismissal } from '../hooks/useWizardDismissal';
import { AuthContext } from '../lib/authContext';
import { useTheme } from '../lib/ThemeProvider';
import { cn } from '../lib/utils';
import {
  clearWizardDismissal,
  computeSetupFingerprint,
  dismissWizard,
} from '../lib/wizardDismissal';
import { ControlPlaneDropdown } from './ControlPlaneDropdown';
import { useNavbarSlotValue } from './NavbarSlot';
import { OnboardingWizard } from './setup/OnboardingWizard';
import { Sidebar } from './sidebar/Sidebar';

function isDesktopViewport(): boolean {
  if (typeof window === 'undefined') return true;
  return window.matchMedia('(min-width: 640px)').matches;
}

type Props = {
  defaultOpen?: boolean;
  mainContent: React.ReactNode;
  /** Left rail content (e.g. chat sessions). */
  sidebarContent:
    | React.ReactNode
    | ((props: { isOpen: boolean; setIsOpen: (open: boolean) => void }) => React.ReactNode);
  className?: string;
};

export function Layout({ defaultOpen = true, mainContent, sidebarContent, className }: Props) {
  const [isSidebarOpen, setSidebarIsOpen] = useState(() => defaultOpen && isDesktopViewport());
  const [isControlPlaneOpen, setControlPlaneOpen] = useState(false);
  const { theme } = useTheme();
  const navigate = useNavigate();
  const { user } = useContext(AuthContext);
  const location = useLocation();
  const isOnLoginPage = location.pathname === '/login';
  const sidebarDisabled = !user;

  const { t } = useTranslation();
  const { data: setupData, isLoading: setupLoading } = useSetupStatus(!!user);
  const { record: dismissalRecord, manualOpen: wizardManuallyOpened } = useWizardDismissal();

  const setupComplete = useMemo(() => {
    if (!setupData) return false;
    const hasErrors = (setupData.issues ?? []).some(i => i.severity === 'error');
    return !hasErrors && setupData.reachableBackendCount > 0;
  }, [setupData]);

  // Completing setup clears a stale dismissal record — otherwise, if setup
  // later regresses, the wizard would compare against a fingerprint that was
  // never meant to survive completion.
  useEffect(() => {
    if (setupComplete && dismissalRecord !== null) {
      clearWizardDismissal();
    }
  }, [setupComplete, dismissalRecord]);

  // Dismissed only "for the state it was dismissed under": if setup regresses
  // to a different set of error issues, or backends go unreachable again, the
  // fingerprint no longer matches and the wizard re-arms automatically.
  const isDismissedForCurrentState =
    dismissalRecord !== null && dismissalRecord.fingerprint === computeSetupFingerprint(setupData);

  // manualOpen (Settings' "Run setup wizard") forces the wizard open even when
  // setup is already complete — the wizard is also a reconfiguration flow, not
  // just a recovery-from-broken one.
  const showWizard =
    !!user &&
    !setupLoading &&
    (wizardManuallyOpened || (!setupComplete && !isDismissedForCurrentState));
  // Persistent escape hatch: setup is still incomplete but the wizard is
  // dismissed for this exact state — surface a slim banner instead of nothing.
  const showIncompleteBanner = !!user && !setupLoading && !setupComplete && !showWizard;
  const logoUrl = theme === 'dark' ? logoMarkDarkUrl : logoMarkLightUrl;
  // A routed page can project one piece of chrome into the navbar center (today
  // the chat connection badge) rather than spending body height on its own
  // header strip — see components/NavbarSlot.tsx.
  const navbarSlot = useNavbarSlotValue();
  const renderedSidebarContent =
    typeof sidebarContent === 'function'
      ? sidebarContent({ isOpen: isSidebarOpen, setIsOpen: setSidebarIsOpen })
      : sidebarContent;

  const headerTitle = useMemo(() => {
    // "Beam" is the product name of the chat surface (same in every language) —
    // it replaces the old in-body "Chat" H2, which moved up into this navbar.
    if (location.pathname.startsWith('/chat')) return 'Beam';
    if (location.pathname.startsWith('/settings')) return 'Settings';
    if (location.pathname.startsWith('/backends')) return 'Backends';
    if (location.pathname.startsWith('/control')) return 'Control Plane';
    return 'Contenox';
  }, [location.pathname]);

  const handleDismissWizard = () => {
    dismissWizard(setupData);
  };

  useEffect(() => {
    const media = window.matchMedia('(min-width: 640px)');
    const handleChange = () => {
      if (!media.matches) {
        setSidebarIsOpen(false);
      }
    };
    handleChange();
    media.addEventListener('change', handleChange);
    return () => media.removeEventListener('change', handleChange);
  }, []);

  const navbar = (
    <Panel
      variant="bordered"
      className="flex h-16 shrink-0 items-center justify-between gap-4 bg-inherit px-4 text-inherit">
      <div className="flex shrink-0 items-center gap-4">
        {/* The wizard replaces the whole main area (see showWizard below) and
            never renders <Sidebar>, so toggling it here would do nothing
            visible — hide it instead of shipping a dead button. */}
        {!sidebarDisabled && !showWizard ? (
          <SidebarToggle isOpen={isSidebarOpen} onToggle={() => setSidebarIsOpen(!isSidebarOpen)} />
        ) : (
          <div className="w-9" />
        )}
        <div className="flex min-w-0 items-center gap-2">
          <img src={logoUrl} alt="" aria-hidden className="h-7 w-7 shrink-0" />
          <span className="text-text dark:text-dark-text truncate text-sm font-semibold tracking-normal sm:text-base">
            {headerTitle}
          </span>
        </div>
      </div>

      {/* The page-supplied navbar slot (chat connection badge), right-aligned
          next to the edge controls. Allowed to shrink and truncate so it never
          pushes them off-screen at narrow widths; empty (and inert) on routes
          that don't fill it. */}
      <div className="flex min-w-0 flex-1 items-center justify-end gap-2 px-2">{navbarSlot}</div>

      <div className="flex shrink-0 items-center gap-2">
        {user ? (
          <ControlPlaneDropdown isOpen={isControlPlaneOpen} setIsOpen={setControlPlaneOpen} />
        ) : (
          !isOnLoginPage && (
            <Button onClick={() => navigate('/login')} variant="primary" size="sm">
              Login
            </Button>
          )
        )}
      </div>
    </Panel>
  );

  if (user && setupLoading) {
    return (
      <div
        className={cn(
          'bg-surface-50 dark:bg-dark-surface-100 flex h-screen flex-col text-inherit',
          className,
        )}>
        {navbar}
        <LoadingState className="flex-1" />
      </div>
    );
  }

  if (showWizard) {
    return (
      <div
        className={cn(
          'bg-surface-50 dark:bg-dark-surface-100 flex h-screen flex-col text-inherit',
          className,
        )}>
        {navbar}
        <div className="min-h-0 flex-1 overflow-hidden">
          <OnboardingWizard data={setupData} onDismiss={handleDismissWizard} />
        </div>
      </div>
    );
  }

  return (
    <div
      className={cn(
        'bg-surface-50 dark:bg-dark-surface-100 flex h-screen flex-col text-inherit',
        className,
      )}>
      {navbar}
      {showIncompleteBanner && (
        <Panel
          variant="warning"
          className="flex shrink-0 flex-wrap items-center justify-between gap-2 rounded-none border-x-0 border-t-0 text-sm">
          <span>{t('setup.incomplete_banner_title')}</span>
          <Button variant="secondary" size="sm" onClick={clearWizardDismissal}>
            {t('setup.incomplete_banner_action')}
          </Button>
        </Panel>
      )}
      <div className="flex h-full min-h-0 flex-1 overflow-hidden">
        <Sidebar
          disabled={sidebarDisabled}
          isOpen={isSidebarOpen}
          setIsOpen={setSidebarIsOpen}
          items={[]}>
          {renderedSidebarContent}
        </Sidebar>
        <main className="bg-surface-50 dark:bg-dark-surface-100 min-h-0 min-w-0 flex-1 overflow-hidden">
          {mainContent}
        </main>
      </div>
    </div>
  );
}

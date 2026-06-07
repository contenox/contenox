import { Button, LoadingState, Panel, SidebarToggle } from '@contenox/ui';
import React, { useContext, useMemo, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import logoMarkDarkUrl from '../assets/logo-mark.svg?url';
import logoMarkLightUrl from '../assets/logo-mark-light.svg?url';
import { useSetupStatus } from '../hooks/useSetupStatus';
import { AuthContext } from '../lib/authContext';
import { useTheme } from '../lib/ThemeProvider';
import { cn } from '../lib/utils';
import { ControlPlaneDropdown } from './ControlPlaneDropdown';
import { OnboardingWizard } from './setup/OnboardingWizard';
import { Sidebar } from './sidebar/Sidebar';

const ONBOARDING_KEY = 'beam_onboarding_dismissed';

function isDismissed(): boolean {
  try {
    return localStorage.getItem(ONBOARDING_KEY) === '1';
  } catch {
    return false;
  }
}

type Props = {
  defaultOpen?: boolean;
  mainContent: React.ReactNode;
  /** Left rail content (e.g. chat sessions). */
  sidebarContent: React.ReactNode;
  className?: string;
};

export function Layout({
  defaultOpen = true,
  mainContent,
  sidebarContent,
  className,
}: Props) {
  const [isSidebarOpen, setSidebarIsOpen] = useState(defaultOpen);
  const [isControlPlaneOpen, setControlPlaneOpen] = useState(false);
  const { theme } = useTheme();
  const navigate = useNavigate();
  const { user } = useContext(AuthContext);
  const location = useLocation();
  const isOnLoginPage = location.pathname === '/login';
  const sidebarDisabled = !user;

  const [wizardDismissed, setWizardDismissed] = useState(isDismissed);
  const { data: setupData, isLoading: setupLoading } = useSetupStatus(!!user);

  const setupComplete = useMemo(() => {
    if (!setupData) return false;
    const hasErrors = (setupData.issues ?? []).some(i => i.severity === 'error');
    return !hasErrors && setupData.reachableBackendCount > 0;
  }, [setupData]);

  const showWizard = !!user && !wizardDismissed && !setupLoading && !setupComplete;
  const logoUrl = theme === 'dark' ? logoMarkDarkUrl : logoMarkLightUrl;

  const dismissWizard = () => {
    try {
      localStorage.setItem(ONBOARDING_KEY, '1');
    } catch { }
    setWizardDismissed(true);
  };

  const navbar = (
    <Panel
      variant="bordered"
      className="flex h-16 shrink-0 items-center justify-between gap-4 bg-inherit px-4 text-inherit">
      <div className="flex items-center gap-4">
        {!sidebarDisabled ? (
          <SidebarToggle
            isOpen={isSidebarOpen}
            onToggle={() => setSidebarIsOpen(!isSidebarOpen)}
          />
        ) : (
          <div className="w-9" />
        )}
        <div className="flex min-w-0 items-center gap-2">
          <img src={logoUrl} alt="" aria-hidden className="h-7 w-7 shrink-0" />
          <span className="truncate text-sm font-semibold tracking-normal text-text dark:text-dark-text sm:text-base">
            Contenox Beam
          </span>
        </div>
      </div>

      <div className="flex items-center gap-2">
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
      <div className={cn('bg-surface-50 dark:bg-dark-surface-100 flex h-screen flex-col text-inherit', className)}>
        {navbar}
        <LoadingState className="flex-1" />
      </div>
    );
  }

  if (showWizard) {
    return (
      <div className={cn('bg-surface-50 dark:bg-dark-surface-100 flex h-screen flex-col text-inherit', className)}>
        {navbar}
        <div className="flex-1 min-h-0 overflow-hidden">
          <OnboardingWizard data={setupData} onDismiss={dismissWizard} />
        </div>
      </div>
    );
  }

  return (
    <div className={cn('bg-surface-50 dark:bg-dark-surface-100 flex h-screen flex-col text-inherit', className)}>
      {navbar}
      <div className="flex h-full min-h-0 flex-1 overflow-hidden">
        <Sidebar
          disabled={sidebarDisabled}
          isOpen={isSidebarOpen}
          setIsOpen={setSidebarIsOpen}
          items={[]}>
          {sidebarContent}
        </Sidebar>
        <main className="bg-surface-50 dark:bg-dark-surface-100 min-h-0 min-w-0 flex-1 overflow-hidden">{mainContent}</main>
      </div>
    </div>
  );
}

import { Button, InlineNotice, LoadingState, Page } from '@contenox/ui';
import { SquareTerminal } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { TerminalView } from '../../../components/terminal/TerminalView';
import { RootSelector } from '../../../components/workspace/RootSelector';
import { useHostTerminalSession } from '../../../hooks/useHostTerminalSession';
import { useWorkspaceRoots } from '../../../hooks/useWorkspaceRoots';

/**
 * The standalone host-console surface: a full-page interactive shell on the
 * machine that serves this runtime, scoped to a chosen project root. It is the
 * top-level sibling of the terminal already embedded in the mission inspector
 * and chat canvas — same `useHostTerminalSession` + `TerminalView` stack, just
 * given the whole viewport.
 *
 * Responsive by construction: `TerminalView` fits xterm to its container via a
 * FitAddon + ResizeObserver (rotation, the mobile keyboard, or a root switch all
 * re-fit and re-send the PTY size), so the only layout job here is to give it a
 * full-height, non-scrolling box (`Page bodyScroll="hidden"`) with a header that
 * stacks on narrow screens. The shell opens READ-ONLY until an explicit
 * take-over (TerminalView owns that confirmation), and degrades to a graceful
 * "unavailable" state when serve does not expose the terminal feature (404).
 *
 * The cwd is part of the session identity: `useHostTerminalSession` memoizes one
 * PTY per `ownerKey`, so keying it by cwd means switching roots opens a fresh
 * shell there and switching back re-attaches to the still-live one rather than
 * spawning duplicates (server-side idle-timeout + max-sessions bound the rest).
 */
export default function ConsolePage() {
  const { t } = useTranslation();
  const { roots, isAbsent: rootsAbsent } = useWorkspaceRoots();
  // '' = serve's default root; a concrete path scopes the shell to that project.
  const [cwd, setCwd] = useState('');
  const { session, isAbsent, isLoading, error, retry } = useHostTerminalSession(
    `host-console:${cwd}`,
    { enabled: true, cwd },
  );

  return (
    <Page bodyScroll="hidden">
      <div className="flex h-full min-h-0 flex-col">
        <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-col gap-2 border-b px-3 py-2 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
          <div className="flex min-w-0 items-center gap-2">
            <SquareTerminal
              className="text-text-muted dark:text-dark-text-muted h-4 w-4 shrink-0"
              aria-hidden
            />
            <h1 className="text-text dark:text-dark-text truncate text-sm font-semibold">
              {t('hostTerminal.page_title')}
            </h1>
          </div>
          {/* Project scoping — hidden when the terminal feature is absent, since
              there is no shell to scope. RootSelector itself degrades to a free
              path input when serve publishes no workspace-root allowlist. */}
          {!isAbsent && (
            <label className="w-full sm:w-72 sm:shrink-0">
              <span className="sr-only">{t('hostTerminal.cwd_label')}</span>
              <RootSelector value={cwd} onChange={setCwd} roots={roots} isAbsent={rootsAbsent} />
            </label>
          )}
        </div>

        <div className="min-h-0 flex-1">
          {isAbsent ? (
            <div className="p-4 md:p-6">
              <InlineNotice variant="info">
                <p className="font-medium">{t('hostTerminal.unavailable_title')}</p>
                <p className="mt-1 text-xs">{t('hostTerminal.unavailable_body')}</p>
              </InlineNotice>
            </div>
          ) : error ? (
            <div className="p-4 md:p-6">
              <InlineNotice variant="error">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <span className="min-w-0">
                    {t('hostTerminal.session_error')} {error.message}
                  </span>
                  <Button type="button" variant="outline" size="xs" onClick={retry}>
                    {t('common.retry')}
                  </Button>
                </div>
              </InlineNotice>
            </div>
          ) : isLoading || !session ? (
            <div className="p-4 md:p-6">
              <LoadingState message={t('hostTerminal.connecting')} />
            </div>
          ) : (
            // Remount on a new session (root switch) so xterm is rebuilt cleanly.
            <TerminalView key={session.wsPath} wsPath={session.wsPath} />
          )}
        </div>
      </div>
    </Page>
  );
}

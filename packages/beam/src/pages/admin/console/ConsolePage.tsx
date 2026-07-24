import { Button, InlineNotice, LoadingState, Page } from '@contenox/ui';
import { SquareTerminal } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { TerminalView } from '../../../components/terminal/TerminalView';
import { useHostTerminalSession } from '../../../hooks/useHostTerminalSession';

/**
 * The break-glass operator console: a full-page interactive shell on the machine
 * that serves this runtime, for manual host intervention — unstick a wedged
 * process, restart or kill a service, free memory, or patch up whatever state an
 * agent left in a bad way. It is the operator's OWN shell (see terminalservice),
 * NOT an agent's terminal — agent command output is the separate ACP/shellsession
 * path rendered inside the chat/mission timeline.
 *
 * Deliberately un-scoped: this is a rescue tool, so it opens ONE shell at serve's
 * default root and you navigate from there — no project picker gating the way to
 * a prompt. (There is no confinement to scope, either: cwd only seeds the shell's
 * starting directory; the PTY runs as serve's OS user and can cd anywhere. Real
 * confinement is an agent concern — libsandbox / goja capabilities — not this
 * human surface.)
 *
 * Responsive by construction: `TerminalView` fits xterm to its container via a
 * FitAddon + ResizeObserver (rotation or the mobile keyboard re-fit and re-send
 * the PTY size), so the only layout job here is to give it a full-height,
 * non-scrolling box (`Page bodyScroll="hidden"`). The shell opens READ-ONLY until
 * an explicit take-over (TerminalView owns that confirmation), and degrades to a
 * graceful "unavailable" state when serve does not expose the terminal feature
 * (404).
 *
 * A single stable session key (`host-console`): `useHostTerminalSession` memoizes
 * one PTY per key, so leaving the page and returning re-attaches to the same
 * still-live shell — your history and half-typed command survive — rather than
 * spawning a duplicate (server-side idle-timeout + max-sessions bound the rest).
 */
export default function ConsolePage() {
  const { t } = useTranslation();
  const { session, isAbsent, isLoading, error, retry } = useHostTerminalSession('host-console', {
    enabled: true,
  });

  return (
    <Page bodyScroll="hidden">
      <div className="flex h-full min-h-0 flex-col">
        <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center gap-2 border-b px-3 py-2">
          <SquareTerminal
            className="text-text-muted dark:text-dark-text-muted h-4 w-4 shrink-0"
            aria-hidden
          />
          <h1 className="text-text dark:text-dark-text truncate text-sm font-semibold">
            {t('hostTerminal.page_title')}
          </h1>
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
            <TerminalView key={session.wsPath} wsPath={session.wsPath} />
          )}
        </div>
      </div>
    </Page>
  );
}

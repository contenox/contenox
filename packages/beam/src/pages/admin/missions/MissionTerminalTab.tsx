import { Button, InlineNotice, LoadingState } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { TerminalView } from '../../../components/terminal/TerminalView';
import { useHostTerminalSession } from '../../../hooks/useHostTerminalSession';

export interface MissionTerminalTabProps {
  /** Owner key for the memoized session — the mission id keeps one PTY per mission. */
  missionId: string;
}

/**
 * The mission inspector's Terminal tab body. Lazily code-split (see
 * MissionDetailPage) so xterm never enters the page's static bundle — the tab
 * pays for itself only when opened. Opens a PTY under the default workspace root
 * (the mission record carries no cwd), degrading to a graceful "unavailable"
 * state when serve does not expose the terminal feature (404), and surfacing a
 * real refusal (e.g. too many sessions) with a retry. The view itself is
 * read-only until an explicit take-over (see TerminalView).
 */
export default function MissionTerminalTab({ missionId }: MissionTerminalTabProps) {
  const { t } = useTranslation();
  const { session, isAbsent, isLoading, error, retry } = useHostTerminalSession(missionId, {
    enabled: true,
  });

  if (isAbsent) {
    return (
      <div className="p-4 md:p-6">
        <InlineNotice variant="info">
          <p className="font-medium">{t('hostTerminal.unavailable_title')}</p>
          <p className="mt-1 text-xs">{t('hostTerminal.unavailable_body')}</p>
        </InlineNotice>
      </div>
    );
  }

  if (error) {
    return (
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
    );
  }

  if (isLoading || !session) {
    return (
      <div className="p-4 md:p-6">
        <LoadingState message={t('hostTerminal.connecting')} />
      </div>
    );
  }

  return (
    <div className="border-surface-200 dark:border-dark-surface-600 h-[60vh] min-h-[24rem] overflow-hidden rounded-lg border">
      <TerminalView wsPath={session.wsPath} />
    </div>
  );
}

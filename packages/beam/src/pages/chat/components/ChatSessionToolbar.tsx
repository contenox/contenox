import { Button } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { AcpUsageState } from '../../../hooks/acpSessionState';
import type { SessionConfigOption, SessionConfigOptionValue } from '../../../lib/acp';
import { ConfigOptionControls } from './ConfigOptionControls';
import { UsageMeter } from './UsageMeter';

export interface ChatSessionToolbarProps {
  /** Context-window usage meter data (hidden by the meter until the agent reports usage). */
  usage: AcpUsageState | null;
  /** Per-session (or staged, on the empty chat) config controls. */
  configOptions: SessionConfigOption[];
  onConfigChange: (configId: string, value: SessionConfigOptionValue) => void;
  /** On a live session, renders the narrow-viewport "new session" affordance; hidden on the empty chat. */
  showNewSession: boolean;
  onNewSession: () => void;
}

/**
 * The per-session chat header strip: the usage meter, the config controls, and
 * the narrow-viewport "new session" affordance. Purely presentational —
 * extracted from `ChatSessionTab` so the tab body stays focused on the
 * transcript/composer flow and this reusable toolbar can be reasoned about (and
 * restyled) on its own. Neither panel toggle lives here: opening the terminal is
 * a right-edge rail in `CanvasRegion`, and the workspace-files toggle is the
 * mirroring left-edge rail in `ChatSessionTab` — each anchored on the side its
 * surface opens, not buried in this config strip.
 */
export function ChatSessionToolbar({
  usage,
  configOptions,
  onConfigChange,
  showNewSession,
  onNewSession,
}: ChatSessionToolbarProps) {
  const { t } = useTranslation();

  return (
    <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-wrap items-center justify-end gap-3 border-b px-3 py-2 sm:px-4">
      <UsageMeter usage={usage} />
      <ConfigOptionControls configOptions={configOptions} onChange={onConfigChange} />
      {showNewSession && (
        // Narrow-viewport "new session" affordance (the sidebar's is canonical
        // at sm+); opens a fresh empty tab.
        <Button type="button" variant="outline" palette="neutral" size="sm" className="sm:hidden" onClick={onNewSession}>
          {t('acp_chat.new_session')}
        </Button>
      )}
    </div>
  );
}

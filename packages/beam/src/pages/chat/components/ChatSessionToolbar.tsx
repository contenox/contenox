import { Button } from '@contenox/ui';
import { PanelLeft, PanelLeftClose } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { AcpUsageState } from '../../../hooks/acpSessionState';
import type { SessionConfigOption, SessionConfigOptionValue } from '../../../lib/acp';
import { ConfigOptionControls } from './ConfigOptionControls';
import { UsageMeter } from './UsageMeter';

export interface ChatSessionToolbarProps {
  /** Whether this session has a workspace root — gates the "show files" toggle. */
  hasWorkspaceRoot: boolean;
  /** Workspace file panel open state + toggle (shared workspace preference). */
  filesPanelOpen: boolean;
  onToggleFilesPanel: () => void;
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
 * The per-session chat header strip: the file-panel toggle, the usage meter, the
 * config controls, and the narrow-viewport "new session" affordance. Purely
 * presentational — extracted from `ChatSessionTab` so the tab body stays focused
 * on the transcript/composer flow and this reusable toolbar can be reasoned
 * about (and restyled) on its own. Opening the terminal is NOT here — it is a
 * canvas-tab affordance owned by `CanvasRegion`, not a toolbar toggle.
 */
export function ChatSessionToolbar({
  hasWorkspaceRoot,
  filesPanelOpen,
  onToggleFilesPanel,
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
        // at sm+); opens a fresh empty tab. Only shown below sm, where the Files
        // toggle is hidden — so it stays right-most in the narrow layout.
        <Button type="button" variant="outline" palette="neutral" size="sm" className="sm:hidden" onClick={onNewSession}>
          {t('acp_chat.new_session')}
        </Button>
      )}
      {hasWorkspaceRoot && (
        // Right-most toolbar element (wide layout only — hidden below sm).
        <Button
          type="button"
          variant={filesPanelOpen ? 'primary' : 'outline'}
          palette="neutral"
          size="sm"
          className="hidden sm:inline-flex"
          aria-pressed={filesPanelOpen}
          aria-label={t('workspace.toggle_label')}
          onClick={onToggleFilesPanel}>
          {filesPanelOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeft className="h-4 w-4" />}
          <span className="ml-1.5">{t('workspace.show_files')}</span>
        </Button>
      )}
    </div>
  );
}

import { Button } from '@contenox/ui';
import { PanelLeft, PanelLeftClose, Terminal } from 'lucide-react';
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
  /** Whether a workspace root exists — gates the mobile files toggle (no root, no files). */
  showFilesToggle: boolean;
  /** Whether the file explorer is open (drives the mobile files toggle's pressed state). */
  filesOpen: boolean;
  /** Opens/closes the file explorer (a drawer on mobile). */
  onToggleFiles: () => void;
  /** Whether the sidecar's terminal tab is open (drives the mobile sidecar toggle's pressed state). */
  sidecarOpen: boolean;
  /** Opens the sidecar (its terminal tab) / closes it — the mobile counterpart of the CanvasRegion rail. */
  onToggleSidecar: () => void;
}

/**
 * The per-session chat header strip: the usage meter, the config controls, the
 * narrow-viewport "new session" affordance, and — ON MOBILE ONLY — the panel
 * toggles for the two secondary surfaces.
 *
 * On desktop the surfaces open from edge rails (the files rail in
 * `ChatSessionTab`, the sidecar rail in `CanvasRegion`), each anchored on the
 * side its surface opens. Those rails are `hidden sm:flex`, so on a phone they
 * would strand both surfaces unreachable; here they reappear as header toggle
 * icons (`sm:hidden`) — the "one pane at a time + a header switcher" shape every
 * chat/IDE uses on narrow screens. The sidecar toggle stays visible during the
 * canvas's full-width takeover, so it also serves as the way back to the chat.
 */
export function ChatSessionToolbar({
  usage,
  configOptions,
  onConfigChange,
  showNewSession,
  onNewSession,
  showFilesToggle,
  filesOpen,
  onToggleFiles,
  sidecarOpen,
  onToggleSidecar,
}: ChatSessionToolbarProps) {
  const { t } = useTranslation();
  const pressedClass = 'bg-surface-200 text-text dark:bg-dark-surface-300 dark:text-dark-text';

  return (
    <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-wrap items-center justify-end gap-3 border-b px-3 py-2 sm:px-4">
      {/* Mobile-only surface toggles, pushed to the leading edge. */}
      <div className="mr-auto flex items-center gap-1 sm:hidden">
        {showFilesToggle && (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-pressed={filesOpen}
            aria-label={t('workspace.toggle_label')}
            title={t('workspace.show_files')}
            className={filesOpen ? pressedClass : undefined}
            onClick={onToggleFiles}>
            {filesOpen ? <PanelLeftClose className="h-4 w-4" /> : <PanelLeft className="h-4 w-4" />}
          </Button>
        )}
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-pressed={sidecarOpen}
          aria-label={sidecarOpen ? t('terminal.close_in_canvas') : t('terminal.open_in_canvas')}
          title={sidecarOpen ? t('terminal.close_in_canvas') : t('terminal.open_in_canvas')}
          className={sidecarOpen ? pressedClass : undefined}
          onClick={onToggleSidecar}>
          <Terminal className="h-4 w-4" />
        </Button>
      </div>
      <UsageMeter usage={usage} />
      <ConfigOptionControls configOptions={configOptions} onChange={onConfigChange} />
      {showNewSession && (
        // Narrow-viewport "new session" affordance (the sidebar's is canonical
        // at sm+); opens a fresh empty tab.
        <Button
          type="button"
          variant="outline"
          palette="neutral"
          size="sm"
          className="sm:hidden"
          onClick={onNewSession}>
          {t('acp_chat.new_session')}
        </Button>
      )}
    </div>
  );
}

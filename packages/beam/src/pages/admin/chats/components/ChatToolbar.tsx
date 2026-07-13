import {
  Button,
  InlineNotice,
  ProgressBar,
  Select,
  Span,
  Spinner,
  Toolbar,
  ToolbarActions,
  ToolbarItem,
  ToolbarSection,
  Tooltip,
} from '@contenox/ui';
import { Pencil, TerminalSquare } from 'lucide-react';
import { t } from 'i18next';
import type { ChatModeId } from '../../../../lib/types';
import { cn } from '../../../../lib/utils';

function contextUsagePalette(pct: number): 'primary' | 'warning' | 'error' {
  if (pct > 90) return 'error';
  if (pct > 70) return 'warning';
  return 'primary';
}

function contextUsageTextClass(pct: number): string | undefined {
  if (pct > 90) return 'text-error dark:text-dark-error';
  if (pct > 70) return 'text-warning dark:text-dark-warning';
  return undefined;
}

function ContextUsageMeter({ used, size }: { used: number; size: number }) {
  const safeUsed = Math.max(0, used);
  const pct = Math.round((safeUsed / size) * 100);
  const palette = contextUsagePalette(pct);
  const textClass = contextUsageTextClass(pct);
  const usedLabel = safeUsed > 0 ? `${safeUsed.toLocaleString()}/` : '';
  const title =
    safeUsed > 0
      ? `Context: ${safeUsed.toLocaleString()} / ${size.toLocaleString()} tokens (${pct}%)`
      : `Context window: ${size.toLocaleString()} tokens`;

  return (
    <div
      className="ml-3 flex shrink-0 items-center gap-2 text-xs font-medium tabular-nums"
      title={title}>
      <Span variant={textClass ? undefined : 'muted'} className={cn(textClass, 'tabular-nums')}>
        {usedLabel}
        {size.toLocaleString()}
      </Span>
      <ProgressBar
        value={Math.min(100, Math.max(0, pct))}
        palette={palette}
        className="h-2 w-24 bg-surface-200 shadow-inner dark:bg-dark-surface-300"
      />
      <Span variant={textClass ? undefined : 'muted'} className={cn(textClass, 'tabular-nums')}>
        {pct}%
      </Span>
    </div>
  );
}

interface ChatToolbarProps {
  chainOptions: { value: string; label: string }[];
  selectedChainId: string;
  onChainChange: (id: string) => void;
  chainsLoading: boolean;

  modeOptions: { value: ChatModeId; label: string }[];
  selectedMode: ChatModeId;
  onModeChange: (mode: ChatModeId) => void;
  isProcessing: boolean;
  policyNames: string[];
  activePolicyName: string;
  onPolicyChange: (name: string) => void;
  policyChangePending: boolean;
  policyChangeError: string | null;
  statsLabel: string;
  /** Conversation context usage: tokens used vs the session's effective token limit (chain token_limit or override).
   * Size is the budget the engine uses for shifting this session (clamped to model cap if known and >0).
   * This is the value users switch (via chain or future per-session control).
   */
  contextUsed?: number;
  contextSize?: number;
  /** When provided, shows a button to open the selected chain in the editor.
   * Omitted while the chain editor route is unwired. */
  onEditChain?: () => void;
  terminalAvailable?: boolean;
  workspacePanelOpen?: boolean;
  onWorkspaceToggle?: () => void;
  onOpenMobileWorkspace?: () => void;
  isLg?: boolean;
}

export function ChatToolbar({
  chainOptions,
  selectedChainId,
  onChainChange,
  chainsLoading,

  modeOptions,
  selectedMode,
  onModeChange,
  isProcessing,
  policyNames,
  activePolicyName,
  onPolicyChange,
  policyChangePending,
  policyChangeError,
  statsLabel,
  contextUsed,
  contextSize,
  onEditChain,
  terminalAvailable = false,
  workspacePanelOpen = false,
  onWorkspaceToggle,
  onOpenMobileWorkspace,
  isLg = true,
}: ChatToolbarProps) {


  return (
    <Toolbar>
      <ToolbarSection>
        <ToolbarItem label={t('chat.task_chain')} tooltip={t('chat.chain_tooltip')}>
          <Select
            options={chainOptions}
            value={selectedChainId}
            onChange={e => onChainChange(e.target.value)}
            className="min-w-[10rem] max-w-full flex-1 sm:max-w-md"
            disabled={chainsLoading}
          />
          {chainsLoading && <Spinner size="sm" />}

          {onEditChain && (
            <Tooltip
              content={
                selectedChainId
                  ? t('chat.edit_chain', 'Open this chain in the editor')
                  : t('chat.edit_chain_disabled', 'Select a chain to edit')
              }
              position="top"
            >
              <Button
                type="button"
                variant="ghost"
                size="xs"
                disabled={!selectedChainId.trim()}
                onClick={onEditChain}
              >
                <Pencil className="h-3.5 w-3.5" />
              </Button>
            </Tooltip>
          )}
        </ToolbarItem>
        <ToolbarItem label={t('chat.mode')} tooltip={t('chat.mode_tooltip')}>
          <Select
            options={modeOptions}
            value={selectedMode}
            onChange={e => onModeChange(e.target.value as ChatModeId)}
            className="min-w-[7rem] max-w-[12rem] shrink-0"
            disabled={isProcessing}
          />
        </ToolbarItem>
        {policyNames.length > 0 && (
          <ToolbarItem
            label={t('chat.hitl_policy', 'Policy')}
            tooltip={t('chat.hitl_policy_tooltip', 'HITL policy applied to this session')}
          >
            <Select
              options={[
                { value: '', label: t('chat.hitl_policy_none', 'None') },
                ...policyNames.map(n => ({ value: n, label: n })),
              ]}
              value={activePolicyName}
              onChange={e => onPolicyChange(e.target.value)}
              className="min-w-[8rem] max-w-[14rem] shrink-0"
              disabled={policyChangePending}
            />
            {policyChangeError && (
              <InlineNotice variant="error" className="text-xs">
                {policyChangeError}
              </InlineNotice>
            )}
          </ToolbarItem>
        )}
      </ToolbarSection>
      <ToolbarActions>
        <Span variant="muted" className="shrink-0 text-xs" title={statsLabel}>
          {statsLabel}
        </Span>
        {contextSize && contextSize > 0 ? (
          <ContextUsageMeter used={contextUsed ?? 0} size={contextSize} />
        ) : null}
        {terminalAvailable && onWorkspaceToggle ? (
          <Tooltip content={t('chat.workspace_toggle_tooltip')}>
            <Button
              type="button"
              variant={workspacePanelOpen ? 'secondary' : 'outline'}
              size="sm"
              className="shrink-0"
              onClick={onWorkspaceToggle}
              aria-pressed={workspacePanelOpen}
              aria-label={t('chat.workspace_toggle_aria')}>
              <TerminalSquare className="h-4 w-4" />
            </Button>
          </Tooltip>
        ) : null}
        {terminalAvailable && workspacePanelOpen && !isLg && onOpenMobileWorkspace ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="lg:hidden"
            onClick={onOpenMobileWorkspace}>
            {t('chat.workspace_open_mobile')}
          </Button>
        ) : null}
      </ToolbarActions>
    </Toolbar>
  );
}

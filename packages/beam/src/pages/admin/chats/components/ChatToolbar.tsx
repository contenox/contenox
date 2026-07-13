import {
  Button,
  InlineNotice,
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
import { ContextUsageMeter } from '../../../../components/ContextUsageMeter';

interface ChatToolbarProps {
  chainOptions: { value: string; label: string }[];
  selectedChainId: string;
  onChainChange: (id: string) => void;
  chainsLoading: boolean;

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

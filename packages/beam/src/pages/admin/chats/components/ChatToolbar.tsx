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
import { Pencil } from 'lucide-react';
import { t } from 'i18next';
import type { ChatModeId } from '../../../../lib/types';

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
  onEditChain: () => void;
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
  onEditChain,
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
        <Span
          className="text-text-muted dark:text-dark-text-muted shrink-0 text-xs"
          title={statsLabel}>
          {statsLabel}
        </Span>
      </ToolbarActions>
    </Toolbar>
  );
}

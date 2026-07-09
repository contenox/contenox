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
  /** Conversation context usage: tokens used vs the session's effective token limit (chain token_limit or override).
   * Size is the budget the engine uses for shifting this session (clamped to model cap if known and >0).
   * This is the value users switch (via chain or future per-session control).
   */
  contextUsed?: number;
  contextSize?: number;
  /** When provided, shows a button to open the selected chain in the editor.
   * Omitted while the chain editor route is unwired. */
  onEditChain?: () => void;
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
        <Span
          className="text-text-muted dark:text-dark-text-muted shrink-0 text-xs"
          title={statsLabel}>
          {statsLabel}
        </Span>
        {contextSize && contextSize > 0 ? (() => {
          const used = Math.max(0, contextUsed || 0);
          const size = contextSize;
          const pct = Math.round((used / size) * 100);
          const cls = pct > 90 ? 'text-red-500' : pct > 70 ? 'text-yellow-500' : 'text-text-muted dark:text-dark-text-muted';
          const barColor = pct > 90 ? 'bg-red-500' : pct > 70 ? 'bg-yellow-500' : 'bg-gradient-to-r from-indigo-500 to-violet-500';
          const usedLabel = used > 0 ? `${used.toLocaleString()}/` : '';
          const title = used > 0
            ? `Context: ${used.toLocaleString()} / ${size.toLocaleString()} tokens (${pct}%)`
            : `Context window: ${size.toLocaleString()} tokens`;
          return (
            <div className="ml-3 flex shrink-0 items-center gap-2 text-xs font-medium tabular-nums" title={title}>
              <span className={cls}>{usedLabel}{size.toLocaleString()}</span>
              <div className="w-24 h-2 rounded-full bg-surface-200 dark:bg-dark-surface-300 overflow-hidden shadow-inner" aria-hidden>
                <div className={`h-full transition-all duration-500 ease-out ${barColor}`} style={{ width: `${Math.min(100, Math.max(0, pct))}%` }} />
              </div>
              <span className={cls}>{pct}%</span>
            </div>
          );
        })() : null}
      </ToolbarActions>
    </Toolbar>
  );
}

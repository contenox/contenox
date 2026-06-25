import { Badge, Button, InlineNotice, P, Page, Panel, Select, Span, Spinner } from '@contenox/ui';
import { GitBranch, MessageSquarePlus, Settings } from 'lucide-react';
import { FormEvent, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useListChains } from '../../../hooks/useChains';
import { useCreateChat } from '../../../hooks/useChats';
import { useSetupStatus } from '../../../hooks/useSetupStatus';
import { ArtifactRegistryProvider } from '../../../lib/artifacts';
import { SlashCommandRegistryProvider } from '../../../lib/slashCommands';
import { ChatSession } from '../../../lib/types';
import { MessageInputForm } from './components/MessageInputForm';

const DEFAULT_CHAIN_PATH = 'default-chain.json';

function formatChainLabel(path: string): string {
  return path.replace(/\.json$/i, '');
}

/**
 * Same provider boundary as [ChatPage]: MessageInputForm uses useSlashCommandRegistry
 * and useArtifactRegistry and must render inside both providers.
 */
export default function ChatLandingPage() {
  return (
    <ArtifactRegistryProvider>
      <SlashCommandRegistryProvider>
        <ChatLandingPageImpl />
      </SlashCommandRegistryProvider>
    </ArtifactRegistryProvider>
  );
}

function ChatLandingPageImpl() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const [message, setMessage] = useState('');
  const [selectedChainId, setSelectedChainId] = useState('');
  const createChat = useCreateChat();

  const { data: setupStatus } = useSetupStatus(true);
  const { data: chainPaths = [], isLoading: chainsLoading, error: chainsError } = useListChains();

  const sortedChainPaths = useMemo(
    () =>
      [...chainPaths].sort((a, b) => {
        if (a === DEFAULT_CHAIN_PATH) return -1;
        if (b === DEFAULT_CHAIN_PATH) return 1;
        return a.localeCompare(b);
      }),
    [chainPaths],
  );

  useEffect(() => {
    if (selectedChainId) return;
    const defaultChain = sortedChainPaths[0];
    if (defaultChain) setSelectedChainId(defaultChain);
  }, [sortedChainPaths, selectedChainId]);

  const chainOptions = useMemo(
    () => [
      { value: '', label: t('chat.no_chain') },
      ...sortedChainPaths.map(p => ({ value: p, label: formatChainLabel(p) })),
    ],
    [sortedChainPaths, t],
  );
  const preferredChains = useMemo(() => sortedChainPaths.slice(0, 6), [sortedChainPaths]);
  const selectedChainLabel = selectedChainId ? formatChainLabel(selectedChainId) : '';

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    const trimmed = message.trim();
    if (!trimmed || !selectedChainId) return;
    createChat.mutate(
      {},
      {
        onSuccess: (data: Partial<ChatSession>) => {
          if (data?.id) {
            navigate(`/chat/${data.id}`, {
              replace: true,
              state: {
                beamInitialMessage: trimmed,
                beamInitialChainId: selectedChainId,
              },
            });
          }
        },
      },
    );
  };

  const canSend = !!selectedChainId && !!message.trim() && !createChat.isPending;

  return (
    <Page bodyScroll="auto">
      <div className="p-6">
        <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
          <div className="flex flex-col gap-3 border-b pb-5 lg:flex-row lg:items-end lg:justify-between">
            <div className="space-y-1">
              <h1 className="text-2xl font-semibold">{t('chat.landing_title')}</h1>
              <P variant="muted" className="max-w-2xl text-sm">
                {t('chat.landing_description')}
              </P>
            </div>
            {setupStatus?.defaultModel ? (
              <Badge variant="outline" size="sm" className="w-fit">
                {[setupStatus.defaultModel, setupStatus.defaultProvider]
                  .filter(Boolean)
                  .join(' · ')}
              </Badge>
            ) : null}
          </div>

          {setupStatus && !setupStatus.defaultModel ? (
            <InlineNotice variant="warning">
              {t('chat.landing_no_model', 'No default model set. Run contenox init to configure.')}
            </InlineNotice>
          ) : null}

          <div className="grid gap-4 lg:grid-cols-[minmax(0,20rem)_minmax(0,1fr)]">
            <Panel variant="surface" className="space-y-4">
              <div className="flex items-center justify-between gap-3">
                <div>
                  <Span className="block text-sm font-semibold">{t('chat.task_chain')}</Span>
                  <Span variant="muted" className="text-xs">
                    {selectedChainLabel || t('chat.no_chain')}
                  </Span>
                </div>
                {chainsLoading ? <Spinner size="sm" /> : <GitBranch className="h-4 w-4" />}
              </div>

              {chainsError ? (
                <InlineNotice variant="error">{chainsError.message}</InlineNotice>
              ) : sortedChainPaths.length === 0 && !chainsLoading ? (
                <div className="border-surface-300 dark:border-dark-surface-600 rounded-md border border-dashed p-4">
                  <Span className="block text-sm font-medium">
                    {t('chat.landing_no_chains_title')}
                  </Span>
                  <P variant="muted" className="mt-1 text-xs">
                    {t('chat.landing_no_chains_desc')}
                  </P>
                </div>
              ) : (
                <>
                  <Select
                    options={chainOptions}
                    value={selectedChainId}
                    onChange={e => setSelectedChainId(e.target.value)}
                    className="w-full"
                    disabled={chainsLoading}
                  />
                  {preferredChains.length > 0 && (
                    <div className="flex flex-wrap gap-2">
                      {preferredChains.map(path => (
                        <button
                          key={path}
                          type="button"
                          onClick={() => setSelectedChainId(path)}
                          title={path}
                          className={[
                            'rounded-md border px-2.5 py-1.5 text-xs transition-colors',
                            selectedChainId === path
                              ? 'border-primary-500 bg-surface-100 text-text ring-primary-500/30 dark:border-dark-primary-500 dark:bg-dark-surface-300 dark:text-dark-text dark:ring-dark-primary-500/30 ring-1'
                              : 'border-surface-300 text-text-muted hover:border-surface-500 hover:bg-surface-100 hover:text-text dark:border-dark-surface-600 dark:text-dark-text-muted dark:hover:border-dark-surface-500 dark:hover:bg-dark-surface-300 dark:hover:text-dark-text',
                          ].join(' ')}>
                          {formatChainLabel(path)}
                        </button>
                      ))}
                    </div>
                  )}
                </>
              )}

              <Button
                type="button"
                variant="secondary"
                size="sm"
                className="w-full gap-2"
                onClick={() => navigate('/chains')}>
                <Settings className="h-4 w-4" />
                {t('chat.landing_manage_chains')}
              </Button>
            </Panel>

            <Panel variant="surface" className="space-y-4">
              <div className="flex items-center gap-2">
                <MessageSquarePlus className="h-4 w-4" />
                <Span className="text-sm font-semibold">{t('chat.landing_composer_title')}</Span>
              </div>
              <MessageInputForm
                value={message}
                onChange={setMessage}
                onSubmit={handleSubmit}
                isPending={createChat.isPending}
                placeholder={t('chat.landing_input_placeholder')}
                title=""
                variant="workbench"
                buttonLabel={t('chat.run_button')}
                canSubmit={canSend}
              />
              {!selectedChainId && sortedChainPaths.length > 0 ? (
                <P variant="muted" className="text-xs">
                  {t('chat.landing_select_chain_hint')}
                </P>
              ) : null}
              {createChat.isError && (
                <P className="text-error text-sm">
                  {createChat.error?.message ?? t('chat.error_create_chat')}
                </P>
              )}
            </Panel>
          </div>
        </div>
      </div>
    </Page>
  );
}

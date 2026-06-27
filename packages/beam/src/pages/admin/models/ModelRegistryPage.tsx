import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  FormField,
  GridLayout,
  Input,
  LoadingState,
  Page,
  ResourceCard,
  Section,
  Span,
} from '@contenox/ui';
import { Search } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  useCreateModelRegistryEntry,
  useDeleteModelRegistryEntry,
  useDownloadModel,
  useModelRegistry,
} from '../../../hooks/useModelRegistry';
import { ModelDescriptor } from '../../../lib/types';

function RegistryEntryRow({
  entry,
  onDelete,
  onDownload,
}: {
  entry: ModelDescriptor;
  onDelete: (id: string) => void;
  onDownload: (name: string) => Promise<void>;
}) {
  const { t } = useTranslation();
  const [isDownloading, setIsDownloading] = useState(false);
  const sizeMB = entry.sizeBytes ? Math.round(entry.sizeBytes / 1024 / 1024) : 0;

  const handleDownload = async () => {
    setIsDownloading(true);
    try {
      await onDownload(entry.name);
    } finally {
      setIsDownloading(false);
    }
  };

  return (
    <ResourceCard
      title={entry.name}
      subtitle={
        <>
          <Span variant="muted" className="block truncate text-xs">
            {entry.sourceUrl}
          </Span>
          {sizeMB > 0 && (
            <Span variant="muted" className="text-xs">
              {sizeMB} MB
            </Span>
          )}
        </>
      }
      status="default"
      badge={
        entry.curated ? (
          <Badge variant="outline" size="sm">
            {t('model_registry.curated')}
          </Badge>
        ) : undefined
      }
      actions={!entry.curated && entry.id ? { delete: () => onDelete(entry.id!) } : undefined}
      isLoading={isDownloading}>
      <Button variant="ghost" size="sm" onClick={handleDownload} disabled={isDownloading}>
        {isDownloading ? t('model_registry.downloading') : t('model_registry.download')}
      </Button>
    </ResourceCard>
  );
}

export default function ModelRegistryPage() {
  const { t } = useTranslation();
  const { data: entries, isLoading, error, refetch } = useModelRegistry();
  const createMutation = useCreateModelRegistryEntry();
  const deleteMutation = useDeleteModelRegistryEntry();
  const downloadMutation = useDownloadModel();

  const [name, setName] = useState('');
  const [sourceUrl, setSourceUrl] = useState('');
  const [search, setSearch] = useState('');

  const resetForm = () => {
    setName('');
    setSourceUrl('');
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate({ name, sourceUrl, sizeBytes: 0 }, { onSuccess: resetForm });
  };

  const handleDelete = (id: string) => {
    deleteMutation.mutate(id);
  };

  const handleDownload = async (modelName: string) => {
    await downloadMutation.mutateAsync(modelName);
  };

  if (isLoading) {
    return <LoadingState message={t('model_registry.loading')} />;
  }

  if (error) {
    return <ErrorState error={error} onRetry={refetch} title={t('model_registry.list_error')} />;
  }

  const sorted = [...(entries ?? [])].sort((a, b) => {
    if (a.curated !== b.curated) return a.curated ? -1 : 1;
    return a.name.localeCompare(b.name);
  });
  const normalizedSearch = search.trim().toLowerCase();
  const filtered = normalizedSearch
    ? sorted.filter(entry =>
        `${entry.name} ${entry.sourceUrl}`.toLowerCase().includes(normalizedSearch),
      )
    : sorted;

  return (
    <Page bodyScroll="auto" className="h-full">
      <GridLayout variant="body" columns={2} responsive={{ base: 1, lg: 2 }} className="gap-6 p-4">
        <Section title={t('model_registry.list_title')} className="order-2 lg:order-1">
          <div className="space-y-2">
            <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
              <div className="relative max-w-md flex-1">
                <Search className="text-text-muted dark:text-dark-text-muted absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
                <Input
                  value={search}
                  onChange={e => setSearch(e.target.value)}
                  placeholder={t(
                    'model_registry.search_placeholder',
                    'Search models or source URLs',
                  )}
                  className="pl-10"
                />
              </div>
              <Span variant="muted" className="text-xs">
                {t('model_registry.results_count', {
                  count: filtered.length,
                  total: sorted.length,
                  defaultValue: `${filtered.length}/${sorted.length} models`,
                })}
              </Span>
            </div>

            {sorted.length === 0 ? (
              <EmptyState
                title={t('model_registry.empty_title')}
                description={t('model_registry.empty_description')}
              />
            ) : filtered.length === 0 ? (
              <EmptyState
                title={t('model_registry.no_search_results', 'No models match this search')}
                description={t(
                  'model_registry.no_search_results_description',
                  'Try a model family, backend format, or source host.',
                )}
              />
            ) : (
              filtered.map(entry => (
                <RegistryEntryRow
                  key={entry.name}
                  entry={entry}
                  onDelete={handleDelete}
                  onDownload={handleDownload}
                />
              ))
            )}
          </div>
        </Section>

        <Section
          title={t('model_registry.add_title')}
          className="order-1 lg:sticky lg:top-4 lg:order-2 lg:self-start">
          <form onSubmit={handleSubmit} className="space-y-4">
            <FormField label={t('model_registry.form_name')} required>
              <Input
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder="my-model"
                required
              />
            </FormField>
            <FormField label={t('model_registry.form_url')} required>
              <Input
                value={sourceUrl}
                onChange={e => setSourceUrl(e.target.value)}
                placeholder="https://huggingface.co/org/model.gguf"
                required
              />
            </FormField>
            <Button type="submit" disabled={createMutation.isPending}>
              {t('model_registry.form_add_action')}
            </Button>
          </form>
        </Section>
      </GridLayout>
    </Page>
  );
}

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
          <Span variant="muted" className="block truncate text-xs">{entry.sourceUrl}</Span>
          {sizeMB > 0 && <Span variant="muted" className="text-xs">{sizeMB} MB</Span>}
        </>
      }
      status="default"
      badge={entry.curated ? <Badge variant="info" size="sm">{t('model_registry.curated')}</Badge> : undefined}
      actions={!entry.curated && entry.id ? { delete: () => onDelete(entry.id!) } : undefined}
      isLoading={isDownloading}
    >
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

  const handleDownload = (modelName: string) => downloadMutation.mutateAsync(modelName);

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

  return (
    <Page bodyScroll="auto" className="h-full">
      <GridLayout variant="body" columns={2} responsive={{ base: 1, lg: 2 }} className="gap-6 p-4">
        <Section title={t('model_registry.list_title')}>
          <div className="space-y-2">
            {sorted.length === 0 ? (
              <EmptyState
                title={t('model_registry.empty_title')}
                description={t('model_registry.empty_description')}
              />
            ) : (
              sorted.map(entry => (
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

        <Section title={t('model_registry.add_title')}>
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

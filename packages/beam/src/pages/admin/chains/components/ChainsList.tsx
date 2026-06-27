import {
  Button,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  Panel,
  Span,
  Spinner,
  Table,
  TableCell,
  TableRow,
} from '@contenox/ui';
import { Search, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useDeleteChain } from '../../../../hooks/useChains';

interface ChainsListProps {
  paths: string[];
  isLoading?: boolean;
  error: Error | null;
  onSelectPath?: (vfsPath: string) => void;
  onCreate?: () => void;
}

export default function ChainsList({
  paths,
  isLoading = false,
  error,
  onSelectPath,
  onCreate,
}: ChainsListProps) {
  const { t } = useTranslation();
  const deleteChain = useDeleteChain();
  const [deletingPath, setDeletingPath] = useState<string | null>(null);
  const [search, setSearch] = useState('');

  const filteredPaths = useMemo(() => {
    if (!search.trim()) return paths;
    const q = search.toLowerCase();
    return paths.filter(p => p.toLowerCase().includes(q));
  }, [paths, search]);

  const handleDelete = async (vfsPath: string) => {
    if (
      !window.confirm(t('chains.confirm_delete', 'Delete this chain file? This cannot be undone.'))
    )
      return;
    setDeletingPath(vfsPath);
    try {
      await deleteChain.mutateAsync(vfsPath);
    } finally {
      setDeletingPath(null);
    }
  };

  if (isLoading) {
    return <LoadingState />;
  }

  if (error) {
    return <ErrorState title={t('chains.list_error')} error={error} />;
  }

  if (!filteredPaths.length) {
    return (
      <div className="flex w-full flex-1 flex-col items-center justify-center p-12">
        <EmptyState
          title={
            paths.length === 0
              ? t('chains.list_empty_title')
              : t('chains.no_search_results', 'No chains match this search')
          }
          description={
            paths.length === 0
              ? t('chains.list_empty_message')
              : t(
                  'chains.no_search_results_description',
                  'Try another file path or clear the search.',
                )
          }
          orientation="horizontal"
          iconSize="lg"
        />
        {onCreate && paths.length === 0 && (
          <Button variant="primary" onClick={onCreate}>
            {t('chains.create_first_chain')}
          </Button>
        )}
      </div>
    );
  }

  return (
    <div className="flex h-full min-h-0 w-full min-w-0 flex-1 flex-col gap-4 p-4 sm:p-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div className="relative w-full max-w-md flex-1">
          <Search className="text-text-muted dark:text-dark-text-muted absolute top-1/2 left-3 h-4 w-4 -translate-y-1/2" />
          <Input
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder={t('chains.search_placeholder', 'Search by file path...')}
            className="pl-10"
          />
        </div>
        {onCreate && (
          <Button onClick={onCreate} variant="primary" className="shrink-0">
            {t('chains.create_new')}
          </Button>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-auto md:hidden">
        <div className="space-y-3">
          {filteredPaths.map(vfsPath => {
            const isDeleting = deletingPath === vfsPath;
            return (
              <Panel key={vfsPath} variant="surface" className="space-y-3 p-4">
                <Span className="text-text dark:text-dark-text block font-mono text-sm break-all">
                  {vfsPath}
                </Span>
                <div className="flex items-center justify-between gap-2">
                  {onSelectPath && (
                    <Button
                      variant="outline"
                      palette="neutral"
                      size="sm"
                      onClick={() => onSelectPath(vfsPath)}
                      disabled={isDeleting}>
                      {t('common.edit')}
                    </Button>
                  )}
                  <Button
                    variant="danger"
                    size="sm"
                    className="ml-auto gap-1.5"
                    onClick={() => handleDelete(vfsPath)}
                    disabled={isDeleting}>
                    {isDeleting ? (
                      <Spinner size="sm" />
                    ) : (
                      <>
                        <Trash2 className="h-4 w-4" aria-hidden />
                        {t('common.delete')}
                      </>
                    )}
                  </Button>
                </div>
              </Panel>
            );
          })}
        </div>
      </div>

      <div className="hidden min-h-0 flex-1 overflow-auto md:block">
        <Table columns={[t('common.path'), t('common.actions')]} className="w-full table-fixed">
          {filteredPaths.map(vfsPath => {
            const isDeleting = deletingPath === vfsPath;

            return (
              <TableRow key={vfsPath} className="hover:bg-secondary/50 transition-colors">
                <TableCell className="font-mono text-sm break-all">{vfsPath}</TableCell>

                <TableCell>
                  <div className="flex flex-wrap items-center gap-2">
                    {onSelectPath && (
                      <Button
                        variant="outline"
                        palette="neutral"
                        size="sm"
                        onClick={() => onSelectPath(vfsPath)}
                        disabled={isDeleting}>
                        {t('common.edit')}
                      </Button>
                    )}

                    <Button
                      variant="danger"
                      size="sm"
                      className="gap-1.5"
                      onClick={() => handleDelete(vfsPath)}
                      disabled={isDeleting}>
                      {isDeleting ? (
                        <Spinner size="sm" />
                      ) : (
                        <>
                          <Trash2 className="h-4 w-4" aria-hidden />
                          {t('common.delete')}
                        </>
                      )}
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            );
          })}
        </Table>
      </div>

      <div className="text-text-muted dark:text-dark-text-muted flex shrink-0 flex-col gap-1 text-sm sm:flex-row sm:items-center sm:justify-between">
        <Span variant="muted">
          {t('chains.total_chains')}: {paths.length}
          {search && filteredPaths.length !== paths.length && (
            <Span variant="muted">
              {' '}
              ({t('chains.filtered')}: {filteredPaths.length})
            </Span>
          )}
        </Span>
        <Span variant="muted">{t('chains.default_chains_notice')}</Span>
      </div>
    </div>
  );
}

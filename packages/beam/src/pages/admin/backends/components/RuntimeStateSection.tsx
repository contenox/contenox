import {
  EmptyState,
  LoadingState,
  Panel,
  Section,
  Table,
  TableCell,
  TableRow,
} from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { BackendRuntimeState } from '../../../../lib/types';

type RuntimeStateSectionProps = {
  data: BackendRuntimeState[] | undefined;
  isLoading: boolean;
  isError: boolean;
  errorMessage?: string;
};

export default function RuntimeStateSection({
  data,
  isLoading,
  isError,
  errorMessage,
}: RuntimeStateSectionProps) {
  const { t } = useTranslation();

  return (
    <Section title={t('state.runtime_title')} description={t('state.runtime_intro')}>
      {isLoading && (
        <LoadingState />
      )}
      {isError && (
        <Panel variant="error">
          {errorMessage || t('state.runtime_error')}
        </Panel>
      )}
      {!isLoading && !isError && data && data.length === 0 && (
        <EmptyState
          title={t('state.runtime_empty_title')}
          description={t('state.runtime_empty_desc')}
          orientation="horizontal"
          iconSize="lg"
        />
      )}
      {!isLoading && !isError && data && data.length > 0 && (
        <Table
          columns={[
            t('state.col_backend'),
            t('state.col_type'),
            t('state.col_url'),
            t('state.col_error'),
            t('state.col_models'),
          ]}>
          {data.map(row => (
            <TableRow key={row.id}>
              <TableCell className="font-medium">{row.name}</TableCell>
              <TableCell>{row.backend?.type ?? '—'}</TableCell>
              <TableCell className="max-w-[220px] truncate font-mono text-xs">
                {row.backend?.baseUrl ?? '—'}
              </TableCell>
              <TableCell className="max-w-[300px]">
                {row.error?.trim() ? (
                  <div 
                    className="line-clamp-3 overflow-hidden rounded-md bg-error/10 px-2 py-1 text-xs font-mono text-error-600 dark:text-dark-error-800"
                    title={row.error}
                  >
                    {row.error}
                  </div>
                ) : (
                  <span className="text-sm text-text-muted dark:text-dark-text-muted">—</span>
                )}
              </TableCell>
              <TableCell>{row.pulledModels?.length ?? row.models?.length ?? 0}</TableCell>
            </TableRow>
          ))}
        </Table>
      )}
    </Section>
  );
}

import {
  Button,
  Form,
  FormField,
  GridLayout,
  Input,
  Panel,
  Section,
  Span,
  Spinner,
} from '@contenox/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearch } from '../../../hooks/useSearch';

export default function SearchPage() {
  const { t } = useTranslation();
  const [query, setQuery] = useState('');
  const [topk, setTopk] = useState<number | undefined>(10);
  const [radius, setRadius] = useState<number | undefined>(20);
  const [epsilon, setEpsilon] = useState<number | undefined>(0.1);
  const [searchParams, setSearchParams] = useState<{
    query: string;
    topk?: number;
    radius?: number;
    epsilon?: number;
  }>();

  const { data, isError, error, isPending } = useSearch(
    searchParams?.query || '',
    searchParams?.topk,
    searchParams?.radius,
    searchParams?.epsilon,
  );

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    setSearchParams({ query, topk, radius, epsilon });
  };

  return (
    <GridLayout variant="body">
      <Section>
        <Form
          onSubmit={handleSubmit}
          title={t('search.title')}
          actions={
            <Button type="submit" variant="primary">
              {t('search.search')}
            </Button>
          }>
          <FormField label={t('search.query')} required>
            <Input
              value={query}
              onChange={e => setQuery(e.target.value)}
              placeholder={t('search.query_placeholder')}
            />
          </FormField>

          <FormField label={t('search.topk')}>
            <Input
              type="parse_number"
              value={topk?.toString() ?? ''}
              onChange={e => setTopk(e.target.value ? parseInt(e.target.value) : undefined)}
              placeholder={t('search.topk_placeholder')}
            />
          </FormField>

          <FormField label={t('search.radius')}>
            <Input
              type="parse_number"
              step="0.01"
              value={radius?.toString() ?? ''}
              onChange={e => setRadius(e.target.value ? parseFloat(e.target.value) : undefined)}
              placeholder={t('search.radius_placeholder')}
            />
          </FormField>

          <FormField label={t('search.epsilon')}>
            <Input
              type="parse_number"
              step="0.01"
              value={epsilon?.toString() ?? ''}
              onChange={e => setEpsilon(e.target.value ? parseFloat(e.target.value) : undefined)}
              placeholder={t('search.epsilon_placeholder')}
            />
          </FormField>
        </Form>
      </Section>

      <Section>
        {searchParams?.query?.trim() && isPending && <Spinner size="lg" />}
        {!searchParams?.query?.trim() && <Panel>{t('search.search_invite')}</Panel>}
        {isError && (
          <Panel variant="error">
            {t('search.error')}: {error?.message}
          </Panel>
        )}

        {data && (
          <div className="space-y-4">
            <Span variant="sectionTitle">{t('search.results')}</Span>
            {data.results.length === 0 ? (
              <Panel variant="raised">{t('search.no_results')}</Panel>
            ) : (
              data.results.map(result => (
                <Panel key={result.id} className="flex items-center justify-between">
                  <Span>{result.id}</Span>
                  <Span>{result.resourceType}</Span>
                  <Span>{result.fileMeta?.path}</Span>
                  <Span variant="muted">{result.distance.toFixed(4)}</Span>
                </Panel>
              ))
            )}
          </div>
        )}
      </Section>
    </GridLayout>
  );
}

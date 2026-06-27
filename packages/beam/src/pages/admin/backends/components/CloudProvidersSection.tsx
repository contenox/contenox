import { GridLayout, Panel, Section } from '@contenox/ui';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../../lib/api';
import { CLOUD_PROVIDER_SETUPS } from '../../../../lib/providerCatalog';
import ProviderForm from './ProviderForm';

export default function CloudProvidersSection() {
  const { t } = useTranslation();
  const { data, error, isLoading } = useQuery({
    queryKey: ['providers', 'supported'],
    queryFn: api.getSupportedProviders,
    staleTime: 60_000,
  });
  const supportedProviders = data ? new Set(data.map(provider => provider.provider)) : null;
  const setups = supportedProviders
    ? CLOUD_PROVIDER_SETUPS.filter(setup => supportedProviders.has(setup.provider))
    : CLOUD_PROVIDER_SETUPS;

  if (isLoading) {
    return (
      <GridLayout variant="body">
        <Panel variant="body">{t('common.loading')}</Panel>
      </GridLayout>
    );
  }

  if (error) {
    return (
      <GridLayout variant="body">
        <Panel variant="error">{error.message}</Panel>
      </GridLayout>
    );
  }

  return (
    <GridLayout variant="body">
      {setups.map(setup => (
        <Section key={setup.provider} title={t(setup.titleKey as Parameters<typeof t>[0])}>
          <ProviderForm setup={setup} />
        </Section>
      ))}
    </GridLayout>
  );
}

import { Button, Form, FormField, Input, P, Panel, Textarea } from '@contenox/ui';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useConfigureProvider, useProviderStatus } from '../../../../hooks/useProviders';
import { getCloudProviderSetup, type CloudProviderSetup } from '../../../../lib/providerCatalog';
import type { CloudProviderType } from '../../../../lib/types';

type ProviderFormProps = {
  provider?: CloudProviderType;
  setup?: CloudProviderSetup;
};

export default function ProviderForm({ provider, setup: setupProp }: ProviderFormProps) {
  const { t } = useTranslation();
  const setup = setupProp ?? getCloudProviderSetup(provider ?? 'openai');
  const [secret, setSecret] = useState('');
  const [apiKeyEnv, setApiKeyEnv] = useState('');
  const [baseUrl, setBaseUrl] = useState(setup.defaultBaseUrl ?? '');
  const [formError, setFormError] = useState<string | null>(null);
  const [hydratedStatus, setHydratedStatus] = useState(false);
  const { data: status, isLoading, error } = useProviderStatus(setup.provider);
  const configureMutation = useConfigureProvider(setup.provider);

  useEffect(() => {
    if (!status || hydratedStatus) return;
    if (status.baseUrl) setBaseUrl(status.baseUrl);
    if (status.apiKeyEnv) setApiKeyEnv(status.apiKeyEnv);
    setHydratedStatus(true);
  }, [hydratedStatus, status]);

  const secretConfigured = status?.secretConfigured ?? false;
  const secretIsNeeded = setup.secretRequired && !secretConfigured;
  const hasSecretInput = secret.trim() !== '' || apiKeyEnv.trim() !== '';
  const baseUrlMissing = !!setup.baseUrlRequired && baseUrl.trim() === '';
  const isSubmitDisabled = configureMutation.isPending || baseUrlMissing || (secretIsNeeded && !hasSecretInput);
  const secretFieldIsMultiline =
    setup.secretKind === 'service-account-json' || setup.secretKind === 'aws-credentials-json';
  const supportsEnvSecret = setup.secretKind === 'api-key';

  const statusText = useMemo(() => {
    if (!status) return '';
    if (!status.configured) return t('cloud_providers.status_not_configured');
    if (status.secretSource === 'env' && status.secretPresent === false) {
      return t('cloud_providers.status_env_missing', { env: status.apiKeyEnv ?? '' });
    }
    if (status.baseUrl) {
      return t('cloud_providers.status_configured_with_url', { url: status.baseUrl });
    }
    return t('cloud_providers.status_configured');
  }, [status, t]);

  const handleSubmit = () => {
    const trimmedSecret = secret.trim();
    const trimmedEnv = apiKeyEnv.trim();
    if (trimmedSecret && trimmedEnv) {
      setFormError(t('cloud_providers.secret_source_error'));
      return;
    }
    setFormError(null);
    configureMutation.mutate(
      {
        apiKey: trimmedSecret || undefined,
        apiKeyEnv: trimmedEnv || undefined,
        baseUrl: baseUrl.trim() || undefined,
        upsert: true,
      },
      {
        onSuccess: () => {
          setSecret('');
        },
      },
    );
  };

  return (
    <Form
      onSubmit={handleSubmit}
      actions={
        <Button type="submit" variant="primary" disabled={isSubmitDisabled}>
          {configureMutation.isPending
            ? t('common.configuring')
            : t('cloud_providers.configure_button')}
        </Button>
      }>
      <P variant="muted" className="text-sm">
        {t(setup.descriptionKey as Parameters<typeof t>[0])}
      </P>

      {isLoading && <Panel variant="body">{t('common.loading')}</Panel>}
      {error && <Panel variant="error">{error.message}</Panel>}
      {formError && <Panel variant="error">{formError}</Panel>}
      {configureMutation.error && <Panel variant="error">{configureMutation.error.message}</Panel>}

      {setup.baseUrlLabelKey && (
        <FormField label={t(setup.baseUrlLabelKey as Parameters<typeof t>[0])} required={setup.baseUrlRequired}>
          <Input
            type="text"
            value={baseUrl}
            onChange={e => setBaseUrl(e.target.value)}
            placeholder={setup.baseUrlPlaceholder}
          />
        </FormField>
      )}

      {setup.secretKind !== 'none' && (
        <FormField
          label={t(setup.secretLabelKey as Parameters<typeof t>[0])}
          required={secretIsNeeded}>
          {secretFieldIsMultiline ? (
            <Textarea
              value={secret}
              onChange={e => setSecret(e.target.value)}
              placeholder={t(setup.secretPlaceholderKey as Parameters<typeof t>[0])}
              rows={6}
            />
          ) : (
            <Input
              type="password"
              value={secret}
              onChange={e => setSecret(e.target.value)}
              placeholder={t(setup.secretPlaceholderKey as Parameters<typeof t>[0])}
            />
          )}
        </FormField>
      )}

      {supportsEnvSecret && (
        <FormField label={t('cloud_providers.api_key_env')}>
          <Input
            type="text"
            value={apiKeyEnv}
            onChange={e => setApiKeyEnv(e.target.value)}
            placeholder={status?.recommendedApiKeyEnv || t('cloud_providers.api_key_env_placeholder')}
          />
        </FormField>
      )}

      <Panel variant="flat">{t('cloud_providers.auto_backend_note')}</Panel>

      {status && (
        <Panel variant="flat">{statusText}</Panel>
      )}
    </Form>
  );
}

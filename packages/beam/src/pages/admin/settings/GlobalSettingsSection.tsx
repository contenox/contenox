import { Button, FormField, H2, InlineNotice, Input, P, Panel, Select } from '@contenox/ui';
import { FormEvent, useContext, useEffect, useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCLIConfig } from '../../../hooks/useCLIConfig';
import { usePutCLIConfig } from '../../../hooks/usePutCLIConfig';
import { useSetupStatus } from '../../../hooks/useSetupStatus';
import { AuthContext } from '../../../lib/authContext';
import type { CLIConfigUpdateRequest } from '../../../lib/types';

const uniqueSorted = (values: string[]) =>
  Array.from(new Set(values.map(value => value.trim()).filter(Boolean))).sort((a, b) =>
    a.localeCompare(b),
  );

export function GlobalSettingsSection() {
  const { t } = useTranslation();
  const { user } = useContext(AuthContext);
  const { data } = useSetupStatus(!!user);
  // useSetupStatus only carries the model/provider subset relevant to
  // onboarding readiness checks; the alt-model/alt-provider defaults live in
  // the full CLI config snapshot instead (see hooks/useCLIConfig.ts).
  const { data: cliConfig } = useCLIConfig(!!user);
  const putConfig = usePutCLIConfig();
  const formId = useId();

  const [model, setModel] = useState('');
  const [provider, setProvider] = useState('');
  const [altModel, setAltModel] = useState('');
  const [altProvider, setAltProvider] = useState('');

  useEffect(() => {
    if (!data) return;
    setModel(data.defaultModel || '');
    setProvider(data.defaultProvider || '');
  }, [data]);

  useEffect(() => {
    if (!cliConfig) return;
    setAltModel(cliConfig.defaultAltModel || '');
    setAltProvider(cliConfig.defaultAltProvider || '');
  }, [cliConfig]);

  useEffect(() => {
    if (!putConfig.isSuccess) return;
    const timer = window.setTimeout(() => putConfig.reset(), 3000);
    return () => window.clearTimeout(timer);
  }, [putConfig.isSuccess, putConfig.reset]);

  const modelOptions = useMemo(() => {
    const values = uniqueSorted(
      (data?.backendChecks ?? []).flatMap(backend => backend.chatModels ?? []),
    );
    const current = model.trim();
    if (current && !values.includes(current)) values.unshift(current);
    return values.map(value => ({ value, label: value }));
  }, [data?.backendChecks, model]);

  const providerOptions = useMemo(() => {
    const values = uniqueSorted((data?.backendChecks ?? []).map(backend => backend.type));
    const current = provider.trim();
    if (current && !values.includes(current)) values.unshift(current);
    return values.map(value => ({ value, label: value }));
  }, [data?.backendChecks, provider]);

  const altModelOptions = useMemo(() => {
    const values = uniqueSorted(
      (data?.backendChecks ?? []).flatMap(backend => backend.chatModels ?? []),
    );
    const current = altModel.trim();
    if (current && !values.includes(current)) values.unshift(current);
    return [
      { value: '', label: t('settingsAdvanced.not_set') },
      ...values.map(value => ({ value, label: value })),
    ];
  }, [altModel, data?.backendChecks, t]);

  const altProviderOptions = useMemo(() => {
    const values = uniqueSorted((data?.backendChecks ?? []).map(backend => backend.type));
    const current = altProvider.trim();
    if (current && !values.includes(current)) values.unshift(current);
    return [
      { value: '', label: t('settingsAdvanced.not_set') },
      ...values.map(value => ({ value, label: value })),
    ];
  }, [altProvider, data?.backendChecks, t]);

  const selectedProviderChecks = useMemo(
    () => (data?.backendChecks ?? []).filter(backend => backend.type === provider),
    [data?.backendChecks, provider],
  );
  const selectedProviderHasModel = useMemo(() => {
    if (!model.trim() || !provider.trim() || selectedProviderChecks.length === 0) return true;
    return selectedProviderChecks.some(backend =>
      (backend.chatModels ?? []).includes(model.trim()),
    );
  }, [model, provider, selectedProviderChecks]);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    putConfig.reset();
    const body: CLIConfigUpdateRequest = {};
    if (model.trim()) body['default-model'] = model.trim();
    if (provider.trim()) body['default-provider'] = provider.trim();
    // Alt model/provider are genuinely optional — always send them (even
    // empty) so clearing the field in the UI actually clears the stored value.
    body['default-alt-model'] = altModel.trim();
    body['default-alt-provider'] = altProvider.trim();
    putConfig.mutate(body);
  };

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settings.global_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settings.global_section_description')}
          </P>
        </div>
        <form id={formId} onSubmit={onSubmit} className="grid max-w-xl gap-4">
          <InlineNotice variant="info" className="rounded-lg">
            {t('settingsAdvanced.restart_notice')}
          </InlineNotice>

          {!selectedProviderHasModel && (
            <InlineNotice variant="warning" className="rounded-lg">
              {t(
                'settings.model_provider_mismatch',
                'The selected provider does not currently report this model as chat-capable. Pick a model from the same provider or pull/register a compatible local model.',
              )}
            </InlineNotice>
          )}

          <FormField
            label={t('settings.default_model_label')}
            tooltip={t('settingsAdvanced.default_model_tooltip')}>
            {modelOptions.length > 0 ? (
              <Select
                name="default-model"
                className="w-full"
                value={model}
                onChange={e => setModel(e.target.value)}
                placeholder={t('setup.model_placeholder')}
                options={modelOptions}
              />
            ) : (
              <Input
                name="default-model"
                className="w-full"
                placeholder={t('setup.model_placeholder')}
                value={model}
                onChange={e => setModel(e.target.value)}
              />
            )}
          </FormField>

          <FormField
            label={t('settings.default_provider_label')}
            tooltip={t('settingsAdvanced.default_provider_tooltip')}>
            {providerOptions.length > 0 ? (
              <Select
                name="default-provider"
                className="w-full"
                value={provider}
                onChange={e => setProvider(e.target.value)}
                placeholder={t('setup.provider_placeholder')}
                options={providerOptions}
              />
            ) : (
              <Input
                name="default-provider"
                className="w-full"
                placeholder={t('setup.provider_placeholder')}
                value={provider}
                onChange={e => setProvider(e.target.value)}
              />
            )}
          </FormField>

          <div className="border-surface-200 dark:border-dark-surface-400 space-y-4 border-t pt-4">
            <div className="space-y-1">
              <P className="text-sm font-medium">{t('settingsAdvanced.routing_section_title')}</P>
              <P variant="muted" className="text-xs">
                {t('settingsAdvanced.routing_section_description')}
              </P>
            </div>

            <FormField
              label={t('settingsAdvanced.alt_model_label')}
              tooltip={t('settingsAdvanced.alt_model_tooltip')}>
              <Select
                name="default-alt-model"
                className="w-full"
                value={altModel}
                onChange={e => setAltModel(e.target.value)}
                options={altModelOptions}
              />
            </FormField>

            <FormField
              label={t('settingsAdvanced.alt_provider_label')}
              tooltip={t('settingsAdvanced.alt_provider_tooltip')}>
              <Select
                name="default-alt-provider"
                className="w-full"
                value={altProvider}
                onChange={e => setAltProvider(e.target.value)}
                options={altProviderOptions}
              />
            </FormField>
          </div>

          {putConfig.isError && <P className="text-error text-sm">{putConfig.error.message}</P>}
          {putConfig.isSuccess && <P className="text-text-muted text-sm">{t('settings.saved')}</P>}

          <div>
            <Button
              type="submit"
              form={formId}
              variant="primary"
              size="sm"
              disabled={putConfig.isPending}>
              {t('settings.save')}
            </Button>
          </div>
        </form>
      </div>
    </Panel>
  );
}

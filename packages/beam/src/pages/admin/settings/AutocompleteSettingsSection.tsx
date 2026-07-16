import { Button, FormField, H2, Input, P, Panel } from '@contenox/ui';
import { FormEvent, useContext, useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCLIConfig } from '../../../hooks/useCLIConfig';
import { usePutCLIConfig } from '../../../hooks/usePutCLIConfig';
import { AuthContext } from '../../../lib/authContext';
import type { CLIConfigUpdateRequest } from '../../../lib/types';

/**
 * VS Code ghost-text autocomplete uses its own model/provider, independent
 * from chat (runtime/vscodeagent/server.go reads these fresh per request via
 * clikv.ReadConfig — unlike the ACP chat defaults, there is no restart caveat
 * here).
 */
export function AutocompleteSettingsSection() {
  const { t } = useTranslation();
  const { user } = useContext(AuthContext);
  const { data } = useCLIConfig(!!user);
  const putConfig = usePutCLIConfig();
  const formId = useId();

  const [model, setModel] = useState('');
  const [provider, setProvider] = useState('');

  useEffect(() => {
    if (!data) return;
    setModel(data.defaultAutocompleteModel ?? '');
    setProvider(data.defaultAutocompleteProvider ?? '');
  }, [data]);

  useEffect(() => {
    if (!putConfig.isSuccess) return;
    const timer = window.setTimeout(() => putConfig.reset(), 3000);
    return () => window.clearTimeout(timer);
  }, [putConfig.isSuccess, putConfig.reset]);

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    putConfig.reset();
    const body: CLIConfigUpdateRequest = {
      'default-autocomplete-model': model.trim(),
      'default-autocomplete-provider': provider.trim(),
    };
    putConfig.mutate(body);
  };

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settingsAdvanced.autocomplete_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settingsAdvanced.autocomplete_section_description')}
          </P>
        </div>
        <form id={formId} onSubmit={onSubmit} className="grid max-w-xl gap-4">
          <FormField
            label={t('settingsAdvanced.autocomplete_model_label')}
            tooltip={t('settingsAdvanced.autocomplete_model_tooltip')}>
            <Input
              name="default-autocomplete-model"
              className="w-full"
              placeholder={t('setup.model_placeholder')}
              value={model}
              onChange={e => setModel(e.target.value)}
            />
          </FormField>

          <FormField
            label={t('settingsAdvanced.autocomplete_provider_label')}
            tooltip={t('settingsAdvanced.autocomplete_provider_tooltip')}>
            <Input
              name="default-autocomplete-provider"
              className="w-full"
              placeholder={t('setup.provider_placeholder')}
              value={provider}
              onChange={e => setProvider(e.target.value)}
            />
          </FormField>

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

import { Button, FormField, H2, InlineNotice, NumberInput, P, Panel, Select } from '@contenox/ui';
import { FormEvent, useContext, useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCLIConfig } from '../../../hooks/useCLIConfig';
import { usePutCLIConfig } from '../../../hooks/usePutCLIConfig';
import { AuthContext } from '../../../lib/authContext';
import type { CLIConfigUpdateRequest } from '../../../lib/types';
import { isValidMaxTokens } from './settingsValidation';

const THINK_VALUES = ['auto', 'off', 'minimal', 'low', 'medium', 'high', 'xhigh'] as const;

export function ResponseSettingsSection() {
  const { t } = useTranslation();
  const { user } = useContext(AuthContext);
  const { data } = useCLIConfig(!!user);
  const putConfig = usePutCLIConfig();
  const formId = useId();

  const [maxTokens, setMaxTokens] = useState('');
  const [maxTokensError, setMaxTokensError] = useState('');
  const [think, setThink] = useState('');

  useEffect(() => {
    if (!data) return;
    setMaxTokens(data.defaultMaxTokens ?? '');
    setThink(data.defaultThink ?? '');
  }, [data]);

  useEffect(() => {
    if (!putConfig.isSuccess) return;
    const timer = window.setTimeout(() => putConfig.reset(), 3000);
    return () => window.clearTimeout(timer);
  }, [putConfig.isSuccess, putConfig.reset]);

  const thinkOptions = [
    { value: '', label: t('settingsAdvanced.not_set') },
    ...THINK_VALUES.map(value => ({
      value,
      label: t(`settingsAdvanced.think_option_${value}` as const),
    })),
  ];

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    putConfig.reset();
    setMaxTokensError('');
    const trimmedTokens = maxTokens.trim();
    if (!isValidMaxTokens(trimmedTokens)) {
      setMaxTokensError(t('settingsAdvanced.max_tokens_invalid'));
      return;
    }
    const body: CLIConfigUpdateRequest = {
      'default-max-tokens': trimmedTokens,
      'default-think': think.trim(),
    };
    putConfig.mutate(body);
  };

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settingsAdvanced.response_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settingsAdvanced.response_section_description')}
          </P>
        </div>
        <form id={formId} onSubmit={onSubmit} className="grid gap-4">
          <InlineNotice variant="info" className="rounded-lg">
            {t('settingsAdvanced.restart_notice')}
          </InlineNotice>

          <FormField
            label={t('settingsAdvanced.max_tokens_label')}
            tooltip={t('settingsAdvanced.max_tokens_tooltip')}
            error={maxTokensError}>
            <NumberInput
              name="default-max-tokens"
              className="w-full"
              placeholder={t('settingsAdvanced.max_tokens_placeholder')}
              value={maxTokens}
              onChange={value => setMaxTokens(value ? String(value) : '')}
            />
          </FormField>

          <FormField
            label={t('settingsAdvanced.think_label')}
            tooltip={t('settingsAdvanced.think_tooltip')}>
            <Select
              name="default-think"
              className="w-full"
              value={think}
              onChange={e => setThink(e.target.value)}
              options={thinkOptions}
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

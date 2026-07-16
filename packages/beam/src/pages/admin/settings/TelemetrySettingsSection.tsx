import { Button, Checkbox, H2, P, Panel } from '@contenox/ui';
import { FormEvent, useContext, useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useCLIConfig } from '../../../hooks/useCLIConfig';
import { usePutCLIConfig } from '../../../hooks/usePutCLIConfig';
import { AuthContext } from '../../../lib/authContext';
import type { CLIConfigUpdateRequest } from '../../../lib/types';
import { resolveTelemetryEnabled, resolveUpdateCheckEnabled } from './settingsValidation';

export function TelemetrySettingsSection() {
  const { t } = useTranslation();
  const { user } = useContext(AuthContext);
  const { data } = useCLIConfig(!!user);
  const putConfig = usePutCLIConfig();
  const formId = useId();

  const [telemetryEnabled, setTelemetryEnabled] = useState(false);
  const [updateCheckEnabled, setUpdateCheckEnabled] = useState(true);

  useEffect(() => {
    if (!data) return;
    setTelemetryEnabled(resolveTelemetryEnabled(data.telemetryEnabled));
    setUpdateCheckEnabled(resolveUpdateCheckEnabled(data.updateCheck));
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
      'telemetry-enabled': telemetryEnabled ? 'true' : 'false',
      'update-check': updateCheckEnabled ? 'true' : 'false',
    };
    putConfig.mutate(body);
  };

  return (
    <Panel variant="surface">
      <div className="space-y-4">
        <div className="space-y-1">
          <H2>{t('settingsAdvanced.telemetry_section_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settingsAdvanced.telemetry_section_description')}
          </P>
        </div>
        <form id={formId} onSubmit={onSubmit} className="grid max-w-xl gap-3">
          <div className="space-y-1">
            <Checkbox
              label={t('settingsAdvanced.telemetry_enabled_label')}
              checked={telemetryEnabled}
              onChange={e => setTelemetryEnabled(e.target.checked)}
            />
            <P variant="muted" className="pl-6 text-xs">
              {t('settingsAdvanced.telemetry_enabled_description')}
            </P>
          </div>

          <div className="space-y-1">
            <Checkbox
              label={t('settingsAdvanced.update_check_label')}
              checked={updateCheckEnabled}
              onChange={e => setUpdateCheckEnabled(e.target.checked)}
            />
            <P variant="muted" className="pl-6 text-xs">
              {t('settingsAdvanced.update_check_description')}
            </P>
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

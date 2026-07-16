import { H1, P, Page } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { AutocompleteSettingsSection } from './AutocompleteSettingsSection';
import { GlobalSettingsSection } from './GlobalSettingsSection';
import { ResponseSettingsSection } from './ResponseSettingsSection';
import { SetupWizardSection } from './SetupWizardSection';
import { TelemetrySettingsSection } from './TelemetrySettingsSection';
import { WorkspaceSettingsSection } from './WorkspaceSettingsSection';

export default function SettingsPage() {
  const { t } = useTranslation();

  return (
    <Page bodyScroll="auto">
      <div className="mx-auto flex w-full max-w-4xl flex-col gap-6 p-4 md:p-6">
        <div className="space-y-1">
          <H1 variant="page">{t('settings.page_title')}</H1>
          <P variant="muted" className="max-w-2xl text-sm">
            {t('settings.page_description')}
          </P>
        </div>
        <GlobalSettingsSection />
        <ResponseSettingsSection />
        <AutocompleteSettingsSection />
        <WorkspaceSettingsSection />
        <TelemetrySettingsSection />
        <SetupWizardSection />
      </div>
    </Page>
  );
}

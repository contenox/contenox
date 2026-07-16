import { Button, H2, P, Panel } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import { openWizardManually } from '../../../lib/wizardDismissal';

/**
 * The onboarding wizard doubles as a "reconfigure anytime" flow (see Layout's
 * showWizard/manualOpen wiring): this button clears any dismissal and forces
 * it open regardless of current setup completeness. Layout renders the
 * wizard full-screen for any route once forced open, so no navigation is
 * needed here.
 */
export function SetupWizardSection() {
  const { t } = useTranslation();

  return (
    <Panel variant="surface">
      <div className="flex flex-wrap items-center justify-between gap-4">
        <div className="space-y-1">
          <H2>{t('settings.run_wizard_title')}</H2>
          <P variant="muted" className="text-sm">
            {t('settings.run_wizard_description')}
          </P>
        </div>
        <Button variant="secondary" size="sm" onClick={openWizardManually}>
          {t('settings.run_wizard_action')}
        </Button>
      </div>
    </Panel>
  );
}

import { Button, H2, P, Panel, Span } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { SetupStatus } from '../../../lib/types';

type Props = {
  data: SetupStatus | undefined;
  onRefreshStatus: () => void;
  onFinish: () => void;
};

export default function StepHealthCheck({ data, onRefreshStatus, onFinish }: Props) {
  const { t } = useTranslation();

  const backendCount = data?.backendCount ?? 0;
  const reachableCount = data?.reachableBackendCount ?? 0;
  const totalModels = (data?.backendChecks ?? []).reduce((n, b) => n + (b.chatModelCount ?? 0), 0);
  const isHealthy = reachableCount > 0 && totalModels > 0;

  return (
    <div className="max-w-xl mx-auto space-y-6">
      <div className="space-y-1">
        <H2 className="text-xl font-semibold">{t('onboarding.step_health.title')}</H2>
        <P variant="muted" className="text-sm">
          {t('onboarding.step_health.desc')}
        </P>
      </div>

      {backendCount === 0 && (
        <P variant="muted" className="text-sm">
          {t('onboarding.step_health.no_backends')}
        </P>
      )}

      {backendCount > 0 && !isHealthy && (
        <P variant="muted" className="text-sm">
          {t('onboarding.step_health.waiting')}
        </P>
      )}

      {isHealthy && (
        <Panel variant="flat" className="text-sm">
          {t('onboarding.step_health.all_good', {
            reachable: reachableCount,
            total: backendCount,
            models: totalModels,
          })}
        </Panel>
      )}

      {(data?.backendChecks ?? []).length > 0 && (
        <div className="flex flex-col gap-2 text-sm">
          {data!.backendChecks.map(check => (
            <div key={check.id} className="flex items-start gap-2">
              <Span className={check.reachable ? 'text-success' : 'text-error'}>
                {check.reachable ? '✓' : '✗'}
              </Span>
              <Span>
                <Span className="font-medium">{check.name}</Span>
                {` (${check.type})`}
                {check.error ? ` — ${check.error}` : ''}
                {check.reachable && check.chatModelCount > 0
                  ? ` — ${check.chatModelCount} model(s)`
                  : ''}
                {check.hint ? ` (${check.hint})` : ''}
              </Span>
            </div>
          ))}
        </div>
      )}

      <div className="flex items-center gap-3">
        <Button variant="secondary" size="sm" onClick={onRefreshStatus}>
          {t('onboarding.step_health.refresh')}
        </Button>
        {isHealthy && (
          <Button variant="primary" size="sm" onClick={onFinish}>
            {t('onboarding.step_health.finish')}
          </Button>
        )}
      </div>
    </div>
  );
}

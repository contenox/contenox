import { H2, P } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { TranslationKey } from '../../../i18n';
import { ONBOARDING_PROVIDER_SETUPS } from '../../../lib/providerCatalog';
import type { CloudProviderType } from '../../../lib/types';
import { cn } from '../../../lib/utils';

export type ProviderChoice = 'local' | CloudProviderType;

type ProviderCardProps = {
  id: ProviderChoice;
  title: string;
  desc: string;
  selected: boolean;
  onSelect: (id: ProviderChoice) => void;
};

function ProviderCard({ id, title, desc, selected, onSelect }: ProviderCardProps) {
  return (
    <button
      type="button"
      onClick={() => onSelect(id)}
      className={cn(
        'w-full rounded-lg border p-4 text-left transition-colors',
        selected
          ? 'border-primary-500 bg-surface-100 text-text ring-primary-500/30 dark:border-dark-primary-500 dark:bg-dark-surface-300 dark:text-dark-text dark:ring-dark-primary-500/30 ring-1'
          : 'border-surface-200 bg-surface-50 text-text hover:border-surface-400 hover:bg-surface-100 dark:border-dark-surface-600 dark:bg-dark-surface-100 dark:text-dark-text dark:hover:border-dark-surface-500 dark:hover:bg-dark-surface-300',
      )}>
      <div className="flex items-start gap-3">
        <div
          className={cn(
            'mt-0.5 h-4 w-4 shrink-0 rounded-full border-2 transition-colors',
            selected
              ? 'border-primary-500 bg-primary-500'
              : 'border-surface-400 dark:border-dark-surface-400',
          )}
        />
        <div>
          <P className="text-text dark:text-dark-text text-sm font-semibold">{title}</P>
          <P variant="muted" className="text-text-muted dark:text-dark-text-muted mt-0.5 text-xs">
            {desc}
          </P>
        </div>
      </div>
    </button>
  );
}

type Props = {
  value: ProviderChoice;
  onChange: (v: ProviderChoice) => void;
};

export default function StepChooseProvider({ value, onChange }: Props) {
  const { t } = useTranslation();

  const providers: { id: ProviderChoice; titleKey: TranslationKey; descKey: TranslationKey }[] = [
    {
      id: 'local',
      titleKey: 'onboarding.step_choose_provider.local_title',
      descKey: 'onboarding.step_choose_provider.local_desc',
    },
    {
      id: 'ollama',
      titleKey: 'onboarding.step_choose_provider.ollama_title',
      descKey: 'onboarding.step_choose_provider.ollama_desc',
    },
    ...ONBOARDING_PROVIDER_SETUPS.map(setup => ({
      id: setup.provider,
      titleKey: setup.titleKey,
      descKey: setup.descriptionKey,
    })),
  ];

  return (
    <div className="mx-auto max-w-xl space-y-6">
      <div className="space-y-1">
        <H2 className="text-xl font-semibold">{t('onboarding.step_choose_provider.title')}</H2>
        <P variant="muted" className="text-text-muted dark:text-dark-text-muted text-sm">
          {t('onboarding.step_choose_provider.desc')}
        </P>
      </div>
      <div className="space-y-3">
        {providers.map(p => (
          <ProviderCard
            key={p.id}
            id={p.id}
            title={t(p.titleKey)}
            desc={t(p.descKey)}
            selected={value === p.id}
            onSelect={onChange}
          />
        ))}
      </div>
    </div>
  );
}

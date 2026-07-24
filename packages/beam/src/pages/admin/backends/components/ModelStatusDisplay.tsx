import { Badge, Span } from '@contenox/ui';
import { useTranslation } from 'react-i18next';

type ModelStatusDisplayProps = {
  modelName: string;
};

export function ModelStatusDisplay({ modelName }: ModelStatusDisplayProps) {
  const { t } = useTranslation();

  return (
    <div className="flex items-center justify-between gap-2 py-1" title={modelName}>
      <Span className="min-w-0 truncate text-sm font-medium">{modelName}</Span>
      <Badge variant="success" size="sm" className="shrink-0">
        {t('backends.status.available')}
      </Badge>
    </div>
  );
}

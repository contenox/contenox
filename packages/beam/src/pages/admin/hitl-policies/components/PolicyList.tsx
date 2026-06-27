import { Badge, Button, P, Span } from '@contenox/ui';
import { useTranslation } from 'react-i18next';

interface Props {
  names: string[];
  activeName: string;
  selectedName: string;
  onSelect: (name: string) => void;
  onSetActive: (name: string) => void;
  onDelete: (name: string) => void;
}

export default function PolicyList({
  names,
  activeName,
  selectedName,
  onSelect,
  onSetActive,
  onDelete,
}: Props) {
  const { t } = useTranslation();

  return (
    <div className="border-surface-200 dark:border-dark-surface-700 flex max-h-72 min-h-0 w-full shrink-0 flex-col gap-1 overflow-y-auto border-b p-3 md:max-h-none md:w-64 md:border-r md:border-b-0">
      {names.length === 0 && (
        <P variant="muted" className="text-sm">
          {t('hitl_policies.list_empty')}
        </P>
      )}
      {names.map(name => (
        <div
          key={name}
          className={`group hover:bg-surface-100 dark:hover:bg-dark-surface-100 flex cursor-pointer flex-col gap-1 rounded-md p-2 ${
            selectedName === name ? 'bg-surface-100 dark:bg-dark-surface-100' : ''
          }`}
          onClick={() => onSelect(name)}>
          <div className="flex items-center justify-between gap-1">
            <Span className="truncate text-sm font-medium">{name}</Span>
            {activeName === name && (
              <Badge variant="success" size="sm">
                {t('hitl_policies.active')}
              </Badge>
            )}
          </div>
          <div className="flex gap-1 opacity-100 sm:opacity-0 sm:group-focus-within:opacity-100 sm:group-hover:opacity-100">
            {activeName !== name && (
              <Button
                variant="secondary"
                size="sm"
                onClick={e => {
                  e.stopPropagation();
                  onSetActive(name);
                }}>
                {t('hitl_policies.set_active')}
              </Button>
            )}
            <Button
              variant="danger"
              size="sm"
              onClick={e => {
                e.stopPropagation();
                void onDelete(name);
              }}>
              {t('common.delete')}
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

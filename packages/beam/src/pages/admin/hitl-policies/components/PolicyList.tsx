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

export default function PolicyList({ names, activeName, selectedName, onSelect, onSetActive, onDelete }: Props) {
  const { t } = useTranslation();

  return (
    <div className="flex w-64 flex-col gap-1 overflow-y-auto border-r border-neutral-200 p-3 dark:border-neutral-700">
      {names.length === 0 && (
        <P variant="muted" className="text-sm">{t('hitl_policies.list_empty')}</P>
      )}
      {names.map(name => (
        <div
          key={name}
          className={`group flex flex-col gap-1 rounded-md p-2 cursor-pointer hover:bg-surface-100 dark:hover:bg-dark-surface-100 ${
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
          <div className="flex gap-1 opacity-0 group-hover:opacity-100">
            {activeName !== name && (
              <Button
                variant="secondary"
                size="sm"
                onClick={e => { e.stopPropagation(); onSetActive(name); }}>
                {t('hitl_policies.set_active')}
              </Button>
            )}
            <Button
              variant="secondary"
              size="sm"
              onClick={e => { e.stopPropagation(); void onDelete(name); }}>
              {t('common.delete')}
            </Button>
          </div>
        </div>
      ))}
    </div>
  );
}

import { Button, Dropdown, Section } from '@contenox/ui';
import type { ReactElement } from 'react';
import { t } from 'i18next';
import { ChevronDown } from 'lucide-react';
import { Link } from 'react-router-dom';
import { cn } from '../lib/utils';

type MenuItem = {
  path: string;
  label: string;
  icon?: React.ReactNode;
};

type NavDropdownProps = {
  isOpen: boolean;
  setIsOpen: (open: boolean) => void;
  items: MenuItem[];
  /** When set, replaces the default chevron menu trigger. */
  trigger?: ReactElement;
  contentClassName?: string;
};

export function DropdownMenu({ isOpen, setIsOpen, items, trigger, contentClassName }: NavDropdownProps) {
  return (
    <Dropdown
      isOpen={isOpen}
      onToggle={setIsOpen}
      trigger={
        trigger ?? (
          <Button variant="ghost" size="sm" aria-label={t('common.menu')} className="gap-1">
            <ChevronDown className={cn('h-8 w-4 transition-transform', isOpen && 'rotate-180')} />
          </Button>
        )
      }
      contentClassName={cn('absolute right-0 top-full mt-2 min-w-[160px]', contentClassName)}>
      <Section>
        <nav className="py-2">
          {items.map(item => (
            <Link
              key={item.path}
              to={item.path}
              className={cn(
                'group flex items-center gap-2 px-4 py-2 text-sm text-text dark:text-dark-text',
                'hover:bg-surface-100 hover:text-text dark:hover:bg-dark-surface-300 dark:hover:text-dark-text',
                'focus:bg-surface-100 focus:text-text dark:focus:bg-dark-surface-300 dark:focus:text-dark-text',
                'transition-colors duration-150',
              )}
              onClick={() => setIsOpen(false)}>
              {item.icon && (
                <span className="flex h-4 w-4 shrink-0 items-center justify-center text-text-muted group-hover:text-text group-focus:text-text dark:text-dark-text-muted dark:group-hover:text-dark-text dark:group-focus:text-dark-text">
                  {item.icon}
                </span>
              )}
              <span className="truncate">{item.label}</span>
            </Link>
          ))}
        </nav>
      </Section>
    </Dropdown>
  );
}

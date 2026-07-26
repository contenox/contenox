import { SidebarProps } from './Sidebar';
import { SidebarNav } from './SidebarNav';

// MobileSidebar.tsx
export function MobileSidebar({ isOpen, setIsOpen, items = [], children }: SidebarProps) {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-x-0 top-16 bottom-0 z-50 overflow-x-hidden sm:hidden">
      <div
        className="bg-surface-900/20 dark:bg-black/40 backdrop-blur-sm fixed inset-x-0 top-16 bottom-0 z-40 min-h-0 transition-opacity"
        onClick={() => setIsOpen(false)}
      />
      <div className="border-surface-200/50 dark:border-dark-surface-600/50 bg-surface-50/90 dark:bg-dark-surface-100/90 backdrop-blur-xl relative z-50 flex h-full min-h-0 flex-col border-r shadow-2xl">
        {children ?? <SidebarNav items={items} setIsOpen={setIsOpen} />}
      </div>
    </div>
  );
}

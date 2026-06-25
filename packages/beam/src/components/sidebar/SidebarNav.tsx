import { NavItem } from '@contenox/ui';
import { Link, useLocation } from 'react-router-dom';
import { MenuItem } from './Sidebar';

export function SidebarNav({
  items,
  setIsOpen,
}: {
  items: MenuItem[];
  setIsOpen: (open: boolean) => void;
}) {
  const { pathname } = useLocation();

  return (
    <nav className="flex-1 space-y-1 p-4">
      {items.map(item => (
        <NavItem
          key={item.path}
          as={Link}
          to={item.path}
          isActive={pathname === item.path || pathname.startsWith(item.path + '/')}
          icon={item.icon}
          onClick={() => setIsOpen(false)}
        >
          {item.label}
        </NavItem>
      ))}
    </nav>
  );
}

import { cn } from "../utils";
import type { ReactNode } from "react";

/**
 * A styled navigation item for sidebar / rail navigation.
 *
 * Router-agnostic: render as `<NavItem as={Link} to="/path">` when using
 * react-router, or as a plain anchor (`<NavItem href="/path">`). Defaults to
 * rendering a `<div>` when neither `as` nor `href` is provided so it can be
 * used as a button-like item via the `onClick` prop.
 *
 * Variants:
 * - default  — standard interactive item
 * - active   — currently selected route
 */

export type NavItemProps = {
  /** When true, applies the active highlight styles. */
  isActive?: boolean;
  /** Leading icon (e.g. a Lucide icon). */
  icon?: ReactNode;
  children: ReactNode;
  className?: string;
  onClick?: () => void;
  /** Override the rendered element. Pass a router Link component here. */
  as?: React.ElementType;
  /** Forwarded to the rendered element (useful for `<a>` or router Link). */
  href?: string;
  /** Forwarded when `as` is a router Link. */
  to?: string;
};

export function NavItem({
  isActive = false,
  icon,
  children,
  className,
  onClick,
  as: As,
  href,
  to,
}: NavItemProps) {
  const Tag = As ?? (href ? "a" : "div");

  return (
    <Tag
      href={href}
      to={to}
      onClick={onClick}
      className={cn(
        "flex items-center gap-3 rounded-lg px-4 py-2.5 transition-colors",
        isActive
          ? "bg-primary-50/50 dark:bg-dark-primary-900/30 text-primary-700 dark:text-dark-primary-400 font-medium"
          : "text-text dark:text-dark-text hover:bg-surface-100 dark:hover:bg-dark-surface-100",
        className,
      )}
    >
      {icon && (
        <span className="text-primary dark:text-dark-primary shrink-0">{icon}</span>
      )}
      <span className="truncate">{children}</span>
    </Tag>
  );
}

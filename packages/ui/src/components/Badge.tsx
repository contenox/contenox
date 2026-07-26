import { cn } from "../utils";

type BadgeVariant =
  | "default"
  | "primary"
  | "accent"
  | "success"
  | "error"
  | "warning"
  | "outline"
  | "secondary";

type BadgeSize = "sm" | "md";

type BadgeProps = React.HTMLAttributes<HTMLSpanElement> & {
  variant?: BadgeVariant;
  size?: BadgeSize;
};

export function Badge({
  className,
  variant = "default",
  size = "md",
  ...props
}: BadgeProps) {
  const baseStyles = cn(
    "inline-flex items-center font-medium rounded-full transition-colors",
    {
      "px-2.5 py-0.5 text-xs": size === "sm",
      "px-3 py-1 text-sm": size === "md",
    },
  );

  const variantStyles: Record<BadgeVariant, string> = {
    default: cn(
      "border border-surface-200/50 dark:border-dark-surface-700/50",
      "bg-surface-50/80 dark:bg-dark-surface-200/80 text-text dark:text-dark-text backdrop-blur-sm shadow-sm",
    ),
    primary: cn(
      "border border-primary-500/20 dark:border-dark-primary-500/20",
      "bg-primary-500/10 dark:bg-dark-primary-500/15 text-primary-600 dark:text-dark-primary-300 backdrop-blur-sm",
    ),
    accent: cn(
      "border border-accent-500/20 dark:border-dark-accent-500/20",
      "bg-accent-500/10 dark:bg-dark-accent-500/15 text-accent-600 dark:text-dark-accent-300 backdrop-blur-sm",
    ),
    success: cn(
      "border border-success-200 dark:border-dark-success-200",
      "bg-success-50 text-success-700",
      "dark:bg-dark-success-50 dark:text-dark-success-800",
    ),
    error: cn(
      "border border-error-200 dark:border-dark-error-200",
      "bg-error-50 text-error-700",
      "dark:bg-dark-error-50 dark:text-dark-error-800",
    ),
    warning: cn(
      "border border-warning-200 dark:border-dark-warning-200",
      "bg-warning-50 text-warning-800",
      "dark:bg-dark-warning-50 dark:text-dark-warning-800",
    ),
    secondary: cn(
      "bg-secondary-500/10 text-secondary-800",
      "dark:bg-dark-secondary-500/15 dark:text-dark-secondary-300 backdrop-blur-sm border border-transparent",
    ),
    outline: cn(
      "bg-transparent border border-secondary-300 text-secondary-700",
      "dark:border-dark-secondary-600 dark:text-dark-secondary-300",
    ),
  };

  return (
    <span
      className={cn(baseStyles, variantStyles[variant], className)}
      {...props}
    />
  );
}

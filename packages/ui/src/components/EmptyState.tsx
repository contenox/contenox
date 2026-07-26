import { cn } from "../utils";
import { Section } from "./Section";
import { H3, P } from "./Typography";

type EmptyStateVariant = "default" | "error" | "warning" | "success" | "info";

type EmptyStateProps = {
  title: string;
  subtitle?: string;
  description: string;
  icon?: React.ReactNode;
  className?: string;
  orientation?: "vertical" | "horizontal";
  iconSize?: "sm" | "md" | "lg";
  variant?: EmptyStateVariant;
};

export function EmptyState({
  title,
  subtitle,
  description,
  icon,
  className,
  orientation = "vertical",
  iconSize = "md",
  variant = "default",
}: EmptyStateProps) {
  const variantStyles: Record<EmptyStateVariant, string> = {
    default: cn("text-text dark:text-dark-text"),
    info: cn(
      "text-text dark:text-dark-text",
      "bg-surface-50 dark:bg-dark-surface-50",
    ),
    /* Dark semantic ramps are inverted (50 = darkest, 900 = palest):
       dark bg wants a LOW step, dark text a HIGH step. */
    success: cn(
      "text-[var(--color-success-800)] dark:text-[var(--color-dark-success-700)]",
      "bg-[var(--color-success-50)] dark:bg-[var(--color-dark-success-100)]",
    ),
    warning: cn(
      "text-[var(--color-warning-800)] dark:text-[var(--color-dark-warning-700)]",
      "bg-[var(--color-warning-50)] dark:bg-[var(--color-dark-warning-100)]",
    ),
    error: cn(
      "text-[var(--color-error-800)] dark:text-[var(--color-dark-error-700)]",
      "bg-[var(--color-error-50)] dark:bg-[var(--color-dark-error-100)]",
    ),
  };

  return (
    <Section
      title={title}
      variant="body"
      className={cn(
        "p-8",
        orientation === "horizontal"
          ? "flex items-center gap-6 text-left"
          : "text-center",
        variantStyles[variant],
        className,
      )}
    >
      {icon && (
        <div
          className={cn(
            "relative flex items-center justify-center text-primary-500 dark:text-dark-primary-400",
            orientation === "horizontal" ? "flex-shrink-0" : "mx-auto mb-6",
            {
              "w-20 h-20 text-4xl": iconSize === "lg",
              "w-16 h-16 text-3xl": iconSize === "md",
              "w-12 h-12 text-2xl": iconSize === "sm",
            },
          )}
        >
          <div className="absolute inset-0 bg-primary-500/20 dark:bg-dark-primary-500/20 blur-xl rounded-full" />
          <div className="relative z-10 flex items-center justify-center w-full h-full bg-surface-50/50 dark:bg-dark-surface-200/50 rounded-full shadow-sm border border-surface-200/50 dark:border-dark-surface-600/50 backdrop-blur-md">
            {icon}
          </div>
        </div>
      )}
      {subtitle && (
        <P
          variant="lead"
          className="text-text-muted dark:text-dark-text-muted"
        >
          {subtitle}
        </P>
      )}
      {description && (
        <H3 className="text-text-secondary dark:text-dark-text-secondary font-medium mt-2">
          {description}
        </H3>
      )}
    </Section>
  );
}

import React, { forwardRef } from "react";
import { cn } from "../utils";

type CardLayout = "default" | "space-between";

type StatusBorder = "default" | "success" | "error" | "warning" | "info";

type CardProps = React.HTMLAttributes<HTMLDivElement> & {
  variant?: "default" | "filled" | "surface" | "error" | "bordered" | "dotted";
  layout?: CardLayout;
  statusBorder?: StatusBorder;
};

export const Card = forwardRef<HTMLDivElement, CardProps>(
  ({ className, variant = "default", layout = "default", statusBorder, ...props }, ref) => {
    const statusBorderStyles: Record<StatusBorder, string> = {
      default: "border-l-4 border-l-border dark:border-l-dark-surface-600",
      success: "border-l-4 border-l-success dark:border-l-dark-success",
      error: "border-l-4 border-l-error dark:border-l-dark-error",
      warning: "border-l-4 border-l-warning dark:border-l-dark-warning",
      info: "border-l-4 border-l-info dark:border-l-dark-info",
    };
    const dottedPatternClasses =
      "[--dot-size:1px] [--dot-gap:18px] " +
      "[--dot-color:rgba(0,0,0,0.14)] dark:[--dot-color:rgba(255,255,255,0.14)] " +
      "[background-image:radial-gradient(var(--dot-color)_var(--dot-size),transparent_var(--dot-size))] " +
      "[background-size:var(--dot-gap)_var(--dot-gap)] " +
      "bg-surface-100 dark:bg-dark-surface-700 " +
      "border-surface-300 dark:border-dark-surface-600";

    return (
      <div
        ref={ref}
        className={cn(
          "rounded-xl border p-6 transition-all duration-300 shadow-sm backdrop-blur-sm",
          "hover:-translate-y-1 hover:border-indigo-500/30 hover:shadow-lg hover:shadow-indigo-500/10 dark:hover:border-indigo-400/30",
          {
            "bg-white/80 border-neutral-200/60 dark:bg-neutral-950/80 dark:border-neutral-800/60":
              variant === "default",
            "bg-surface-100/80 border-surface-200/60 dark:bg-dark-surface-300/80 dark:border-dark-surface-400/60":
              variant === "filled",
            "bg-surface-200/80 border-surface-300/60 dark:bg-dark-surface-400/80 dark:border-dark-surface-500/60":
              variant === "surface",
            "bg-error-50 dark:bg-dark-error-50 text-error dark:text-dark-error-800":
              variant === "error",
            "border border-surface-400 dark:border-dark-surface-500":
              variant === "bordered",
            [dottedPatternClasses]: variant === "dotted",
          },
          {
            "flex items-center justify-between": layout === "space-between",
          },
          statusBorder && statusBorderStyles[statusBorder],
          className,
        )}
        {...props}
      />
    );
  },
);

Card.displayName = "Card";

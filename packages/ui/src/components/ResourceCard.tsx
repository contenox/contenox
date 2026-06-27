import type { ReactNode } from "react";
import { cn } from "../utils";
import { Button } from "./Button";
import { ButtonGroup } from "./ButtonGroup";
import { Panel } from "./Panel";
import { Spinner } from "./Spinner";
import { H2 } from "./Typography";

interface ResourceCardProps {
  title: string;
  subtitle?: ReactNode;
  badge?: ReactNode;
  status?: "default" | "success" | "error" | "warning";
  children: ReactNode;
  actions?: {
    edit?: () => void;
    delete?: () => void;
    custom?: ReactNode;
  };
  isLoading?: boolean;
  className?: string;
}

const statusBorderStyles: Record<
  NonNullable<ResourceCardProps["status"]>,
  string
> = {
  default: "border-l-4 border-l-border dark:border-l-dark-surface-600",
  success: "border-l-4 border-l-success dark:border-l-dark-success",
  error: "border-l-4 border-l-error dark:border-l-dark-error",
  warning: "border-l-4 border-l-warning dark:border-l-dark-warning",
};

export function ResourceCard({
  title,
  subtitle,
  badge,
  status = "default",
  children,
  actions,
  isLoading = false,
  className = "",
}: ResourceCardProps) {
  return (
    <Panel
      variant="bordered"
      className={cn(
        "bg-surface dark:bg-dark-surface-100 relative space-y-4 rounded-lg",
        statusBorderStyles[status],
        className,
      )}
    >
      <div className="flex min-w-0 flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
        <div className="min-w-0 space-y-1">
          <H2 className="break-words">{title}</H2>
          {subtitle && (
            <div className="text-text-muted dark:text-dark-text-muted text-sm">
              {subtitle}
            </div>
          )}
        </div>
        {badge && <div className="shrink-0">{badge}</div>}
      </div>

      <div className="space-y-4">{children}</div>

      {actions && (
        <div className="border-t pt-4">
          <ButtonGroup className="flex items-center justify-between">
            <div className="flex gap-2">
              {actions.edit && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={actions.edit}
                  disabled={isLoading}
                >
                  Edit
                </Button>
              )}
              {actions.custom}
            </div>

            {actions.delete && (
              <Button
                variant="danger"
                size="sm"
                onClick={actions.delete}
                disabled={isLoading}
              >
                {isLoading ? (
                  <>
                    <Spinner size="sm" />
                    Deleting
                  </>
                ) : (
                  "Delete"
                )}
              </Button>
            )}
          </ButtonGroup>
        </div>
      )}
    </Panel>
  );
}

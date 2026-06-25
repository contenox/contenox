import { Label } from "./Label";
import { P } from "./Typography";
import { cn } from "../utils";
import { HelpCircle } from "lucide-react";
import { Tooltip } from "./Tooltip";

export type FormFieldProps = {
  label: string | React.ReactNode;
  required?: boolean;
  error?: string;
  description?: string;
  tooltip?: string;
  children: React.ReactNode;
  className?: string;
};

export function FormField({
  label,
  required,
  error,
  description,
  tooltip,
  children,
  className,
}: FormFieldProps) {
  return (
    <div className={cn("space-y-1.5", className)}>
      <div className="flex items-baseline justify-between">
        <div className="flex items-center gap-1">
          <Label className="text-sm font-medium">
            {label}
            {required && (
              <span className="text-error dark:text-dark-error">*</span>
            )}
          </Label>
          {tooltip && (
            <Tooltip content={tooltip}>
              <HelpCircle className="h-4 w-4 text-text-muted dark:text-dark-text-muted cursor-help" />
            </Tooltip>
          )}
        </div>
        {description && (
          <span className="text-xs text-text-muted dark:text-dark-text-muted">
            {description}
          </span>
        )}
      </div>

      {children}

      {error && (
        <P className="text-xs text-error dark:text-dark-error flex items-center gap-1">
          {error}
        </P>
      )}
    </div>
  );
}

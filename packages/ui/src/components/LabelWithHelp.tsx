import { HelpCircle } from "lucide-react";
import { cn } from "../utils";
import { Label } from "./Label";
import { Tooltip } from "./Tooltip";

export type LabelWithHelpProps = {
  /** Text of the label. */
  label: string;
  /** Distilled 1-3 sentence explanation shown in a hover/focus tooltip. */
  tooltip?: string;
  required?: boolean;
  className?: string;
  /**
   * Rendering element: "label" for form fields (pairs with an input via
   * htmlFor), "span" for read-only captions above a value/badge. Defaults to
   * "span" since this component is most useful outside of <FormField> —
   * FormField already has its own built-in tooltip affordance for inputs.
   */
  as?: "label" | "span";
  htmlFor?: string;
};

/**
 * A label (or read-only caption) paired with an optional info-icon tooltip.
 * Mirrors the tooltip affordance already built into FormField, for places
 * that show a labeled value without an input — status badges, table row
 * captions, button hints, etc. Keeping both call sites visually consistent
 * (HelpCircle + Tooltip) is the point: one help pattern across the app.
 */
export function LabelWithHelp({
  label,
  tooltip,
  required,
  className,
  as = "span",
  htmlFor,
}: LabelWithHelpProps) {
  const textClassName =
    as === "span"
      ? "text-text-muted dark:text-dark-text-muted text-xs"
      : "text-sm font-medium";

  return (
    <div className={cn("flex items-center gap-1", className)}>
      {as === "label" ? (
        <Label htmlFor={htmlFor} className={textClassName}>
          {label}
          {required && (
            <span className="text-error dark:text-dark-error">*</span>
          )}
        </Label>
      ) : (
        <span className={textClassName}>
          {label}
          {required && (
            <span className="text-error dark:text-dark-error">*</span>
          )}
        </span>
      )}
      {tooltip && (
        <Tooltip content={tooltip}>
          <HelpCircle className="text-text-muted dark:text-dark-text-muted h-3.5 w-3.5 cursor-help" />
        </Tooltip>
      )}
    </div>
  );
}

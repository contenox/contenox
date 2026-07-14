import { useEffect } from "react";
import { createPortal } from "react-dom";
import { CheckCircle2, X, XCircle } from "lucide-react";
import { cn } from "../utils";
import { Span } from "./Typography";

type ToastProps = {
  message: string;
  variant: "success" | "error";
  className?: string;
  /** When provided, renders a dismiss button and enables auto-dismiss. */
  onDismiss?: () => void;
  /** Auto-dismiss after this many milliseconds (requires onDismiss). */
  duration?: number;
  /** Accessible name of the dismiss button. */
  dismissLabel?: string;
};

export function Toast({
  message,
  variant,
  className,
  onDismiss,
  duration,
  dismissLabel = "Dismiss",
}: ToastProps) {
  useEffect(() => {
    if (!onDismiss || !duration) return;
    const timer = setTimeout(onDismiss, duration);
    return () => clearTimeout(timer);
  }, [onDismiss, duration]);

  return createPortal(
    <div
      // Errors interrupt (alert/assertive); successes wait their turn.
      role={variant === "error" ? "alert" : "status"}
      aria-live={variant === "error" ? "assertive" : "polite"}
      className={cn(
        "fixed bottom-4 left-1/2 z-50 -translate-x-1/2 rounded-lg p-4 shadow-lg",
        "flex items-center gap-3",
        variant === "success"
          ? "bg-primary-500 text-text-inverted dark:bg-dark-primary-600 dark:text-dark-text-inverted"
          : "bg-error-500 text-text-inverted dark:bg-dark-error-600 dark:text-dark-text",
        className,
      )}
    >
      {variant === "success" ? (
        <CheckCircle2 className="h-5 w-5" />
      ) : (
        <XCircle className="h-5 w-5" />
      )}
      <Span className="text-sm font-medium">{message}</Span>
      {onDismiss && (
        <button
          type="button"
          onClick={onDismiss}
          aria-label={dismissLabel}
          className="rounded-sm p-0.5 opacity-80 hover:opacity-100 focus-visible:opacity-100"
        >
          <X className="h-4 w-4" />
        </button>
      )}
    </div>,
    document.body,
  );
}

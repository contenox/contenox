// components/Dialog.tsx
import { useEffect, useId, useRef } from "react";
import { createPortal } from "react-dom";
import { X } from "lucide-react";
import { cn } from "../utils";
import { Card } from "./Card";
import { H3 } from "./Typography";
import { Button } from "./Button";

type DialogProps = {
  open: boolean;
  onClose: () => void;
  title: string;
  children: React.ReactNode;
  className?: string;
  /** Accessible name of the icon-only close button. */
  closeLabel?: string;
};

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function Dialog({
  open,
  onClose,
  title,
  children,
  className,
  closeLabel = "Close",
}: DialogProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const titleId = useId();
  // Ref so the trap effect doesn't tear down and re-run when a consumer passes
  // a new onClose identity every render.
  const onCloseRef = useRef(onClose);
  onCloseRef.current = onClose;

  // Focus management while open: move focus into the panel, trap Tab inside,
  // close on Escape, and restore focus to the opener on close/unmount.
  useEffect(() => {
    if (!open) return;
    const previouslyFocused = document.activeElement as HTMLElement | null;
    panelRef.current?.focus();

    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.stopPropagation();
        onCloseRef.current();
        return;
      }
      if (e.key !== "Tab") return;
      const panel = panelRef.current;
      if (!panel) return;
      const focusables =
        panel.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR);
      if (focusables.length === 0) {
        e.preventDefault();
        return;
      }
      const first = focusables[0];
      const last = focusables[focusables.length - 1];
      const active = document.activeElement;
      if (e.shiftKey) {
        if (active === first || active === panel) {
          e.preventDefault();
          last.focus();
        }
      } else if (active === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", handleKeyDown, true);
    return () => {
      document.removeEventListener("keydown", handleKeyDown, true);
      previouslyFocused?.focus?.();
    };
  }, [open]);

  // Scroll lock: the page behind a modal must not scroll.
  useEffect(() => {
    if (!open) return;
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
    };
  }, [open]);

  if (!open) return null;

  return createPortal(
    <div
      className="fixed inset-0 z-50 bg-black/50 backdrop-blur-sm"
      onClick={onClose}
    >
      <div
        ref={panelRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="fixed top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 focus:outline-none"
        onClick={(e) => e.stopPropagation()}
      >
        <Card className={cn("w-[400px]", className)}>
          <div className="mb-4 flex items-center justify-between">
            <H3
              id={titleId}
              className="text-primary-600 dark:text-dark-primary-500 text-lg font-semibold"
            >
              {title}
            </H3>
            <Button
              onClick={onClose}
              aria-label={closeLabel}
              className="text-secondary-500 hover:bg-secondary-100 dark:text-dark-secondary-400 dark:hover:bg-dark-surface-200 rounded-sm p-1"
            >
              <X className="h-5 w-5 dark:text-dark-secondary-400" />
            </Button>
          </div>
          {children}
        </Card>
      </div>
    </div>,
    document.body,
  );
}

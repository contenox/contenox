// packages/ui/src/Tabs/Tabs.tsx
import React, { useRef } from "react";
import { X } from "lucide-react";
import { TabTrigger } from "./TabTrigger";
import { cn } from "../utils";

export type Tab<T extends string = string> = {
  id: T;
  label: React.ReactNode;
  disabled?: boolean;
  /** Renders a close ✕ affordance on this tab's trigger (see {@link TabsProps.onClose}). */
  closable?: boolean;
  /** Accessible label for this tab's close button (e.g. "Close tab: Session 1a2b"). */
  closeLabel?: string;
};

export interface TabsProps<T extends string = string> {
  tabs: readonly Tab<T>[];
  activeTab: T;
  onTabChange: (tabId: T) => void;
  /**
   * Called when a `closable` tab's ✕ is activated. The click is stopped from
   * propagating to the trigger, so closing a tab never also switches to it.
   */
  onClose?: (tabId: T) => void;
  className?: string;
}

/**
 * Modern, accessible Tabs component
 * - Keyboard navigation (← → Home End)
 * - Uses TabTrigger for styling
 * - Optional per-tab close ✕ (`closable` + `onClose`), rendered as its own
 *   button beside the trigger (a real sibling, not a nested <button>) so it
 *   can stop propagation and close without switching tabs.
 * - Drop-in replacement for the old Tabs
 */
export function Tabs<T extends string = string>({
  tabs,
  activeTab,
  onTabChange,
  onClose,
  className,
}: TabsProps<T>) {
  const refs = useRef<Record<string, HTMLButtonElement | null>>({});

  const onKeyDown: React.KeyboardEventHandler<HTMLDivElement> = (e) => {
    const idx = tabs.findIndex((t) => t.id === activeTab);
    if (idx === -1) return;

    let nextIdx = idx;
    if (e.key === "ArrowRight") nextIdx = (idx + 1) % tabs.length;
    else if (e.key === "ArrowLeft")
      nextIdx = (idx - 1 + tabs.length) % tabs.length;
    else if (e.key === "Home") nextIdx = 0;
    else if (e.key === "End") nextIdx = tabs.length - 1;
    else return;

    e.preventDefault();
    const nextId = tabs[nextIdx].id;
    onTabChange(nextId);
    refs.current[String(nextId)]?.focus();
  };

  return (
    <div
      role="tablist"
      aria-orientation="horizontal"
      className={cn(
        "flex max-w-full flex-wrap gap-1 overflow-x-auto overflow-y-hidden",
        className,
      )}
      onKeyDown={onKeyDown}
    >
      {tabs.map((tab) => {
        const isActive = tab.id === activeTab;
        const closable = tab.closable && !!onClose;
        return (
          <div key={String(tab.id)} className="relative inline-flex items-stretch">
            <TabTrigger
              ref={(el) => {
                refs.current[tab.id] = el ?? null;
              }}
              active={isActive}
              disabled={tab.disabled}
              id={`tab-${String(tab.id)}`}
              aria-controls={`panel-${String(tab.id)}`}
              tabIndex={isActive ? 0 : -1}
              onClick={() => onTabChange(tab.id)}
              className={cn(closable && "pr-9")}
            >
              {tab.label}
            </TabTrigger>
            {closable && (
              <button
                type="button"
                aria-label={tab.closeLabel}
                tabIndex={isActive ? 0 : -1}
                className={cn(
                  "absolute top-1/2 right-1.5 -translate-y-1/2 rounded p-0.5",
                  "text-foreground/50 hover:text-foreground hover:bg-white/10",
                  "focus:outline-none focus-visible:ring-1 focus-visible:ring-primary/30",
                )}
                onClick={(e) => {
                  e.stopPropagation();
                  onClose?.(tab.id);
                }}
              >
                <X className="h-3.5 w-3.5" aria-hidden />
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}

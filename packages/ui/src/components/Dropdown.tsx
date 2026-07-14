import React, { useState, useEffect, useRef } from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "../utils";
import { Button } from "./Button";

export interface DropdownProps {
  isOpen?: boolean;
  onToggle?: (isOpen: boolean) => void;
  trigger?: React.ReactElement<{ onClick?: React.MouseEventHandler<Element> }>;
  options?: { value: string; label: string }[];
  value?: string;
  onChange?: (value: string) => void;
  children?: React.ReactNode;
  contentClassName?: string;
  className?: string;
  /** Accessible label of the built-in trigger when no option is selected. */
  placeholder?: string;
}

export function Dropdown({
  isOpen: controlledOpen,
  onToggle,
  trigger,
  options,
  value,
  onChange,
  children,
  contentClassName,
  className,
  placeholder = "Select",
}: DropdownProps) {
  const [internalOpen, setInternalOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  // The element that had focus when the dropdown opened; Escape and selection
  // return focus there so keyboard users don't get dropped at <body>.
  const openerRef = useRef<HTMLElement | null>(null);
  const isControlled = controlledOpen !== undefined;
  const isOpen = isControlled ? controlledOpen : internalOpen;

  const setOpen = (next: boolean) => {
    if (next && !isOpen) {
      openerRef.current = document.activeElement as HTMLElement | null;
    }
    if (!isControlled) setInternalOpen(next);
    onToggle?.(next);
  };

  const toggle = () => setOpen(!isOpen);
  const close = () => setOpen(false);

  const closeRef = useRef(close);
  closeRef.current = close;

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (
        dropdownRef.current &&
        !dropdownRef.current.contains(event.target as Node)
      ) {
        closeRef.current();
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  const itemElements = () =>
    Array.from(
      contentRef.current?.querySelectorAll<HTMLElement>(
        '[role="option"], [role="menuitem"], button, a[href]',
      ) ?? [],
    );

  const focusItem = (target: "first" | "last" | 1 | -1) => {
    const items = itemElements();
    if (items.length === 0) return;
    if (target === "first") {
      (
        items.find((el) => el.getAttribute("aria-selected") === "true") ??
        items[0]
      ).focus();
      return;
    }
    if (target === "last") {
      items[items.length - 1].focus();
      return;
    }
    const current = items.indexOf(document.activeElement as HTMLElement);
    const next =
      current === -1 ? 0 : (current + target + items.length) % items.length;
    items[next].focus();
  };

  // Keyboard contract: arrows open and move focus through the items, Home/End
  // jump, Escape closes and restores focus, Tab closes and lets focus move on.
  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case "Escape":
        if (isOpen) {
          e.stopPropagation();
          close();
          openerRef.current?.focus();
        }
        break;
      case "ArrowDown":
      case "ArrowUp": {
        e.preventDefault();
        if (!isOpen) {
          setOpen(true);
          requestAnimationFrame(() =>
            focusItem(e.key === "ArrowDown" ? "first" : "last"),
          );
        } else {
          focusItem(e.key === "ArrowDown" ? 1 : -1);
        }
        break;
      }
      case "Home":
        if (isOpen) {
          e.preventDefault();
          focusItem("first");
        }
        break;
      case "End":
        if (isOpen) {
          e.preventDefault();
          focusItem("last");
        }
        break;
      case "Tab":
        if (isOpen) close();
        break;
    }
  };

  // Close when focus leaves the widget entirely (e.g. Tab past the last item).
  const handleBlur = (e: React.FocusEvent) => {
    if (isOpen && !e.currentTarget.contains(e.relatedTarget as Node)) {
      close();
    }
  };

  const selectOption = (optionValue: string) => {
    onChange?.(optionValue);
    close();
    openerRef.current?.focus();
  };

  const triggerElement = trigger ? (
    React.cloneElement(trigger, {
      onClick: (e: React.MouseEvent) => {
        e.stopPropagation();
        trigger.props.onClick?.(e);
        toggle();
      },
      "aria-haspopup": options ? "listbox" : true,
      "aria-expanded": isOpen,
    } as React.HTMLAttributes<HTMLElement>)
  ) : options ? (
    <Button
      variant="ghost"
      onClick={toggle}
      aria-haspopup="listbox"
      aria-expanded={isOpen}
      className={cn(
        "border-secondary-300 bg-surface-50 flex items-center justify-between rounded-lg border px-4 py-2.5",
        "focus:ring-primary-500 focus:ring-2 focus:ring-offset-2",
        "dark:border-dark-secondary-300 dark:bg-dark-surface-50",
      )}
    >
      <span className="text-text dark:text-dark-text">
        {options.find((opt) => opt.value === value)?.label || placeholder}
      </span>
      <ChevronDown className="text-secondary-400 dark:text-dark-secondary-400 h-5 w-5" />
    </Button>
  ) : null;

  const content = children
    ? children
    : options
      ? options.map((option) => (
          <Button
            variant="ghost"
            key={option.value}
            role="option"
            aria-selected={option.value === value}
            onClick={() => selectOption(option.value)}
            className={cn(
              "text-text hover:bg-secondary-100 w-full px-4 py-2 text-left",
              "dark:text-dark-text dark:hover:bg-dark-surface-100",
              option.value === value &&
                "bg-primary-50 dark:bg-dark-primary-900",
            )}
          >
            {option.label}
          </Button>
        ))
      : null;

  return (
    <div
      className={cn("relative", className)}
      ref={dropdownRef}
      onKeyDown={handleKeyDown}
      onBlur={handleBlur}
    >
      {triggerElement}
      {isOpen && (
        <div
          ref={contentRef}
          className={cn(
            "absolute z-50 mt-2 w-full rounded-lg border bg-surface-50 dark:bg-dark-surface-200 shadow-lg",
            contentClassName,
          )}
          role={options && !children ? "listbox" : undefined}
        >
          {content}
        </div>
      )}
    </div>
  );
}

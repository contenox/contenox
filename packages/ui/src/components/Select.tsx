import { forwardRef } from "react";
import { cn } from "../utils";

type SelectProps = React.SelectHTMLAttributes<HTMLSelectElement> & {
  options: Array<{ value: string; label: string }>;
  placeholder?: string;
};

export const Select = forwardRef<HTMLSelectElement, SelectProps>(
  ({ className, options, placeholder, ...props }, ref) => (
    <select
      ref={ref}
      className={cn(
        "rounded-lg border h-9 pl-3 pr-8 py-1 text-sm",
        "text-text dark:text-dark-text",
        "bg-surface-50 dark:bg-dark-surface-50",
        "border-surface-300 dark:border-dark-surface-600",
        "focus:ring-2 focus:outline-none",
        "focus:ring-primary-500 dark:focus:ring-dark-primary-500",
        "focus:border-transparent",
        "focus:ring-offset-2 focus:ring-offset-surface-50 dark:focus:ring-offset-dark-surface-100",
        "appearance-none bg-no-repeat transition-colors",
        "bg-[url('data:image/svg+xml,%3csvg%20xmlns=%27http://www.w3.org/2000/svg%27%20width=%2724%27%20height=%2724%27%20viewBox=%270%200%2024%2024%27%20fill=%27none%27%20stroke=%27%23888%27%20stroke-width=%272%27%20stroke-linecap=%27round%27%20stroke-linejoin=%27round%27%3e%3cpath%20d=%27m6%209%206%206%206-6%27/%3e%3c/svg%3e')]",
        "bg-[length:16px_16px] bg-[position:right_10px_center]",
        "hover:bg-surface-100 dark:hover:bg-dark-surface-100",
        className,
      )}
      {...props}
    >
      {placeholder && (
        <SelectOption value="" disabled hidden>
          {placeholder}
        </SelectOption>
      )}
      {options.map((option) => (
        <SelectOption key={option.value} value={option.value}>
          {option.label}
        </SelectOption>
      ))}
    </select>
  ),
);
Select.displayName = "Select";

type SelectOptionProps = React.OptionHTMLAttributes<HTMLOptionElement>;

export const SelectOption = forwardRef<HTMLOptionElement, SelectOptionProps>(
  ({ className, ...props }, ref) => (
    <option
      ref={ref}
      className={cn(
        "bg-surface-50 text-text dark:bg-dark-surface-50 dark:text-dark-text",
        className,
      )}
      {...props}
    />
  ),
);
SelectOption.displayName = "SelectOption";

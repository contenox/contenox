import { Input, Select } from '@contenox/ui';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { WorkspaceRoot } from '../../lib/types';
import { shortenRootPath } from '../../lib/workspaceRoots';

/** Select sentinel for "type a path outside the offered roots". */
export const CUSTOM_ROOT_VALUE = '__custom_root__';

export interface RootSelectorProps {
  /** Current working directory (the DispatchRequest.cwd being built). */
  value: string;
  /** Emits the new cwd (`''` means "inherit the default root"). */
  onChange: (cwd: string) => void;
  /** The allowlisted roots; ignored when `isAbsent`. */
  roots: readonly WorkspaceRoot[];
  /** True when serve exposes no allowlist — degrade to a plain free-path input. */
  isAbsent: boolean;
}

/**
 * The dispatch form's working-directory control. When serve publishes a
 * workspace-root allowlist it offers those roots as first-class options plus a
 * "custom path" escape hatch (a free-path input); the chosen value feeds
 * `DispatchRequest.cwd`, and the server does the real bounds check on submit —
 * this UI never pretends to validate the path itself, it only makes the
 * allowlist legible. When the allowlist is absent (older serve / no roots
 * configured) it degrades to a single free-path input, preserving the prior
 * behavior. A refused path surfaces as a legible 422 owned by the parent
 * FormField, not a raw wire string.
 */
export function RootSelector({ value, onChange, roots, isAbsent }: RootSelectorProps) {
  const { t } = useTranslation();
  const isKnownRoot = roots.some(r => r.path === value);
  // Custom mode when the user picks the escape hatch, or the seeded value is a
  // path that isn't one of the offered roots (and isn't the empty default).
  const [customMode, setCustomMode] = useState(!isKnownRoot && value.trim() !== '');

  if (isAbsent) {
    return (
      <Input
        type="text"
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={t('roots.free_path_placeholder')}
      />
    );
  }

  const options = [
    ...roots.map(r => ({
      value: r.path,
      label: r.default
        ? `${shortenRootPath(r.path)} — ${t('roots.default_marker')}`
        : shortenRootPath(r.path),
    })),
    { value: CUSTOM_ROOT_VALUE, label: t('roots.selector_custom') },
  ];

  const selectValue = customMode ? CUSTOM_ROOT_VALUE : value;

  const handleSelect = (next: string) => {
    if (next === CUSTOM_ROOT_VALUE) {
      setCustomMode(true);
      onChange('');
      return;
    }
    setCustomMode(false);
    onChange(next);
  };

  return (
    <div className="flex flex-col gap-2">
      <Select
        className="w-full"
        value={selectValue}
        onChange={e => handleSelect(e.target.value)}
        options={options}
        placeholder={t('roots.selector_placeholder')}
      />
      {customMode && (
        <Input
          type="text"
          value={value}
          onChange={e => onChange(e.target.value)}
          placeholder={t('roots.free_path_placeholder')}
          aria-label={t('roots.free_path_label')}
        />
      )}
    </div>
  );
}

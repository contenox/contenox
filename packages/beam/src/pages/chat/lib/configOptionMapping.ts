import type {
  SessionConfigGroup,
  SessionConfigOption,
  SessionConfigOptionValue,
  SessionConfigOptionValues,
  SessionConfigValue,
} from '../../../lib/acp';

/**
 * Pure mapping between the wire shape of `SessionConfigOption` (flat values,
 * grouped values, or boolean) and the props a control (Select/Checkbox) needs
 * to render it, plus the reverse mapping back to the wire value
 * `setConfigOption` expects. No React — ConfigOptionControls.tsx wires this.
 */

export type ConfigControlKind = 'boolean' | 'select-flat' | 'select-grouped';

export interface SelectControlOption {
  value: string;
  label: string;
}

export interface SelectControlGroup {
  group: string;
  label: string;
  options: SelectControlOption[];
}

export interface ConfigControlProps {
  kind: ConfigControlKind;
  /** boolean controls only. */
  checked?: boolean;
  /** select controls only — the wire `currentValue`. */
  value?: string;
  /** select-flat only. */
  options?: SelectControlOption[];
  /** select-grouped only. */
  groups?: SelectControlGroup[];
}

export function isGroupedOptions(options: SessionConfigOptionValues): options is SessionConfigGroup[] {
  return options.length > 0 && 'group' in options[0];
}

/** Flattens grouped values into a single option list for controls (e.g. a plain `<select>`) that don't render `<optgroup>` — group name prefixed onto the label so the value stays unambiguous. */
export function flattenGroups(groups: SelectControlGroup[]): SelectControlOption[] {
  return groups.flatMap(g => g.options.map(o => ({ value: o.value, label: `${g.label} / ${o.label}` })));
}

/** Maps one `SessionConfigOption` to the props its control needs. `type === 'boolean'` wins regardless of `options` (booleans carry no options list on the wire). */
export function toControlProps(option: SessionConfigOption): ConfigControlProps {
  if (option.type === 'boolean') {
    return { kind: 'boolean', checked: option.currentValue === 'true' };
  }
  if (isGroupedOptions(option.options)) {
    return {
      kind: 'select-grouped',
      value: option.currentValue,
      groups: option.options.map((g: SessionConfigGroup) => ({
        group: g.group,
        label: g.name,
        options: g.options.map((o: SessionConfigValue) => ({ value: o.value, label: o.name })),
      })),
    };
  }
  return {
    kind: 'select-flat',
    value: option.currentValue,
    options: (option.options as SessionConfigValue[]).map(o => ({ value: o.value, label: o.name })),
  };
}

/** Maps a control's raw output value back to the wire value `setConfigOption` expects for that control kind. */
export function fromControlValue(kind: ConfigControlKind, raw: string | boolean): SessionConfigOptionValue {
  if (kind === 'boolean') return typeof raw === 'boolean' ? raw : raw === 'true';
  return String(raw);
}

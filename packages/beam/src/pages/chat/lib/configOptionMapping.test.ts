import { describe, expect, it } from 'vitest';
import type { SessionConfigOption } from '../../../lib/acp';
import { flattenGroups, fromControlValue, isGroupedOptions, toControlProps } from './configOptionMapping';

describe('toControlProps: boolean', () => {
  it('maps type "boolean" + currentValue "true" to checked: true', () => {
    const opt: SessionConfigOption = { id: 'think', name: 'Think', type: 'boolean', currentValue: 'true', options: [] };
    expect(toControlProps(opt)).toEqual({ kind: 'boolean', checked: true });
  });

  it('maps currentValue "false" to checked: false', () => {
    const opt: SessionConfigOption = { id: 'think', name: 'Think', type: 'boolean', currentValue: 'false', options: [] };
    expect(toControlProps(opt)).toEqual({ kind: 'boolean', checked: false });
  });

  it('boolean type wins even if options is non-empty (should not happen on the wire, but must not crash)', () => {
    const opt: SessionConfigOption = {
      id: 'x',
      name: 'X',
      type: 'boolean',
      currentValue: 'true',
      options: [{ value: 'a', name: 'A' }],
    };
    expect(toControlProps(opt).kind).toBe('boolean');
  });
});

describe('toControlProps: flat select', () => {
  it('maps a flat value list to select-flat with mapped label/value', () => {
    const opt: SessionConfigOption = {
      id: 'model',
      name: 'Model',
      type: 'select',
      currentValue: 'fast',
      options: [
        { value: 'fast', name: 'Fast' },
        { value: 'slow', name: 'Thorough' },
      ],
    };
    expect(toControlProps(opt)).toEqual({
      kind: 'select-flat',
      value: 'fast',
      options: [
        { value: 'fast', label: 'Fast' },
        { value: 'slow', label: 'Thorough' },
      ],
    });
  });

  it('maps an empty options list to select-flat with an empty options array', () => {
    const opt: SessionConfigOption = { id: 'x', name: 'X', type: 'select', currentValue: '', options: [] };
    expect(toControlProps(opt)).toEqual({ kind: 'select-flat', value: '', options: [] });
  });
});

describe('toControlProps: grouped select', () => {
  const opt: SessionConfigOption = {
    id: 'provider',
    name: 'Provider',
    type: 'select',
    currentValue: 'gpt',
    options: [
      { group: 'openai', name: 'OpenAI', options: [{ value: 'gpt', name: 'GPT' }] },
      { group: 'local', name: 'Local', options: [{ value: 'llama', name: 'Llama' }] },
    ],
  };

  it('detects grouped values via isGroupedOptions', () => {
    expect(isGroupedOptions(opt.options)).toBe(true);
  });

  it('maps grouped values to select-grouped preserving group labels', () => {
    expect(toControlProps(opt)).toEqual({
      kind: 'select-grouped',
      value: 'gpt',
      groups: [
        { group: 'openai', label: 'OpenAI', options: [{ value: 'gpt', label: 'GPT' }] },
        { group: 'local', label: 'Local', options: [{ value: 'llama', label: 'Llama' }] },
      ],
    });
  });

  it('flattenGroups prefixes each option label with its group label', () => {
    const props = toControlProps(opt);
    expect(flattenGroups(props.groups!)).toEqual([
      { value: 'gpt', label: 'OpenAI / GPT' },
      { value: 'llama', label: 'Local / Llama' },
    ]);
  });
});

describe('fromControlValue', () => {
  it('coerces a boolean control back to a real boolean', () => {
    expect(fromControlValue('boolean', true)).toBe(true);
    expect(fromControlValue('boolean', false)).toBe(false);
  });

  it('coerces a string "true"/"false" input (e.g. from a native checkbox event) for boolean controls', () => {
    expect(fromControlValue('boolean', 'true')).toBe(true);
    expect(fromControlValue('boolean', 'false')).toBe(false);
  });

  it('stringifies select control values regardless of kind', () => {
    expect(fromControlValue('select-flat', 'fast')).toBe('fast');
    expect(fromControlValue('select-grouped', 'gpt')).toBe('gpt');
  });
});

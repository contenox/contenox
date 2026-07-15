import { describe, expect, it } from 'vitest';
import type { PermissionOption } from '../../../lib/acp';
import { keyHintForOption, optionForKey, orderedPermissionOptions, safestRejectOption } from './permissionKeymap';

function opt(optionId: string, kind: PermissionOption['kind'], name = optionId): PermissionOption {
  return { optionId, kind, name };
}

describe('optionForKey', () => {
  const options = [
    opt('allow-1', 'allow_once'),
    opt('allow-2', 'allow_always'),
    opt('reject-1', 'reject_once'),
  ];

  it('maps y/a/n to their kind, case-insensitively', () => {
    expect(optionForKey(options, 'y')?.optionId).toBe('allow-1');
    expect(optionForKey(options, 'Y')?.optionId).toBe('allow-1');
    expect(optionForKey(options, 'a')?.optionId).toBe('allow-2');
    expect(optionForKey(options, 'A')?.optionId).toBe('allow-2');
    expect(optionForKey(options, 'n')?.optionId).toBe('reject-1');
  });

  it('returns null for unbound keys', () => {
    expect(optionForKey(options, 'x')).toBeNull();
    expect(optionForKey(options, 'Enter')).toBeNull();
  });

  it('returns null when no option of the bound kind is offered', () => {
    expect(optionForKey([opt('allow-1', 'allow_once')], 'n')).toBeNull();
  });

  it('resolves by KIND, not position — reordering the options array does not rebind keys', () => {
    const reordered = [opt('reject-1', 'reject_once'), opt('allow-2', 'allow_always'), opt('allow-1', 'allow_once')];
    expect(optionForKey(reordered, 'y')?.optionId).toBe('allow-1');
    expect(optionForKey(reordered, 'n')?.optionId).toBe('reject-1');
  });
});

describe('safestRejectOption', () => {
  it('prefers reject_once when offered', () => {
    const options = [opt('a', 'allow_once'), opt('r1', 'reject_once'), opt('r2', 'reject_always')];
    expect(safestRejectOption(options)?.optionId).toBe('r1');
  });

  it('falls back to any reject_* kind when reject_once is absent', () => {
    const options = [opt('a', 'allow_once'), opt('r2', 'reject_always')];
    expect(safestRejectOption(options)?.optionId).toBe('r2');
  });

  it('returns null when no reject option is offered at all', () => {
    const options = [opt('a1', 'allow_once'), opt('a2', 'allow_always')];
    expect(safestRejectOption(options)).toBeNull();
  });
});

describe('orderedPermissionOptions', () => {
  it('sorts allow options before reject options, preserving within-group order', () => {
    const options = [
      opt('r1', 'reject_once'),
      opt('a1', 'allow_once'),
      opt('r2', 'reject_always'),
      opt('a2', 'allow_always'),
    ];
    expect(orderedPermissionOptions(options).map(o => o.optionId)).toEqual(['a1', 'a2', 'r1', 'r2']);
  });

  it('is a no-op copy when already ordered', () => {
    const options = [opt('a1', 'allow_once'), opt('r1', 'reject_once')];
    expect(orderedPermissionOptions(options)).toEqual(options);
  });
});

describe('keyHintForOption', () => {
  it('hints Y/A/N for the three bound kinds', () => {
    expect(keyHintForOption(opt('a', 'allow_once'))).toBe('Y');
    expect(keyHintForOption(opt('a', 'allow_always'))).toBe('A');
    expect(keyHintForOption(opt('a', 'reject_once'))).toBe('N');
  });

  it('has no hint for reject_always (no single-key binding for the most destructive option)', () => {
    expect(keyHintForOption(opt('a', 'reject_always'))).toBeNull();
  });
});

import { describe, expect, it } from 'vitest';
import {
  activeWorkspaceRoot,
  extractRefusedRoot,
  isWorkspaceRootRefusal,
  shortenRootPath,
} from './workspaceRoots';
import type { WorkspaceRoot } from './types';

describe('activeWorkspaceRoot', () => {
  it('prefers the default-flagged root over position', () => {
    const roots: WorkspaceRoot[] = [
      { path: '/a', default: false },
      { path: '/b', default: true },
    ];
    expect(activeWorkspaceRoot(roots)?.path).toBe('/b');
  });

  it('falls back to the first root when none is flagged default', () => {
    const roots: WorkspaceRoot[] = [
      { path: '/a', default: false },
      { path: '/b', default: false },
    ];
    expect(activeWorkspaceRoot(roots)?.path).toBe('/a');
  });

  it('is undefined for an empty allowlist', () => {
    expect(activeWorkspaceRoot([])).toBeUndefined();
  });
});

describe('shortenRootPath', () => {
  it('returns a short path unchanged (sans trailing slash)', () => {
    expect(shortenRootPath('/home/naro')).toBe('/home/naro');
    expect(shortenRootPath('/home/naro/')).toBe('/home/naro');
  });

  it('keeps the last N segments with an ellipsis prefix when longer', () => {
    expect(shortenRootPath('/home/naro/src/github.com/contenox/runtime')).toBe(
      '…/github.com/contenox/runtime',
    );
  });

  it('honours a custom segment budget', () => {
    expect(shortenRootPath('/a/b/c/d/e', 2)).toBe('…/d/e');
  });

  it('never collapses to empty', () => {
    expect(shortenRootPath('/')).toBe('/');
    expect(shortenRootPath('')).toBe('');
  });
});

describe('isWorkspaceRootRefusal', () => {
  it('matches the server refusal phrasing regardless of the wrapped prefix or case', () => {
    expect(
      isWorkspaceRootRefusal('unprocessable entity: workspace root "/etc" is not permitted'),
    ).toBe(true);
    expect(isWorkspaceRootRefusal('Workspace Root "/x" Is Not Permitted')).toBe(true);
  });

  it('does not match unrelated errors', () => {
    expect(isWorkspaceRootRefusal('too many terminal sessions')).toBe(false);
    expect(isWorkspaceRootRefusal('')).toBe(false);
    expect(isWorkspaceRootRefusal(undefined)).toBe(false);
  });
});

describe('extractRefusedRoot', () => {
  it('pulls the quoted offending path', () => {
    expect(
      extractRefusedRoot('unprocessable entity: workspace root "/etc/passwd" is not permitted'),
    ).toBe('/etc/passwd');
  });

  it('returns null when there is no quoted path', () => {
    expect(extractRefusedRoot('workspace root is not permitted')).toBeNull();
    expect(extractRefusedRoot(undefined)).toBeNull();
  });
});

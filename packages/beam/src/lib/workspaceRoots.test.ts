import { describe, expect, it } from 'vitest';
import {
  activeWorkspaceRoot,
  extractRefusedRoot,
  isWorkspaceRootRefusal,
  projectForCwd,
  projectName,
  shortenRootPath,
  workspaceNameForCwd,
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
    expect(shortenRootPath('/home/user')).toBe('/home/user');
    expect(shortenRootPath('/home/user/')).toBe('/home/user');
  });

  it('keeps the last N segments with an ellipsis prefix when longer', () => {
    expect(shortenRootPath('/home/user/src/github.com/contenox/runtime')).toBe(
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

describe('projectForCwd', () => {
  const roots: WorkspaceRoot[] = [
    { path: '/a', default: false },
    { path: '/a/b', default: false },
  ];

  it('returns the longest segment-aware prefix (deepest root wins)', () => {
    expect(projectForCwd('/a/b/c', roots)?.path).toBe('/a/b');
    expect(projectForCwd('/a/c', roots)?.path).toBe('/a');
  });

  it('matches a root that equals the cwd exactly', () => {
    expect(projectForCwd('/a/b', roots)?.path).toBe('/a/b');
  });

  it('respects the segment boundary — /a/b does not match /a/bc', () => {
    expect(projectForCwd('/a/bc', roots)?.path).toBe('/a');
    expect(projectForCwd('/a/bc', [{ path: '/a/b', default: false }])).toBeNull();
  });

  it('tolerates trailing slashes on both root and cwd', () => {
    expect(projectForCwd('/a/b/', [{ path: '/a/b/', default: false }])?.path).toBe('/a/b/');
  });

  it('returns null when no root contains the cwd', () => {
    expect(projectForCwd('/x/y', roots)).toBeNull();
  });

  it('returns null for an absent cwd', () => {
    expect(projectForCwd(undefined, roots)).toBeNull();
    expect(projectForCwd(null, roots)).toBeNull();
    expect(projectForCwd('', roots)).toBeNull();
  });
});

describe('projectName', () => {
  it('prefers the server-supplied name', () => {
    expect(projectName({ path: '/home/user/runtime', default: false, name: 'Runtime' })).toBe(
      'Runtime',
    );
  });

  it('falls back to the path basename when the name is empty or absent', () => {
    expect(projectName({ path: '/home/user/runtime', default: false, name: '  ' })).toBe('runtime');
    expect(projectName({ path: '/home/user/runtime', default: false })).toBe('runtime');
  });
});

describe('workspaceNameForCwd', () => {
  const roots: WorkspaceRoot[] = [
    { path: '/a', default: false, name: 'Alpha' },
    { path: '/a/b', default: false, name: 'Beta' },
  ];

  it('names the deepest matching root', () => {
    expect(workspaceNameForCwd('/a/b/c', roots)).toBe('Beta');
    expect(workspaceNameForCwd('/a/c', roots)).toBe('Alpha');
  });

  it('ignores unnamed roots — a structural root must not swallow the label', () => {
    // The bare home/default root has no marker name; a session under it keeps
    // the caller's cwd-basename fallback instead of reading as the root.
    expect(workspaceNameForCwd('/a/b/c', [{ path: '/a/b', default: false }])).toBeNull();
    expect(
      workspaceNameForCwd('/home/me/demo-app', [
        { path: '/home/me', default: true, managed: false },
      ]),
    ).toBeNull();
  });

  it('names from the deepest NAMED root even when a deeper unnamed root matches', () => {
    expect(
      workspaceNameForCwd('/a/b/c/d', [
        { path: '/a/b', default: false, name: 'Beta' },
        { path: '/a/b/c', default: false },
      ]),
    ).toBe('Beta');
  });

  it('is null when no root matches (caller uses a bare-cwd fallback)', () => {
    expect(workspaceNameForCwd('/x/y', roots)).toBeNull();
    expect(workspaceNameForCwd(undefined, roots)).toBeNull();
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

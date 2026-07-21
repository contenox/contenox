import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../lib/fetch';
import { deriveWorkspaceRootsState, fetchWorkspaceRoots } from './useWorkspaceRoots';

vi.mock('../lib/api', () => ({
  api: {
    getWorkspaceRoots: vi.fn(),
  },
}));

const { api } = await import('../lib/api');

beforeEach(() => {
  vi.clearAllMocks();
});

describe('fetchWorkspaceRoots', () => {
  it('returns the roots array when present', async () => {
    vi.mocked(api.getWorkspaceRoots).mockResolvedValueOnce({
      roots: [
        { path: '/a', default: true },
        { path: '/b', default: false },
      ],
    });
    await expect(fetchWorkspaceRoots()).resolves.toEqual([
      { path: '/a', default: true },
      { path: '/b', default: false },
    ]);
  });

  it('folds a 404 into the absent sentinel (null), not an error', async () => {
    vi.mocked(api.getWorkspaceRoots).mockRejectedValueOnce(new ApiError('not found', 404));
    await expect(fetchWorkspaceRoots()).resolves.toBeNull();
  });

  it('returns an empty array for a configured-but-empty allowlist', async () => {
    vi.mocked(api.getWorkspaceRoots).mockResolvedValueOnce({ roots: [] });
    await expect(fetchWorkspaceRoots()).resolves.toEqual([]);
  });

  it('rethrows non-404 errors (a genuine failure is not silently absent)', async () => {
    vi.mocked(api.getWorkspaceRoots).mockRejectedValueOnce(new ApiError('boom', 500));
    await expect(fetchWorkspaceRoots()).rejects.toMatchObject({ status: 500 });
  });
});

describe('deriveWorkspaceRootsState', () => {
  it('marks absent when data is the null sentinel and exposes no default', () => {
    const s = deriveWorkspaceRootsState(null, false, null);
    expect(s.isAbsent).toBe(true);
    expect(s.roots).toEqual([]);
    expect(s.defaultRoot).toBeUndefined();
  });

  it('is not absent for an empty configured allowlist', () => {
    const s = deriveWorkspaceRootsState([], false, null);
    expect(s.isAbsent).toBe(false);
    expect(s.roots).toEqual([]);
  });

  it('surfaces the default root and never reads as absent when populated', () => {
    const s = deriveWorkspaceRootsState(
      [
        { path: '/a', default: false },
        { path: '/b', default: true },
      ],
      false,
      null,
    );
    expect(s.isAbsent).toBe(false);
    expect(s.defaultRoot?.path).toBe('/b');
  });

  it('treats a still-loading (undefined) result as not-absent', () => {
    const s = deriveWorkspaceRootsState(undefined, true, null);
    expect(s.isAbsent).toBe(false);
    expect(s.isLoading).toBe(true);
  });
});

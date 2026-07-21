import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../lib/fetch';
import { fetchOperatorInbox } from './useOperatorInbox';

vi.mock('../lib/api', () => ({
  api: {
    getOperatorInbox: vi.fn(),
  },
}));

const { api } = await import('../lib/api');

beforeEach(() => {
  vi.clearAllMocks();
});

describe('fetchOperatorInbox', () => {
  it('returns the items array when present', async () => {
    const items = [
      {
        id: 'item-1',
        missionId: 'm1',
        reason: 'operator_fired' as const,
        report: {
          id: 'r1',
          missionId: 'm1',
          kind: 'progress' as const,
          summary: 'made progress',
          createdAt: '2026-07-21T09:00:00Z',
        },
        createdAt: '2026-07-21T09:00:01Z',
      },
    ];
    vi.mocked(api.getOperatorInbox).mockResolvedValueOnce(items);
    await expect(fetchOperatorInbox()).resolves.toEqual(items);
  });

  it('folds a 404 into the absent sentinel (null), not an error', async () => {
    vi.mocked(api.getOperatorInbox).mockRejectedValueOnce(new ApiError('not found', 404));
    await expect(fetchOperatorInbox()).resolves.toBeNull();
  });

  it('returns an empty array for a genuinely empty inbox', async () => {
    vi.mocked(api.getOperatorInbox).mockResolvedValueOnce([]);
    await expect(fetchOperatorInbox()).resolves.toEqual([]);
  });

  it('rethrows non-404 errors (a genuine failure is not silently absent)', async () => {
    vi.mocked(api.getOperatorInbox).mockRejectedValueOnce(new ApiError('boom', 500));
    await expect(fetchOperatorInbox()).rejects.toMatchObject({ status: 500 });
  });
});

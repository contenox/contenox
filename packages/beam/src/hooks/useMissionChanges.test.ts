import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../lib/fetch';
import type { MissionChangesResponse } from '../lib/types';
import { deriveMissionChangesState, fetchMissionChanges } from './useMissionChanges';

vi.mock('../lib/api', () => ({
  api: { getMissionChanges: vi.fn() },
}));

import { api } from '../lib/api';
const getMissionChanges = api.getMissionChanges as unknown as ReturnType<typeof vi.fn>;

afterEach(() => vi.clearAllMocks());

const sample: MissionChangesResponse = {
  files: [{ path: '/repo/a.ts', status: 'modified', score: 5 }],
  incomplete: false,
  scope: { files: 1, dirs: 1, anomaly: false },
};

describe('fetchMissionChanges', () => {
  it('returns the response when present', async () => {
    getMissionChanges.mockResolvedValueOnce(sample);
    await expect(fetchMissionChanges('m1')).resolves.toEqual(sample);
  });

  it('folds a 404 into the absent sentinel (null), not an error', async () => {
    getMissionChanges.mockRejectedValueOnce(new ApiError('not found', 404));
    await expect(fetchMissionChanges('m1')).resolves.toBeNull();
  });

  it('rethrows a non-404 error (a real failure must surface)', async () => {
    getMissionChanges.mockRejectedValueOnce(new ApiError('boom', 500));
    await expect(fetchMissionChanges('m1')).rejects.toThrow('boom');
  });
});

describe('deriveMissionChangesState', () => {
  const noop = () => {};
  it('marks absent when the data is the null sentinel', () => {
    const s = deriveMissionChangesState(null, false, null, noop);
    expect(s.isAbsent).toBe(true);
    expect(s.data).toBeUndefined();
  });
  it('exposes the response and is not absent when present', () => {
    const s = deriveMissionChangesState(sample, false, null, noop);
    expect(s.isAbsent).toBe(false);
    expect(s.data).toEqual(sample);
  });
  it('passes loading and error through', () => {
    const err = new Error('x');
    expect(deriveMissionChangesState(undefined, true, null, noop).isLoading).toBe(true);
    expect(deriveMissionChangesState(undefined, false, err, noop).error).toBe(err);
  });
});

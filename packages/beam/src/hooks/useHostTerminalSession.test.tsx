import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../lib/fetch';
import { createHostTerminalSession } from './useHostTerminalSession';

vi.mock('../lib/api', () => ({
  api: {
    createTerminalSession: vi.fn(),
  },
}));

const { api } = await import('../lib/api');

beforeEach(() => {
  vi.clearAllMocks();
});

describe('createHostTerminalSession', () => {
  it('returns the opened session on success and omits a blank cwd from the wire', async () => {
    vi.mocked(api.createTerminalSession).mockResolvedValueOnce({
      id: 'sess-1',
      wsPath: '/api/terminal/sessions/sess-1/ws',
    });
    await expect(createHostTerminalSession('', 80, 24)).resolves.toEqual({
      id: 'sess-1',
      wsPath: '/api/terminal/sessions/sess-1/ws',
    });
    expect(api.createTerminalSession).toHaveBeenCalledWith({ cols: 80, rows: 24 });
  });

  it('includes a non-blank cwd', async () => {
    vi.mocked(api.createTerminalSession).mockResolvedValueOnce({ id: 'x', wsPath: '/ws' });
    await createHostTerminalSession('/home/user/project', 120, 40);
    expect(api.createTerminalSession).toHaveBeenCalledWith({
      cwd: '/home/user/project',
      cols: 120,
      rows: 40,
    });
  });

  it('folds a 404 into the absent sentinel (null)', async () => {
    vi.mocked(api.createTerminalSession).mockRejectedValueOnce(new ApiError('not found', 404));
    await expect(createHostTerminalSession('', 80, 24)).resolves.toBeNull();
  });

  it('propagates a 422 refusal (too many sessions / bad cwd) as a real error', async () => {
    vi.mocked(api.createTerminalSession).mockRejectedValueOnce(
      new ApiError('too many terminal sessions', 422),
    );
    await expect(createHostTerminalSession('', 80, 24)).rejects.toMatchObject({ status: 422 });
  });
});

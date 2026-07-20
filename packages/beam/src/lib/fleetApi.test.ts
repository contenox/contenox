import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

/**
 * Wire-level tests for the two fleet lifecycle verbs, in the same shape as
 * authApi.test.ts: `fetch` is stubbed, so these pin the exact path, method and
 * — for cancel — whether a body is sent at all. The presence/absence of the
 * body is the whole cancel-all-vs-per-session distinction on the wire
 * (runtime/internal/fleetapi: an absent body means "every session").
 */
function jsonResponse(body: unknown, ok = true, status = 200): Response {
  return {
    ok,
    status,
    headers: new Headers({ 'Content-Type': 'application/json' }),
    json: async () => body,
    text: async () => JSON.stringify(body),
  } as unknown as Response;
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  vi.unstubAllGlobals();
});

function lastCall() {
  const [url, init] = fetchMock.mock.calls[fetchMock.mock.calls.length - 1] as [
    string,
    RequestInit,
  ];
  return { url: String(url), init };
}

describe('fleet lifecycle api', () => {
  it('stopInstance DELETEs /api/fleet/{id} and returns the server string', async () => {
    fetchMock.mockResolvedValue(jsonResponse('deleted'));
    await expect(api.stopInstance('inst-1')).resolves.toBe('deleted');
    const { url, init } = lastCall();
    expect(url).toContain('/api/fleet/inst-1');
    expect(init.method).toBe('DELETE');
    expect(init.body).toBeUndefined();
  });

  it('stopInstance percent-encodes the instance id', async () => {
    fetchMock.mockResolvedValue(jsonResponse('deleted'));
    await api.stopInstance('inst/../x');
    expect(lastCall().url).toContain('/api/fleet/inst%2F..%2Fx');
  });

  it('cancelInstance with a session id POSTs that session in the body', async () => {
    fetchMock.mockResolvedValue(jsonResponse('cancelled'));
    await expect(api.cancelInstance('inst-1', 'sess-a')).resolves.toBe('cancelled');
    const { url, init } = lastCall();
    expect(url).toContain('/api/fleet/inst-1/cancel');
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toEqual({ sessionId: 'sess-a' });
  });

  it('cancelInstance without a session id sends NO body — the server cancels every session', async () => {
    fetchMock.mockResolvedValue(jsonResponse('cancelled'));
    await api.cancelInstance('inst-1');
    const { url, init } = lastCall();
    expect(url).toContain('/api/fleet/inst-1/cancel');
    expect(init.method).toBe('POST');
    expect(init.body).toBeUndefined();
  });

  it('an empty session id is treated as cancel-all, not as a session named ""', async () => {
    fetchMock.mockResolvedValue(jsonResponse('cancelled'));
    await api.cancelInstance('inst-1', '');
    expect(lastCall().init.body).toBeUndefined();
  });

  it('surfaces a 404 from an unknown instance on cancel as a rejected ApiError', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ error: { message: 'not found' } }, false, 404));
    await expect(api.cancelInstance('nope')).rejects.toMatchObject({ status: 404 });
  });
});

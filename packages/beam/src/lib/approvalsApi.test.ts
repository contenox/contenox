import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

/**
 * Wire-level tests for the approval inbox verbs, in the same shape as
 * fleetApi.test.ts / authApi.test.ts: `fetch` is stubbed, so these pin the exact
 * path, method, and body of GET /api/approvals and POST /api/approvals/{id}.
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

describe('approvals api', () => {
  it('listApprovals GETs /api/approvals', async () => {
    fetchMock.mockResolvedValue(jsonResponse([]));
    await expect(api.listApprovals()).resolves.toEqual([]);
    const { url, init } = lastCall();
    expect(url).toContain('/api/approvals');
    expect(url).not.toContain('limit');
    // A GET goes through apiFetch with no explicit method/body.
    expect(init?.body).toBeUndefined();
  });

  it('listApprovals passes a limit when given', async () => {
    fetchMock.mockResolvedValue(jsonResponse([]));
    await api.listApprovals(10);
    expect(lastCall().url).toContain('/api/approvals?limit=10');
  });

  it('answerApproval POSTs {approved:true} to /api/approvals/{id}', async () => {
    fetchMock.mockResolvedValue(jsonResponse('approved'));
    await expect(api.answerApproval('ask-1', true)).resolves.toBe('approved');
    const { url, init } = lastCall();
    expect(url).toContain('/api/approvals/ask-1');
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toEqual({ approved: true });
  });

  it('answerApproval sends {approved:false} for a deny', async () => {
    fetchMock.mockResolvedValue(jsonResponse('denied'));
    await api.answerApproval('ask-2', false);
    expect(JSON.parse(String(lastCall().init.body))).toEqual({ approved: false });
  });

  it('percent-encodes the approval id', async () => {
    fetchMock.mockResolvedValue(jsonResponse('approved'));
    await api.answerApproval('ask/../x', true);
    expect(lastCall().url).toContain('/api/approvals/ask%2F..%2Fx');
  });

  it('surfaces a 404 from an unknown ask as a rejected ApiError', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ error: { message: 'not found' } }, false, 404));
    await expect(api.answerApproval('nope', true)).rejects.toMatchObject({ status: 404 });
  });
});

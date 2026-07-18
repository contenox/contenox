import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';
import { apiFetch } from './fetch';

/**
 * Wire-level tests for the remote-access endpoints: they assert the client hits
 * the right /ui/* path, method, and body, and parses the JSON back. `fetch` is
 * stubbed so no server is needed (apiFetch resolves the base URL to
 * http://localhost:32123 in the node test env).
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

describe('remote-access auth api', () => {
  it('getAuthStatus GETs /ui/auth-status and returns the parsed status', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ required: true, authenticated: false }));
    const status = await api.getAuthStatus();
    expect(status).toEqual({ required: true, authenticated: false });
    const { url, init } = lastCall();
    expect(url).toContain('/ui/auth-status');
    expect((init.method ?? 'GET').toUpperCase()).toBe('GET');
  });

  it('loginWithToken POSTs the token to /ui/login', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ required: true, authenticated: true }));
    const status = await api.loginWithToken('super-secret');
    expect(status).toEqual({ required: true, authenticated: true });
    const { url, init } = lastCall();
    expect(url).toContain('/ui/login');
    expect(init.method).toBe('POST');
    expect(init.credentials).toBe('same-origin');
    expect(JSON.parse(String(init.body))).toEqual({ token: 'super-secret' });
  });

  it('surfaces a 401 from a wrong token as a rejected ApiError', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ required: true, authenticated: false }, false, 401));
    await expect(api.loginWithToken('wrong')).rejects.toMatchObject({ status: 401 });
  });

  it('logout POSTs /ui/logout', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ required: true, authenticated: false }));
    await api.logout();
    const { url, init } = lastCall();
    expect(url).toContain('/ui/logout');
    expect(init.method).toBe('POST');
  });
});

describe('browser auth is pure-cookie (no localStorage bearer)', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('sends no Authorization header even when a stale token sits in localStorage, and uses same-origin credentials', async () => {
    // Simulate a browser still holding the old token that an earlier build wrote
    // to localStorage. The request layer must ignore it entirely — the HttpOnly
    // cookie is the only browser credential now.
    vi.stubGlobal('localStorage', {
      getItem: (k: string) => (k === 'contenox_api_token' ? 'stale-raw-token' : null),
      setItem: () => {},
      removeItem: () => {},
    });
    fetchMock.mockResolvedValue(jsonResponse({ required: true, authenticated: false }));

    await apiFetch('/api/state');

    const { init } = lastCall();
    const headers = new Headers(init.headers as HeadersInit);
    expect(headers.has('Authorization')).toBe(false);
    expect(init.credentials).toBe('same-origin');
  });
});

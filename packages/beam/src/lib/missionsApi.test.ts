import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { api } from './api';

/**
 * Wire-level tests for the mission surface: list/get/reports over
 * /api/missions, plus fire-a-mission (`dispatchMission`, POST
 * /api/fleet/dispatch) — grouped here rather than in fleetApi.test.ts because
 * conceptually all four are "mission mode" calls even though dispatch's URL
 * sits under /fleet (see runtime/missionservice's package doc: "every
 * dispatch is a mission"). Same fetch-stub shape as fleetApi.test.ts.
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

describe('missions api', () => {
  it('listMissions GETs /api/missions with no query string by default', async () => {
    fetchMock.mockResolvedValue(jsonResponse([]));
    await api.listMissions();
    const { url, init } = lastCall();
    const parsed = new URL(url);
    expect(parsed.pathname).toBe('/api/missions');
    expect(parsed.search).toBe('');
    expect(init.method ?? 'GET').toBe('GET');
  });

  it('listMissions forwards limit and cursor as query params', async () => {
    fetchMock.mockResolvedValue(jsonResponse([]));
    await api.listMissions({ limit: 10, cursor: '2026-07-20T00:00:00Z' });
    const parsed = new URL(lastCall().url);
    expect(parsed.searchParams.get('limit')).toBe('10');
    expect(parsed.searchParams.get('cursor')).toBe('2026-07-20T00:00:00Z');
  });

  it('getMission GETs /api/missions/{id}', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: 'm-1' }));
    await api.getMission('m-1');
    expect(new URL(lastCall().url).pathname).toBe('/api/missions/m-1');
  });

  it('getMission percent-encodes the id', async () => {
    fetchMock.mockResolvedValue(jsonResponse({ id: 'weird' }));
    await api.getMission('m/../x');
    expect(lastCall().url).toContain('/api/missions/m%2F..%2Fx');
  });

  it('listMissionReports GETs /api/missions/{id}/reports', async () => {
    fetchMock.mockResolvedValue(jsonResponse([]));
    await api.listMissionReports('m-1');
    expect(new URL(lastCall().url).pathname).toBe('/api/missions/m-1/reports');
  });

  it('dispatchMission POSTs the fire-a-mission body to /api/fleet/dispatch', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ instanceId: 'inst-1', sessionId: 'sess-1', missionId: 'mission-1' }),
    );
    const req = {
      agentName: 'researcher',
      intent: 'Investigate the flaky test',
      hitlPolicyName: 'hitl-policy-dev.json',
    };
    await expect(api.dispatchMission(req)).resolves.toEqual({
      instanceId: 'inst-1',
      sessionId: 'sess-1',
      missionId: 'mission-1',
    });
    const { url, init } = lastCall();
    expect(new URL(url).pathname).toBe('/api/fleet/dispatch');
    expect(init.method).toBe('POST');
    expect(JSON.parse(String(init.body))).toEqual(req);
  });

  it('dispatchMission forwards an optional cwd verbatim', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ instanceId: 'inst-1', sessionId: 'sess-1', missionId: 'mission-1' }),
    );
    await api.dispatchMission({
      agentName: 'researcher',
      intent: 'Investigate',
      hitlPolicyName: 'hitl-policy-dev.json',
      cwd: '/workspace/repo',
    });
    const body = JSON.parse(String(lastCall().init.body));
    expect(body.cwd).toBe('/workspace/repo');
  });

  it('surfaces a 409 from a disabled agent as a rejected ApiError', async () => {
    fetchMock.mockResolvedValue(
      jsonResponse({ error: { message: 'agent is disabled' } }, false, 409),
    );
    await expect(
      api.dispatchMission({
        agentName: 'disabled-agent',
        intent: 'x',
        hitlPolicyName: 'hitl-policy-dev.json',
      }),
    ).rejects.toMatchObject({ status: 409 });
  });
});

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fleetKeys, missionKeys } from '../lib/queryKeys';
import { useCancelInstance, useDispatchMission, useStopInstance } from './useFleet';

vi.mock('../lib/api', () => ({
  api: {
    getFleet: vi.fn(),
    stopInstance: vi.fn(async () => 'deleted'),
    cancelInstance: vi.fn(async () => 'cancelled'),
    dispatchMission: vi.fn(async () => ({
      instanceId: 'inst-1',
      sessionId: 'sess-1',
      missionId: 'mission-1',
    })),
  },
}));

// Imported after the mock so the hooks see the doubles.
const { api } = await import('../lib/api');

/**
 * `@testing-library/react` is not a dependency of `packages/beam` (see
 * PermissionCard.test.tsx / acpWorkspaceController.test.ts). A mutation hook is
 * still drivable without a DOM: `renderToStaticMarkup` runs the component body,
 * so the `MutationObserver` react-query builds during render is real, and
 * `mutateAsync` on the captured result performs the request and runs
 * `onSuccess` — which is exactly the endpoint + invalidation contract under
 * test. Only the re-render of `isPending`/`error` is out of reach here.
 */
function mountHook<T>(client: QueryClient, hook: () => T): T {
  let captured: T | undefined;
  const Probe = (): ReactNode => {
    captured = hook();
    return null;
  };
  renderToStaticMarkup(createElement(QueryClientProvider, { client }, createElement(Probe)));
  if (captured === undefined) throw new Error('hook did not run');
  return captured;
}

function makeClient(): QueryClient {
  return new QueryClient({ defaultOptions: { mutations: { retry: false } } });
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe('useStopInstance', () => {
  it('DELETEs the instance by id and invalidates the fleet list', async () => {
    const client = makeClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    const stop = mountHook(client, () => useStopInstance());

    await expect(stop.mutateAsync('inst-1')).resolves.toBe('deleted');

    expect(api.stopInstance).toHaveBeenCalledTimes(1);
    expect(api.stopInstance).toHaveBeenCalledWith('inst-1');
    expect(invalidate).toHaveBeenCalledWith({ queryKey: fleetKeys.list() });
  });

  it('surfaces a failed stop instead of invalidating', async () => {
    const client = makeClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    vi.mocked(api.stopInstance).mockRejectedValueOnce(new Error('boom'));
    const stop = mountHook(client, () => useStopInstance());

    await expect(stop.mutateAsync('inst-1')).rejects.toThrow('boom');
    expect(invalidate).not.toHaveBeenCalled();
  });
});

describe('useCancelInstance', () => {
  it('cancels a single named session (per-session call shape)', async () => {
    const client = makeClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    const cancel = mountHook(client, () => useCancelInstance());

    await expect(cancel.mutateAsync({ instanceId: 'inst-1', sessionId: 'sess-a' })).resolves.toBe(
      'cancelled',
    );

    expect(api.cancelInstance).toHaveBeenCalledWith('inst-1', 'sess-a');
    expect(invalidate).toHaveBeenCalledWith({ queryKey: fleetKeys.list() });
  });

  it('omits the session id entirely for cancel-all — the server reads that as "every session"', async () => {
    const client = makeClient();
    const cancel = mountHook(client, () => useCancelInstance());

    await cancel.mutateAsync({ instanceId: 'inst-1' });

    expect(api.cancelInstance).toHaveBeenCalledWith('inst-1', undefined);
  });
});

describe('useDispatchMission', () => {
  it('fires the mission and invalidates BOTH the fleet list and the mission list', async () => {
    const client = makeClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    const dispatch = mountHook(client, () => useDispatchMission());
    const req = {
      agentName: 'researcher',
      intent: 'Investigate',
      hitlPolicyName: 'hitl-policy-dev.json',
    };

    await expect(dispatch.mutateAsync(req)).resolves.toEqual({
      instanceId: 'inst-1',
      sessionId: 'sess-1',
      missionId: 'mission-1',
    });

    expect(api.dispatchMission).toHaveBeenCalledExactlyOnceWith(req);
    expect(invalidate).toHaveBeenCalledWith({ queryKey: fleetKeys.list() });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: missionKeys.list() });
  });

  it('surfaces a failed dispatch instead of invalidating either list', async () => {
    const client = makeClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    vi.mocked(api.dispatchMission).mockRejectedValueOnce(new Error('agent is disabled'));
    const dispatch = mountHook(client, () => useDispatchMission());

    await expect(
      dispatch.mutateAsync({ agentName: 'x', intent: 'y', hitlPolicyName: 'z' }),
    ).rejects.toThrow('agent is disabled');
    expect(invalidate).not.toHaveBeenCalled();
  });
});

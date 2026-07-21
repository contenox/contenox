import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { approvalKeys, fleetKeys, missionKeys } from '../lib/queryKeys';
import { useAnswerApproval } from './useApprovals';

vi.mock('../lib/api', () => ({
  api: {
    listApprovals: vi.fn(async () => []),
    answerApproval: vi.fn(async () => 'approved'),
    listMissions: vi.fn(async () => []),
    listMissionReports: vi.fn(async () => []),
  },
}));

const { api } = await import('../lib/api');

/**
 * Same DOM-free harness as useFleet.test.tsx: `@testing-library/react` is not a
 * dependency of packages/beam, so a mutation hook is driven through
 * `renderToStaticMarkup` + `mutateAsync`, which exercises the real endpoint call
 * and `onSuccess` invalidation contract.
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

describe('useAnswerApproval', () => {
  it('POSTs the allow answer by id and invalidates the approvals, fleet, and mission lists', async () => {
    const client = makeClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    const answer = mountHook(client, () => useAnswerApproval());

    await expect(answer.mutateAsync({ id: 'ask-1', approved: true })).resolves.toBe('approved');

    expect(api.answerApproval).toHaveBeenCalledExactlyOnceWith('ask-1', true);
    // Answering can unblock the unit, so all three surfaces are refreshed.
    expect(invalidate).toHaveBeenCalledWith({ queryKey: approvalKeys.list() });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: fleetKeys.list() });
    expect(invalidate).toHaveBeenCalledWith({ queryKey: missionKeys.list() });
  });

  it('sends the deny answer (approved:false) unchanged', async () => {
    const client = makeClient();
    const answer = mountHook(client, () => useAnswerApproval());

    await answer.mutateAsync({ id: 'ask-2', approved: false });

    expect(api.answerApproval).toHaveBeenCalledExactlyOnceWith('ask-2', false);
  });

  it('surfaces a failed answer instead of invalidating anything', async () => {
    const client = makeClient();
    const invalidate = vi.spyOn(client, 'invalidateQueries');
    vi.mocked(api.answerApproval).mockRejectedValueOnce(new Error('already answered'));
    const answer = mountHook(client, () => useAnswerApproval());

    await expect(answer.mutateAsync({ id: 'ask-3', approved: true })).rejects.toThrow(
      'already answered',
    );
    expect(invalidate).not.toHaveBeenCalled();
  });
});

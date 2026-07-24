import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement, type ReactNode } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { workspaceAccessKeys } from '../lib/queryKeys';
import { dedupeSortPaths, type AccessEvaluateResponse } from '../lib/workspaceAccess';
import { useWorkspaceAccess, type WorkspaceAccessResult } from './useWorkspaceAccess';

/**
 * Same DOM-free harness as useApprovals.test.tsx: `@testing-library/react` is not
 * a dependency, so the hook is read once through `renderToStaticMarkup` + a probe.
 * A query hook is exercised by SEEDING the react-query cache (setQueryData) under
 * the exact key the hook builds, then asserting the hook projects that cached
 * batch onto its `path → verdict` map — which also pins the cache KEY (root +
 * policy + sorted-path union), the lazy-union contract.
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
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

const RESPONSE: AccessEvaluateResponse = {
  policyName: 'hitl-policy-strict.json',
  verdicts: [
    { path: 'README.md', reachable: true, read: { action: 'allow' }, write: { action: 'approve', reason: 'default_action' } },
    { path: '.ssh/id_rsa', reachable: true, read: { action: 'deny', reason: 'matched_rule', rule: 0 }, write: { action: 'deny', reason: 'matched_rule', rule: 0 } },
    { path: '../escape', reachable: false },
  ],
};

/** Seeds the cache under the key the hook derives for (root, policyKey, sorted paths). */
function seed(client: QueryClient, root: string, policyKey: string, paths: string[], data = RESPONSE) {
  const pathsKey = dedupeSortPaths(paths).join('\n');
  client.setQueryData(workspaceAccessKeys.batch(root, policyKey, pathsKey), data);
}

describe('useWorkspaceAccess', () => {
  const PATHS = ['README.md', '.ssh/id_rsa', '../escape'];

  it('projects the cached batch onto a path→verdict map (mapping + unreachable)', () => {
    const client = makeClient();
    seed(client, '/ws', 'strict', PATHS);

    const res = mountHook<WorkspaceAccessResult>(client, () =>
      useWorkspaceAccess({ root: '/ws', policy: 'strict', paths: PATHS, enabled: true }),
    );

    expect(res.policyName).toBe('hitl-policy-strict.json');
    expect(res.verdicts.size).toBe(3);
    expect(res.verdicts.get('README.md')?.read?.action).toBe('allow');
    expect(res.verdicts.get('README.md')?.write?.action).toBe('approve');
    expect(res.verdicts.get('.ssh/id_rsa')?.read?.action).toBe('deny');
    // Unreachable path flows through with no read/write verdict.
    expect(res.verdicts.get('../escape')?.reachable).toBe(false);
    expect(res.verdicts.get('../escape')?.read).toBeUndefined();
  });

  it('re-keys on the grown union: a superset of paths misses the smaller cached batch', () => {
    const client = makeClient();
    // Seed only the first two paths (the pre-expand union) with their own batch.
    const twoPathBatch: AccessEvaluateResponse = {
      policyName: 'hitl-policy-strict.json',
      verdicts: RESPONSE.verdicts.slice(0, 2),
    };
    seed(client, '/ws', 'strict', ['README.md', '.ssh/id_rsa'], twoPathBatch);

    // The tree expands a directory → the loaded union grows. The larger union is a
    // DIFFERENT key, so nothing is cached for it yet → empty until its batch runs.
    const res = mountHook<WorkspaceAccessResult>(client, () =>
      useWorkspaceAccess({ root: '/ws', policy: 'strict', paths: PATHS, enabled: true }),
    );
    expect(res.verdicts.size).toBe(0);

    // The pre-expand union still resolves from cache (order-independent key).
    const before = mountHook<WorkspaceAccessResult>(client, () =>
      useWorkspaceAccess({ root: '/ws', policy: 'strict', paths: ['.ssh/id_rsa', 'README.md'], enabled: true }),
    );
    expect(before.verdicts.size).toBe(2);
  });

  it('reports an empty map when disabled (agent view off), even with a warm cache', () => {
    const client = makeClient();
    seed(client, '/ws', 'strict', PATHS);

    const res = mountHook<WorkspaceAccessResult>(client, () =>
      useWorkspaceAccess({ root: '/ws', policy: 'strict', paths: PATHS, enabled: false }),
    );
    expect(res.verdicts.size).toBe(0);
    expect(res.policyName).toBeUndefined();
  });

  it('is disabled (empty map) when there are no paths to evaluate', () => {
    const client = makeClient();
    const res = mountHook<WorkspaceAccessResult>(client, () =>
      useWorkspaceAccess({ root: '/ws', policy: 'strict', paths: [], enabled: true }),
    );
    expect(res.verdicts.size).toBe(0);
  });
});

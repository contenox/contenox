import { describe, expect, it } from 'vitest';
import { filesUrl, HITL_POLICY_DEFAULT_VALUE } from './useWorkspaceFiles';
import { ROOT_DIR } from '../pages/chat/lib/workspaceTree';

/**
 * Covers the pure `filesUrl` builder behind `useWorkspaceFiles`. The React hook
 * itself needs a DOM renderer this package's test env doesn't provide (see
 * usePersistentToggle.test.ts), so the policy-threading contract is pinned here
 * on the pure URL builder — which is exactly the cache key the hook keys its
 * refetch on, so a distinct URL per policy is also what forces the reload.
 */
describe('filesUrl', () => {
  const parse = (url: string) => new URLSearchParams(url.split('?')[1]);

  it('omits filter and policy when agent-view is off (raw tree, backward compatible)', () => {
    const params = parse(filesUrl(ROOT_DIR, '/ws', false, 'strict'));
    expect(params.get('filter')).toBeNull();
    expect(params.get('policy')).toBeNull();
    expect(params.get('path')).toBe('.');
    expect(params.get('root')).toBe('/ws');
  });

  it('sets filter=agent but omits policy for the Default sentinel (server default-resolves)', () => {
    const params = parse(filesUrl(ROOT_DIR, '/ws', true, HITL_POLICY_DEFAULT_VALUE));
    expect(params.get('filter')).toBe('agent');
    expect(params.get('policy')).toBeNull();
  });

  it('omits policy under agent-view when no policy value is supplied', () => {
    expect(parse(filesUrl('src', '/ws', true)).get('policy')).toBeNull();
    expect(parse(filesUrl('src', '/ws', true, null)).get('policy')).toBeNull();
    expect(parse(filesUrl('src', '/ws', true, '   ')).get('policy')).toBeNull();
  });

  it('appends policy=<name> under agent-view for a concrete policy selection', () => {
    const params = parse(filesUrl('src', '/ws', true, 'hitl-policy-strict.json'));
    expect(params.get('filter')).toBe('agent');
    expect(params.get('policy')).toBe('hitl-policy-strict.json');
    expect(params.get('path')).toBe('src');
  });

  it('produces a distinct URL per policy so changing it invalidates the hook cache/refetches', () => {
    const strict = filesUrl('src', '/ws', true, 'strict');
    const dev = filesUrl('src', '/ws', true, 'dev');
    const def = filesUrl('src', '/ws', true, HITL_POLICY_DEFAULT_VALUE);
    expect(strict).not.toBe(dev);
    expect(strict).not.toBe(def);
    expect(dev).not.toBe(def);
  });
});

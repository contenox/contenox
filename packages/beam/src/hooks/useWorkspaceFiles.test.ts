import { describe, expect, it } from 'vitest';
import { filesUrl } from './useWorkspaceFiles';
import { ROOT_DIR } from '../pages/chat/lib/workspaceTree';

/**
 * Covers the pure `filesUrl` builder behind `useWorkspaceFiles`. The React hook
 * itself needs a DOM renderer this package's test env doesn't provide (see
 * usePersistentToggle.test.ts), so the URL contract is pinned here on the pure
 * builder. `/files` is now a RAW listing — the agent-view verdict overlay moved
 * to a separate `POST /workspace/access` batch, so the listing URL no longer
 * carries `filter=agent` or `policy` (that contract lives in workspaceAccess.test).
 */
describe('filesUrl', () => {
  const parse = (url: string) => new URLSearchParams(url.split('?')[1]);

  it('builds a raw root listing URL (no filter, no policy — byte-identical under any overlay)', () => {
    const params = parse(filesUrl(ROOT_DIR, '/ws'));
    expect(params.get('path')).toBe('.');
    expect(params.get('root')).toBe('/ws');
    expect(params.get('filter')).toBeNull();
    expect(params.get('policy')).toBeNull();
  });

  it('passes a concrete directory path through unchanged', () => {
    const params = parse(filesUrl('src/lib', '/ws'));
    expect(params.get('path')).toBe('src/lib');
    expect(params.get('root')).toBe('/ws');
  });
});

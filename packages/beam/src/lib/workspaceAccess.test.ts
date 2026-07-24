import { beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('./fetch', () => ({
  apiFetch: vi.fn(),
}));

const { apiFetch } = await import('./fetch');
import {
  accessUrl,
  dedupeSortPaths,
  fetchWorkspaceAccess,
  verdictsToMap,
  HITL_POLICY_DEFAULT_VALUE,
  type AccessEvaluateResponse,
  type PathVerdict,
} from './workspaceAccess';

const parse = (url: string) => new URLSearchParams(url.split('?')[1]);

beforeEach(() => {
  vi.clearAllMocks();
});

describe('accessUrl', () => {
  it('pins policy=<name> for a concrete selection', () => {
    const params = parse(accessUrl('/ws', 'hitl-policy-strict.json'));
    expect(params.get('root')).toBe('/ws');
    expect(params.get('policy')).toBe('hitl-policy-strict.json');
  });

  it('omits policy for the Default sentinel, nullish, and whitespace (server default-resolves)', () => {
    expect(parse(accessUrl('/ws', HITL_POLICY_DEFAULT_VALUE)).get('policy')).toBeNull();
    expect(parse(accessUrl('/ws')).get('policy')).toBeNull();
    expect(parse(accessUrl('/ws', null)).get('policy')).toBeNull();
    expect(parse(accessUrl('/ws', '   ')).get('policy')).toBeNull();
  });
});

describe('dedupeSortPaths', () => {
  it('de-dupes and sorts so the batch key is order-independent (expands do not thrash)', () => {
    expect(dedupeSortPaths(['b', 'a', 'a', 'c'])).toEqual(['a', 'b', 'c']);
    // Same set in a different order → identical key.
    expect(dedupeSortPaths(['c', 'a', 'b']).join('\n')).toBe(dedupeSortPaths(['a', 'b', 'c']).join('\n'));
  });

  it('grows the union as the lazy tree expands (superset re-keys)', () => {
    const before = dedupeSortPaths(['src', 'README.md']).join('\n');
    const after = dedupeSortPaths(['src', 'README.md', 'src/app.ts']).join('\n');
    expect(after).not.toBe(before);
    expect(after.split('\n')).toContain('src/app.ts');
  });
});

describe('verdictsToMap', () => {
  it('projects the ordered verdict list into a path lookup, unreachable included', () => {
    const verdicts: PathVerdict[] = [
      { path: 'src/main.go', reachable: true, read: { action: 'allow' }, write: { action: 'approve' } },
      { path: '../escape', reachable: false },
    ];
    const map = verdictsToMap(verdicts);
    expect(map.get('src/main.go')?.read?.action).toBe('allow');
    expect(map.get('src/main.go')?.write?.action).toBe('approve');
    expect(map.get('../escape')?.reachable).toBe(false);
    expect(map.get('../escape')?.read).toBeUndefined();
  });
});

describe('fetchWorkspaceAccess', () => {
  it('POSTs the paths body to the access URL and returns the response', async () => {
    const response: AccessEvaluateResponse = {
      policyName: 'hitl-policy-strict.json',
      verdicts: [{ path: 'a.txt', reachable: true, read: { action: 'allow' }, write: { action: 'deny', reason: 'matched_rule', rule: 0 } }],
    };
    vi.mocked(apiFetch).mockResolvedValueOnce(response);

    const out = await fetchWorkspaceAccess('/ws', 'strict', ['a.txt', 'b.txt']);

    expect(out).toBe(response);
    expect(apiFetch).toHaveBeenCalledTimes(1);
    const [url, init] = vi.mocked(apiFetch).mock.calls[0];
    expect(parse(url).get('root')).toBe('/ws');
    expect(parse(url).get('policy')).toBe('strict');
    expect(init?.method).toBe('POST');
    expect(JSON.parse(init?.body as string)).toEqual({ paths: ['a.txt', 'b.txt'] });
  });
});

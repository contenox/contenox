import type { TFunction } from 'i18next';
import { describe, expect, it, vi } from 'vitest';
import type { SessionInfo } from '../acp';
import type { Agent, FleetEntry, HITLApproval, Mission, WorkspaceRoot } from '../types';
import {
  actionsProvider,
  agentsProvider,
  buildPaletteItems,
  fleetProvider,
  inboxProvider,
  missionsProvider,
  sessionsProvider,
  workspaceProvider,
} from './providers';
import type { PaletteProviderContext } from './types';

// A t() stub that echoes the key (interpolating {{shortId}} when present) — the
// provider mapping is what is under test, not the German copy.
const t = ((key: string, opts?: { shortId?: string }) =>
  opts?.shortId ? `${key}:${opts.shortId}` : key) as unknown as TFunction;

function ctx(over: Partial<PaletteProviderContext>): PaletteProviderContext {
  return {
    t,
    navigate: vi.fn(),
    startAdopt: vi.fn(),
    missions: [],
    fleet: [],
    agents: [],
    approvals: [],
    sessions: [],
    workspaceRoots: [],
    ...over,
  };
}

const mission = (over: Partial<Mission> & { id: string }): Mission => ({
  intent: 'Investigate the flaky nightly test',
  agentName: 'Researcher',
  hitlPolicyName: 'hitl-dev.json',
  status: 'open',
  createdAt: '2026-07-21T00:00:00Z',
  updatedAt: '2026-07-21T00:00:00Z',
  ...over,
});

describe('missionsProvider', () => {
  it('maps a mission to a mission item that opens its detail', () => {
    const navigate = vi.fn();
    const [item] = missionsProvider(ctx({ navigate, missions: [mission({ id: 'm1' })] }));
    expect(item.id).toBe('mission:m1');
    expect(item.type).toBe('mission');
    expect(item.title).toBe('Investigate the flaky nightly test');
    expect(item.keywords).toContain('Researcher');
    item.action();
    expect(navigate).toHaveBeenCalledWith('/missions/m1');
  });
});

describe('fleetProvider', () => {
  const entry = (instances: FleetEntry['instances']): FleetEntry => ({
    agentId: 'a1',
    agentName: 'Researcher',
    kind: 'external',
    instances,
  });

  it('adopts the running instance session', () => {
    const startAdopt = vi.fn();
    const items = fleetProvider(
      ctx({
        startAdopt,
        fleet: [
          entry([
            {
              id: 'inst-1',
              agentId: 'a1',
              agentName: 'Researcher',
              kind: 'external',
              state: 'running',
              sessions: 1,
              viewers: 0,
              startedAt: '2026-07-21T00:00:00Z',
              sessionIds: ['sess-1'],
            },
          ]),
        ],
      }),
    );
    expect(items).toHaveLength(1);
    expect(items[0].id).toBe('fleet:inst-1');
    items[0].action();
    expect(startAdopt).toHaveBeenCalledWith({ instanceId: 'inst-1', sessionId: 'sess-1' });
  });

  it('falls back to the board for a non-running instance', () => {
    const navigate = vi.fn();
    const startAdopt = vi.fn();
    const items = fleetProvider(
      ctx({
        navigate,
        startAdopt,
        fleet: [
          entry([
            {
              id: 'inst-2',
              agentId: 'a1',
              agentName: 'Researcher',
              kind: 'external',
              state: 'error',
              sessions: 0,
              viewers: 0,
              startedAt: '2026-07-21T00:00:00Z',
              sessionIds: [],
            },
          ]),
        ],
      }),
    );
    items[0].action();
    expect(navigate).toHaveBeenCalledWith('/fleet');
    expect(startAdopt).not.toHaveBeenCalled();
  });

  it('yields nothing for an idle (instance-less) agent', () => {
    expect(fleetProvider(ctx({ fleet: [entry(null)] }))).toEqual([]);
  });
});

describe('agentsProvider', () => {
  const agent = (over: Partial<Agent> & { id: string; name: string }): Agent => ({
    kind: 'external',
    enabled: true,
    configJson: {},
    createdAt: '2026-07-21T00:00:00Z',
    updatedAt: '2026-07-21T00:00:00Z',
    ...over,
  });

  it('offers only enabled agents and prefills the dispatch form', () => {
    const navigate = vi.fn();
    const items = agentsProvider(
      ctx({
        navigate,
        agents: [
          agent({ id: 'a1', name: 'Researcher' }),
          agent({ id: 'a2', name: 'Disabled One', enabled: false }),
        ],
      }),
    );
    expect(items.map(i => i.id)).toEqual(['agent:a1']);
    items[0].action();
    expect(navigate).toHaveBeenCalledWith('/missions/new?agent=Researcher');
  });
});

describe('sessionsProvider', () => {
  it('falls back to a short-id title when the session has no meaningful one', () => {
    const navigate = vi.fn();
    const session: SessionInfo = { sessionId: 'abcdef123456', title: 'abcdef123456' };
    const [item] = sessionsProvider(ctx({ navigate, sessions: [session] }));
    expect(item.type).toBe('session');
    expect(item.title).toBe('palette.session_fallback:abcdef12');
    item.action();
    expect(navigate).toHaveBeenCalledWith('/chat/abcdef123456');
  });
});

describe('workspaceProvider', () => {
  it('yields nothing when the roots snapshot is empty (absent API)', () => {
    expect(workspaceProvider(ctx({ workspaceRoots: [] }))).toEqual([]);
  });

  it('maps a root, marking the default', () => {
    const root: WorkspaceRoot = { path: '/srv/project', default: true };
    const [item] = workspaceProvider(ctx({ workspaceRoots: [root] }));
    expect(item.id).toBe('workspace:/srv/project');
    expect(item.subtitle).toBe('palette.workspace_default');
  });
});

describe('inboxProvider', () => {
  it('maps a pending ask to an item that opens the inbox', () => {
    const navigate = vi.fn();
    const approval: HITLApproval = {
      id: 'ap1',
      toolsName: 'local_fs',
      toolName: 'write_file',
      state: 'pending',
      agentName: 'Researcher',
      createdAt: '2026-07-21T00:00:00Z',
      expiresAt: '2026-07-21T01:00:00Z',
    };
    const [item] = inboxProvider(ctx({ navigate, approvals: [approval] }));
    expect(item.id).toBe('inbox:ap1');
    expect(item.title).toBe('local_fs / write_file');
    item.action();
    expect(navigate).toHaveBeenCalledWith('/inbox');
  });
});

describe('actionsProvider + buildPaletteItems', () => {
  it('always offers fire-a-mission and every nav target', () => {
    const ids = actionsProvider(ctx({})).map(i => i.id);
    expect(ids).toContain('action:new-mission');
    expect(ids).toContain('action:new-chat');
    expect(ids).toContain('nav:/fleet');
    expect(ids).toContain('nav:/projects');
    expect(ids).toContain('nav:/settings');
  });

  it('navigates to the projects page from its nav target', () => {
    const navigate = vi.fn();
    const item = actionsProvider(ctx({ navigate })).find(i => i.id === 'nav:/projects')!;
    item.action();
    expect(navigate).toHaveBeenCalledWith('/projects');
  });

  it('fire-a-mission navigates to the dispatch form', () => {
    const navigate = vi.fn();
    const item = actionsProvider(ctx({ navigate })).find(i => i.id === 'action:new-mission')!;
    item.action();
    expect(navigate).toHaveBeenCalledWith('/missions/new');
  });

  it('concatenates every provider', () => {
    const items = buildPaletteItems(
      ctx({
        missions: [mission({ id: 'm1' })],
        approvals: [
          {
            id: 'ap1',
            toolsName: 'fs',
            toolName: 'write',
            state: 'pending',
            createdAt: '2026-07-21T00:00:00Z',
            expiresAt: '2026-07-21T01:00:00Z',
          },
        ],
      }),
    );
    // actions (9) + inbox (1) + mission (1)
    expect(items.filter(i => i.type === 'mission')).toHaveLength(1);
    expect(items.filter(i => i.type === 'inbox')).toHaveLength(1);
    expect(items.filter(i => i.type === 'action').length).toBeGreaterThanOrEqual(8);
  });
});

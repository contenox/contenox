import { describe, expect, it } from 'vitest';
import type { MissionReport } from './types';
import { blockedMissionIds, composeUnitStatus } from './unitStatus';

describe('composeUnitStatus — the one composed truth', () => {
  it('says nothing when there is nothing to say', () => {
    expect(composeUnitStatus({})).toEqual([]);
  });

  it('renders a lone process state (an instance with no mission) as a single atom', () => {
    const atoms = composeUnitStatus({ instanceState: 'running' });
    expect(atoms).toHaveLength(1);
    expect(atoms[0]).toMatchObject({ kind: 'process', labelKey: 'fleet.state.running' });
  });

  it('orders the atoms loudest-first: blocked, process, verdict, liveness', () => {
    const atoms = composeUnitStatus({
      instanceState: 'running',
      missionStatus: 'open',
      lastHeartbeat: undefined,
      blocked: true,
    });
    expect(atoms.map(a => a.kind)).toEqual(['blocked', 'process', 'verdict', 'liveness']);
  });

  it('frames an open mission as "no result yet" (neutral), never as a health warning', () => {
    const [verdict] = composeUnitStatus({ missionStatus: 'open' });
    // Neutral outline badge, and a label that reads as "no verdict", not an alarm.
    expect(verdict).toMatchObject({
      kind: 'verdict',
      variant: 'outline',
      labelKey: 'unit.verdict_open',
    });
  });

  it('gives each terminal verdict its own tone — landed good, derailed alert', () => {
    expect(composeUnitStatus({ missionStatus: 'landed' })[0]).toMatchObject({ variant: 'success' });
    expect(composeUnitStatus({ missionStatus: 'derailed' })[0]).toMatchObject({ variant: 'error' });
  });

  it('separates stuck from derailed: attention (warning), not a verdict (error)', () => {
    // The runtime keeps these distinct on purpose — stuck asks for a nudge, a
    // derailment asks for a post-mortem — so the chips must not read alike.
    expect(composeUnitStatus({ missionStatus: 'stuck' })[0]).toMatchObject({
      kind: 'verdict',
      variant: 'warning',
      labelKey: 'unit.verdict_stuck',
    });
  });

  it('survives a mission status this build does not know, echoing the runtime word', () => {
    // REGRESSION: `stuck` shipped in the runtime while this map had four entries,
    // and the undefined lookup threw during render — the whole missions page fell
    // to the error boundary ("can't access property labelKey"). The Go status set
    // is contracted to only GROW, and a runtime is routinely newer than the Beam
    // served beside it, so an unknown value must degrade, never throw.
    const atoms = composeUnitStatus({
      missionStatus: 'quarantined' as never,
      instanceState: 'running',
    });
    const verdict = atoms.find(a => a.kind === 'verdict');
    expect(verdict).toMatchObject({ variant: 'secondary', label: 'quarantined' });
    expect(verdict?.labelKey).toBeUndefined();
    // and the atoms around it still compose normally
    expect(atoms.map(a => a.kind)).toEqual(['process', 'verdict', 'liveness']);
  });

  it('renders liveness as the heartbeat when present, and honestly as "never" when absent', () => {
    const withHeartbeat = composeUnitStatus({
      missionStatus: 'open',
      lastHeartbeat: '2026-07-21T09:00:00Z',
    });
    const liveness = withHeartbeat.find(a => a.kind === 'liveness');
    expect(liveness?.heartbeat).toBe('2026-07-21T09:00:00Z');
    expect(liveness?.labelKey).toBeUndefined();

    const never = composeUnitStatus({ missionStatus: 'open' }).find(a => a.kind === 'liveness');
    expect(never?.heartbeat).toBeUndefined();
    expect(never?.labelKey).toBe('missions.heartbeat_never');
  });

  it('does not invent a liveness atom for a unit that is not on a mission', () => {
    expect(composeUnitStatus({ instanceState: 'running' }).some(a => a.kind === 'liveness')).toBe(
      false,
    );
  });
});

describe('blockedMissionIds — a mission is blocked when its LATEST report is a blocker', () => {
  const report = (
    over: Partial<MissionReport> & { id: string; missionId: string },
  ): MissionReport => ({
    kind: 'progress',
    summary: 's',
    createdAt: new Date().toISOString(),
    ...over,
  });

  it('includes a mission whose newest report is a blocker', () => {
    const set = blockedMissionIds([
      report({ id: 'r1', missionId: 'm1', kind: 'blocker', createdAt: '2026-07-21T10:00:00Z' }),
    ]);
    expect(set.has('m1')).toBe(true);
  });

  it('clears a mission once a later non-blocker report supersedes the blocker', () => {
    const set = blockedMissionIds([
      report({ id: 'r1', missionId: 'm1', kind: 'blocker', createdAt: '2026-07-21T10:00:00Z' }),
      report({ id: 'r2', missionId: 'm1', kind: 'progress', createdAt: '2026-07-21T11:00:00Z' }),
    ]);
    expect(set.has('m1')).toBe(false);
  });

  it('scopes blocked-ness per mission', () => {
    const set = blockedMissionIds([
      report({ id: 'r1', missionId: 'm1', kind: 'blocker', createdAt: '2026-07-21T10:00:00Z' }),
      report({ id: 'r2', missionId: 'm2', kind: 'result', createdAt: '2026-07-21T10:00:00Z' }),
    ]);
    expect([...set]).toEqual(['m1']);
  });

  it('tolerates an unparseable timestamp without throwing', () => {
    expect(() =>
      blockedMissionIds([
        report({ id: 'r1', missionId: 'm1', kind: 'blocker', createdAt: 'not-a-date' }),
      ]),
    ).not.toThrow();
  });
});

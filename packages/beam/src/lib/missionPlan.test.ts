import { describe, expect, it } from 'vitest';
import { hasPlan, planProgress } from './missionPlan';
import type { MissionPlan, MissionPlanEntry } from './types';

function entry(over: Partial<MissionPlanEntry> & { id: string }): MissionPlanEntry {
  return { content: 'do a thing', status: 'pending', priority: 'medium', ...over };
}

function plan(entries: MissionPlanEntry[] | null, over: Partial<MissionPlan> = {}): MissionPlan {
  return { entries, revision: 1, ...over };
}

describe('planProgress — the one shared progress helper', () => {
  it('is null for no plan at all (an older serve omits the field)', () => {
    expect(planProgress(undefined)).toBeNull();
    expect(planProgress(null)).toBeNull();
  });

  it('is null for the zero Plan a never-planned mission carries (null or empty entries)', () => {
    // Go marshals a nil slice as null; a fresh Plan is also revision 0 empty.
    expect(planProgress(plan(null, { revision: 0 }))).toBeNull();
    expect(planProgress(plan([], { revision: 0 }))).toBeNull();
  });

  it('counts completed and total, and reports in-progress separately', () => {
    const p = plan([
      entry({ id: '1', status: 'completed' }),
      entry({ id: '2', status: 'completed' }),
      entry({ id: '3', status: 'in_progress' }),
      entry({ id: '4', status: 'pending' }),
    ]);
    expect(planProgress(p)).toEqual({ completed: 2, total: 4, inProgress: 1 });
  });

  it('reports zero in-progress when nothing is mid-flight (so the marker is hidden)', () => {
    const p = plan([entry({ id: '1', status: 'pending' }), entry({ id: '2', status: 'completed' })]);
    expect(planProgress(p)).toEqual({ completed: 1, total: 2, inProgress: 0 });
  });

  it('hasPlan mirrors planProgress: true only when there is something to render', () => {
    expect(hasPlan(plan([entry({ id: '1' })]))).toBe(true);
    expect(hasPlan(plan([]))).toBe(false);
    expect(hasPlan(undefined)).toBe(false);
  });
});

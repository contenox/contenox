import { describe, expect, it } from 'vitest';
import { MISSION_META_KEY, missionFromMeta } from './acp';

/**
 * `missionFromMeta` is what lets the session rail tell a fleet MISSION UNIT
 * apart from a chat the operator started: both are created in the same
 * workspace under the same identity, and a unit — which has real messages while
 * the session that fired it may have none — sorts above them. Reading the
 * attribution wrong in either direction is a user-visible lie, so both
 * directions are pinned here.
 */
describe('missionFromMeta', () => {
  it('reads the mission id from a unit session entry', () => {
    expect(missionFromMeta({ [MISSION_META_KEY]: { missionId: 'm-42' } })).toBe('m-42');
  });

  it('returns null for an ordinary chat session (no attribution at all)', () => {
    expect(missionFromMeta(undefined)).toBeNull();
    expect(missionFromMeta(null)).toBeNull();
    expect(missionFromMeta({})).toBeNull();
    expect(missionFromMeta({ 'contenox.agent': 'claude' })).toBeNull();
  });

  it('tolerates malformed shapes rather than throwing', () => {
    // `_meta` is an open envelope other producers write into, so a wrong shape
    // must read as "not a mission", never as a crash in the sidebar.
    expect(missionFromMeta({ [MISSION_META_KEY]: 'm-42' })).toBeNull();
    expect(missionFromMeta({ [MISSION_META_KEY]: null })).toBeNull();
    expect(missionFromMeta({ [MISSION_META_KEY]: { missionId: '' } })).toBeNull();
    expect(missionFromMeta({ [MISSION_META_KEY]: { missionId: 7 } })).toBeNull();
  });

  it('trims a padded id, matching the agent-name helper beside it', () => {
    expect(missionFromMeta({ [MISSION_META_KEY]: { missionId: '  m-7  ' } })).toBe('m-7');
  });
});

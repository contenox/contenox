import { describe, expect, it } from 'vitest';
import { MISSION_ASK_META_KEY, missionAskFromMeta } from './acp';

/**
 * A delivered question is what unblocks a parked unit, so misreading its `_meta`
 * has two bad failure modes: dropping the answer box (the unit waits for a human
 * who never sees the ask), or rendering one for a message that is not a question
 * (an answer with no ask to resolve). Both directions are pinned.
 */
describe('missionAskFromMeta', () => {
  it('reads a delivered question, trimming the handle', () => {
    const ask = missionAskFromMeta({
      [MISSION_ASK_META_KEY]: {
        askId: ' ask-1 ',
        missionId: 'm-1',
        agentName: 'chain-acp',
        summary: 'which project?',
        detail: 'the intent named none',
      },
    });
    expect(ask).toEqual({
      askId: 'ask-1',
      missionId: 'm-1',
      agentName: 'chain-acp',
      intent: undefined,
      summary: 'which project?',
      detail: 'the intent named none',
    });
  });

  it('is null for an ordinary message', () => {
    expect(missionAskFromMeta(undefined)).toBeNull();
    expect(missionAskFromMeta({})).toBeNull();
    expect(missionAskFromMeta({ 'contenox.missionReport': { missionId: 'm-1' } })).toBeNull();
  });

  it('refuses a question with no handle or no text — there would be nothing to answer', () => {
    expect(missionAskFromMeta({ [MISSION_ASK_META_KEY]: { summary: 'no id' } })).toBeNull();
    expect(
      missionAskFromMeta({ [MISSION_ASK_META_KEY]: { askId: 'a', summary: '  ' } }),
    ).toBeNull();
    expect(missionAskFromMeta({ [MISSION_ASK_META_KEY]: 'not an object' })).toBeNull();
  });
});

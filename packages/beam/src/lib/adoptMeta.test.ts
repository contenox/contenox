import { describe, expect, it } from 'vitest';
import { ADOPT_META_KEY, adoptMeta, adoptResultFromMeta } from './adoptMeta';

describe('adoptMeta — the request wire shape', () => {
  it('builds the { "contenox.adopt": { instanceId, sessionId } } session/new _meta', () => {
    expect(adoptMeta('inst-1', 'down-1')).toEqual({
      'contenox.adopt': { instanceId: 'inst-1', sessionId: 'down-1' },
    });
  });

  it('round-trips through adoptResultFromMeta once the server echoes a controller flag', () => {
    // The server echoes the same ids plus `controller` (see acpsvc/adopt.go's
    // adoptedSessionMetaJSON). Simulate that echo and confirm the decode reads it.
    const request = adoptMeta('inst-1', 'down-1');
    const ref = (request[ADOPT_META_KEY] as { instanceId: string; sessionId: string });
    const echo = { [ADOPT_META_KEY]: { ...ref, controller: true } };
    expect(adoptResultFromMeta(echo)).toEqual({
      instanceId: 'inst-1',
      sessionId: 'down-1',
      controller: true,
    });
  });
});

describe('adoptResultFromMeta — defensive, mirroring the server parseAdoptResultMeta', () => {
  it('reads a full adopt outcome, controller true and false', () => {
    expect(
      adoptResultFromMeta({ 'contenox.adopt': { instanceId: 'i', sessionId: 's', controller: true } }),
    ).toEqual({ instanceId: 'i', sessionId: 's', controller: true });
    expect(
      adoptResultFromMeta({ 'contenox.adopt': { instanceId: 'i', sessionId: 's', controller: false } }),
    ).toEqual({ instanceId: 'i', sessionId: 's', controller: false });
  });

  it('treats a missing controller as observer (false), never assuming control', () => {
    expect(adoptResultFromMeta({ 'contenox.adopt': { instanceId: 'i', sessionId: 's' } })).toEqual({
      instanceId: 'i',
      sessionId: 's',
      controller: false,
    });
  });

  it('is null for a native or older-serve session that carries no adopt key', () => {
    expect(adoptResultFromMeta(undefined)).toBeNull();
    expect(adoptResultFromMeta(null)).toBeNull();
    expect(adoptResultFromMeta({})).toBeNull();
    // An external (contenox.agent) session that was NOT adopted stays null.
    expect(adoptResultFromMeta({ 'contenox.agent': 'stub-bot' })).toBeNull();
  });

  it('is null for a malformed / wrong-shaped value rather than throwing', () => {
    expect(adoptResultFromMeta({ 'contenox.adopt': null })).toBeNull();
    expect(adoptResultFromMeta({ 'contenox.adopt': 'nope' })).toBeNull();
    expect(adoptResultFromMeta({ 'contenox.adopt': 42 })).toBeNull();
    // Present but with a blank/absent id — an adopt needs both ids to be real.
    expect(adoptResultFromMeta({ 'contenox.adopt': { instanceId: '  ', sessionId: 's' } })).toBeNull();
    expect(adoptResultFromMeta({ 'contenox.adopt': { sessionId: 's', controller: true } })).toBeNull();
  });
});

describe('the sidebar delete-guard predicate', () => {
  // The sidebar withholds session/delete for adopted sessions (deleting one
  // STOPS the running dispatch — acpsvc/adopt.go). Its decision is exactly
  // `adoptResultFromMeta(session._meta) != null`, pinned here at that seam.
  const isAdopted = (meta: Record<string, unknown> | undefined) => adoptResultFromMeta(meta) != null;

  it('recognizes an adopted session (delete must be hidden)', () => {
    expect(isAdopted({ 'contenox.adopt': { instanceId: 'i', sessionId: 's', controller: true } })).toBe(true);
  });

  it('leaves native and external-only sessions deletable', () => {
    expect(isAdopted(undefined)).toBe(false);
    expect(isAdopted({ 'contenox.agent': 'stub-bot' })).toBe(false);
  });
});

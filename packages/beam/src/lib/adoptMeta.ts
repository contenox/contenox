/**
 * The client half of the fleet ADOPT wire contract — binding an
 * already-running instance + downstream session (a fleet dispatch left
 * unwatched) to a NEW upstream chat session, so an operator can watch and steer
 * it from the normal chat surface. Mirrors `acpsvc/adopt.go` field-for-field and
 * is the single client definition of the shape; keeping the encode here and the
 * server's `parseAdoptMeta`/`adoptedSessionMetaJSON` provably symmetrical is the
 * whole reason this lives in one small module.
 *
 * # Request  (session/new `_meta`)
 *
 *   { "contenox.adopt": { "instanceId": "<id>", "sessionId": "<downstream acp session id>" } }
 *
 * A conformant agent that does not recognize the key ignores `_meta` entirely
 * (an older serve routes it as an ordinary native session/new), which is why the
 * decode below is fail-soft rather than throwing.
 *
 * # Response  (session/new response `_meta`, alongside `contenox.agent`)
 *
 *   { "contenox.adopt": { "instanceId": ..., "sessionId": ..., "controller": <bool> } }
 *
 * `controller` is the one fact the client cannot compute: the kernel makes the
 * FIRST viewer of a controller-less dispatched session its controller and a
 * later adopter an observer. The UI labels the two "Übernommen" (controller
 * true — permission asks come here) vs "Beobachten" (false — watching only).
 */

export const ADOPT_META_KEY = 'contenox.adopt';

/**
 * The REQUEST-side adopt reference: which running instance, and which of ITS
 * downstream sessions, a session/new is binding to. Both ids are required (an
 * instance multiplexes many sessions), mirroring acpsvc's adoptRef.
 */
export type AdoptRef = {
  instanceId: string;
  sessionId: string;
};

/** Builds the `{ "contenox.adopt": {...} }` request `_meta` a session/new sends to adopt. */
export function adoptMeta(instanceId: string, sessionId: string): Record<string, unknown> {
  return { [ADOPT_META_KEY]: { instanceId, sessionId } };
}

/**
 * The RESPONSE-side adopt outcome echoed on the session/new response: the exact
 * binding (instanceId/sessionId) plus whether this connection took control.
 * Mirrors acpsvc's adoptResult.
 */
export type AdoptResult = {
  instanceId: string;
  sessionId: string;
  controller: boolean;
};

/**
 * Decodes the RESPONSE-side `contenox.adopt` outcome from a session's `_meta`
 * (the session/new response echo, threaded into the roster). The counterpart of
 * the server's `parseAdoptResultMeta`, deliberately just as defensive: an absent
 * key, a non-object value, or missing ids all read as `null` ("no adopt outcome
 * reported") rather than an error, so a native session, an older serve that does
 * not speak adopt, or a future protocol revision all degrade to "not an adopted
 * session" instead of crashing the chat surface.
 */
export function adoptResultFromMeta(meta: Record<string, unknown> | undefined | null): AdoptResult | null {
  const raw = meta?.[ADOPT_META_KEY];
  if (raw === null || typeof raw !== 'object') return null;
  const r = raw as Partial<AdoptResult>;
  const instanceId = typeof r.instanceId === 'string' ? r.instanceId.trim() : '';
  const sessionId = typeof r.sessionId === 'string' ? r.sessionId.trim() : '';
  if (instanceId === '' || sessionId === '') return null;
  return { instanceId, sessionId, controller: r.controller === true };
}

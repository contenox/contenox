#!/usr/bin/env node
/**
 * Drives one full session lifecycle against a live `/acp` WebSocket endpoint
 * (runtime/contenoxcli/acp_ws.go) to prove the multi-session primitives the
 * Stage 2 workspace layer relies on: initialize -> session/new -> prompt ->
 * session/list (assert present) -> session/load (assert replay) ->
 * session/delete -> session/list (assert gone). Sibling to driveRealTurn.mjs
 * (same conventions: zero dependencies, plain Node, hand-rolled JSON-RPC —
 * see that file's header comment for why this doesn't import client.ts).
 *
 * Usage:
 *   node driveSessionLifecycle.mjs <ws-url> [bearer-token]
 *
 * IMPORTANT environment caveat this script is deliberately written to
 * tolerate: on a machine where `contenox serve` is running but no models
 * have been pulled, `session/prompt` fails with an RPC error (e.g. "no
 * models found in runtime state") — the chain never reaches a model call.
 * That failure is NOT a session-lifecycle bug: acpsvc/prompt.go still builds
 * the user's message into the chat history it hands to
 * agentservice.Prompt() (runtime/agentservice/agent.go's buildChatInput),
 * and persistHistory() synthesizes+persists that history (including the user
 * message) on any non-"poison pill" failure — see
 * taskengine/synthesizer.go's SynthesizeHistory, which seeds its output with
 * `prior` (history + the new user message) unconditionally, then appends
 * whatever the chain produced (nothing, on a hard failure) plus a failure
 * annotation. So the user message is expected to survive to session/load's
 * replay even when the prompt call itself errors — this script asserts
 * exactly that, and reports plainly which half (prompt success vs. user-
 * message persistence) was actually exercised, without faking a pass on the
 * half it couldn't reach.
 *
 * Exits 0 (echoing what WAS and WAS NOT verified) when the session-lifecycle
 * primitives all check out, regardless of whether the prompt turn itself
 * succeeded. Exits 1 on any lifecycle assertion failure, connection failure,
 * or timeout.
 */

const CONNECT_TIMEOUT_MS = 10_000;
const SCRIPT_TIMEOUT_MS = 30_000;
const PROMPT_TEXT = 'driveSessionLifecycle probe: reply with one word: ready';

function fail(message) {
  console.error(`FAIL: ${message}`);
  process.exit(1);
}

const [, , rawUrl, token] = process.argv;
if (!rawUrl) {
  fail('usage: node driveSessionLifecycle.mjs <ws-url> [bearer-token]');
}
if (typeof WebSocket !== 'function') {
  fail('global WebSocket is not available — this script requires Node 22+');
}

let url = rawUrl;
if (token) {
  const u = new URL(rawUrl);
  u.searchParams.set('token', token);
  url = u.toString();
}

console.log(`connecting to ${rawUrl} ...`);

const ws = new WebSocket(url);

let nextId = 1;
const pending = new Map();
/** Every `session/update` this connection receives, in arrival order — includes session/load replay. */
const updates = [];
let sessionId;
let settled = false;

// What this run actually established, printed in the final summary instead
// of silently assuming success.
const verified = {
  lifecyclePrimitives: false, // new -> list(present) -> load(replay ok) -> delete -> list(absent)
  promptSucceeded: null, // true | false | null (unknown/not reached)
  userMessagePersisted: null, // true | false | null (unknown/not reached)
};

function send(frame) {
  ws.send(JSON.stringify(frame));
}

function call(method, params) {
  const id = nextId++;
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    send({ jsonrpc: '2.0', id, method, params });
  });
}

function respondResult(id, result) {
  send({ jsonrpc: '2.0', id, result });
}

function respondError(id, code, message) {
  send({ jsonrpc: '2.0', id, error: { code, message } });
}

function finish(code, message) {
  if (settled) return;
  settled = true;
  clearTimeout(connectTimer);
  clearTimeout(scriptTimer);
  console.log('--- summary ---');
  console.log(`session lifecycle (new/list/load/delete/list): ${verified.lifecyclePrimitives ? 'VERIFIED' : 'NOT verified'}`);
  console.log(
    `prompt turn: ${verified.promptSucceeded === null ? 'not reached' : verified.promptSucceeded ? 'succeeded' : 'FAILED (see caveat in this script\'s header — expected with no models pulled)'}`,
  );
  console.log(
    `user message persisted despite prompt outcome: ${verified.userMessagePersisted === null ? 'not verified (session/load never reached)' : verified.userMessagePersisted ? 'VERIFIED (present in replay)' : 'NOT present in replay'}`,
  );
  if (message) {
    if (code === 0) console.log(message);
    else console.error(`FAIL: ${message}`);
  }
  try {
    ws.close();
  } catch {
    // already closed; ignore.
  }
  process.exitCode = code;
  setTimeout(() => process.exit(code), 50);
}

const connectTimer = setTimeout(() => {
  finish(1, `did not connect within ${CONNECT_TIMEOUT_MS}ms`);
}, CONNECT_TIMEOUT_MS);

const scriptTimer = setTimeout(() => {
  finish(1, `did not complete within ${SCRIPT_TIMEOUT_MS}ms`);
}, SCRIPT_TIMEOUT_MS);
scriptTimer.unref?.();

ws.addEventListener('open', () => {
  clearTimeout(connectTimer);
  console.log('connected; running the session lifecycle ...');
  run().catch(err => finish(1, err instanceof Error ? err.stack || err.message : String(err)));
});

ws.addEventListener('error', ev => {
  finish(1, `websocket error: ${ev?.message ?? ev}`);
});

ws.addEventListener('close', ev => {
  if (!settled) {
    finish(1, `connection closed before the sequence finished (code=${ev.code}, reason=${ev.reason})`);
  }
});

ws.addEventListener('message', ev => {
  let msg;
  try {
    msg = JSON.parse(typeof ev.data === 'string' ? ev.data : String(ev.data));
  } catch {
    console.error('WARN: ignoring malformed frame:', ev.data);
    return;
  }

  const hasMethod = typeof msg.method === 'string';
  const hasId = Object.prototype.hasOwnProperty.call(msg, 'id') && msg.id !== undefined;

  if (hasMethod && hasId) {
    handleIncomingRequest(msg);
    return;
  }
  if (hasMethod) {
    handleNotification(msg);
    return;
  }
  if (hasId) {
    const entry = pending.get(msg.id);
    if (!entry) return;
    pending.delete(msg.id);
    if (msg.error) entry.reject(new RpcError(msg.error));
    else entry.resolve(msg.result);
  }
});

class RpcError extends Error {
  constructor(errorObject) {
    super(`RPC error ${errorObject.code}: ${errorObject.message}`);
    this.code = errorObject.code;
  }
}

function handleNotification(n) {
  if (n.method !== 'session/update') return;
  const update = n.params?.update;
  if (!update?.sessionUpdate) return;
  updates.push(update);
  console.log(`update: ${update.sessionUpdate}${update.messageId ? ` (messageId=${update.messageId})` : ''}`);
}

function handleIncomingRequest(req) {
  if (req.method === 'session/request_permission') {
    const options = req.params?.options ?? [];
    const allow = options.find(o => String(o.kind).startsWith('allow_')) ?? options[0];
    console.log(`session/request_permission received; auto-selecting ${allow?.optionId ?? '<none>'}`);
    if (allow) {
      respondResult(req.id, { outcome: { outcome: 'selected', optionId: allow.optionId } });
    } else {
      respondResult(req.id, { outcome: { outcome: 'cancelled' } });
    }
    return;
  }
  respondError(req.id, -32601, `not supported by driveSessionLifecycle.mjs: ${req.method}`);
}

function assert(cond, message) {
  if (!cond) throw new Error(`assertion failed: ${message}`);
}

async function run() {
  const cwd = process.cwd();

  const initResult = await call('initialize', {
    protocolVersion: 1,
    clientCapabilities: {},
    clientInfo: { name: 'contenox-beam-acp-driveSessionLifecycle', version: '0.0.0' },
  });
  console.log(`initialized: protocolVersion=${initResult.protocolVersion} agent=${initResult.agentInfo?.name ?? '<unknown>'}`);

  // --- session/new ---
  const newSessionResult = await call('session/new', { cwd, mcpServers: [] });
  sessionId = newSessionResult.sessionId;
  assert(sessionId, 'session/new response carried no sessionId');
  console.log(`session created: ${sessionId}`);

  // --- session/prompt (tolerated failure — see the header comment) ---
  console.log(`sending prompt: ${JSON.stringify(PROMPT_TEXT)}`);
  try {
    const promptResult = await call('session/prompt', {
      sessionId,
      prompt: [{ type: 'text', text: PROMPT_TEXT }],
    });
    verified.promptSucceeded = true;
    console.log(`prompt turn succeeded: stopReason=${promptResult.stopReason}`);
  } catch (err) {
    verified.promptSucceeded = false;
    console.log(`prompt turn failed (tolerated): ${err instanceof Error ? err.message : String(err)}`);
  }

  // --- session/list: assert the session is present ---
  const list1 = await call('session/list', {});
  const present = (list1.sessions ?? []).some(s => s.sessionId === sessionId);
  assert(present, `session/list did not include ${sessionId} after session/new`);
  console.log(`session/list: ${sessionId} present (of ${list1.sessions?.length ?? 0} sessions)`);

  // --- session/load: assert the user message survived to replay ---
  updates.length = 0;
  await call('session/load', { sessionId, cwd, mcpServers: [] });
  const userChunks = updates.filter(u => u.sessionUpdate === 'user_message_chunk');
  const promptTextPersisted = userChunks.some(u => u.content?.text === PROMPT_TEXT);
  verified.userMessagePersisted = promptTextPersisted;
  if (promptTextPersisted) {
    console.log('session/load replay: user_message_chunk for the probe prompt is present — the user turn survived the prompt outcome above.');
  } else {
    console.log(
      `session/load replay: user_message_chunk for the probe prompt NOT found (saw ${userChunks.length} user_message_chunk update(s) total) — this contradicts the documented persistence path (see this script's header comment) and should be investigated, not silently accepted.`,
    );
  }

  // --- session/delete ---
  await call('session/delete', { sessionId });
  console.log(`session deleted: ${sessionId}`);

  // --- session/list: assert the session is gone ---
  const list2 = await call('session/list', {});
  const stillPresent = (list2.sessions ?? []).some(s => s.sessionId === sessionId);
  assert(!stillPresent, `session/list still included ${sessionId} after session/delete`);
  console.log(`session/list: ${sessionId} absent after delete, as expected`);

  verified.lifecyclePrimitives = true;
  finish(promptTextPersisted ? 0 : 1, 'OK: session lifecycle verified end to end');
}

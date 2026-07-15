#!/usr/bin/env node
/**
 * Drives one real turn against a live `/acp` WebSocket endpoint
 * (runtime/contenoxcli/acp_ws.go) to prove the wire protocol end to end,
 * outside vitest — this is NOT part of the test suite and touches the
 * network on purpose.
 *
 * Usage:
 *   node driveRealTurn.mjs <ws-url> [bearer-token]
 *
 * Examples:
 *   node driveRealTurn.mjs ws://127.0.0.1:32123/acp
 *   node driveRealTurn.mjs ws://127.0.0.1:32123/acp secret-token
 *
 * Runtime: plain Node.js, no build step, no TypeScript. It intentionally does
 * NOT import client.ts/transport.ts (those are ESM+TS and importing them
 * un-transpiled would require a loader) — instead it reimplements just enough
 * of the same wire protocol inline, in plain JS, so this script has zero
 * toolchain dependencies beyond Node itself. The wire shapes mirror
 * src/lib/acp/{types,client}.ts and, ultimately, /libacp/*.go — keep them in
 * sync if either changes.
 *
 * Uses Node's global `WebSocket` (stable since Node 22, built on undici) —
 * not the `ws` package — so this script has zero npm dependencies of its own.
 *
 * Exits 0 and prints "OK" with the observed update kinds + stopReason on
 * success. Exits 1 (with a diagnostic on stderr) on any error, RPC error
 * response, connection failure, or timeout.
 */

const TURN_TIMEOUT_MS = 30_000;
const CONNECT_TIMEOUT_MS = 10_000;

function fail(message) {
  console.error(`FAIL: ${message}`);
  process.exit(1);
}

const [, , rawUrl, token] = process.argv;
if (!rawUrl) {
  fail('usage: node driveRealTurn.mjs <ws-url> [bearer-token]');
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
const seenUpdateKinds = [];
let sessionId;
let settled = false;

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
  clearTimeout(turnTimer);
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
  // Give the socket a beat to close cleanly before the event loop drains.
  setTimeout(() => process.exit(code), 50);
}

const connectTimer = setTimeout(() => {
  finish(1, `did not connect within ${CONNECT_TIMEOUT_MS}ms`);
}, CONNECT_TIMEOUT_MS);

const turnTimer = setTimeout(() => {
  finish(1, `turn did not complete within ${TURN_TIMEOUT_MS}ms`);
}, TURN_TIMEOUT_MS);
turnTimer.unref?.();

ws.addEventListener('open', () => {
  clearTimeout(connectTimer);
  console.log('connected; sending initialize ...');
  runTurn().catch((err) => finish(1, err instanceof Error ? err.stack || err.message : String(err)));
});

ws.addEventListener('error', (ev) => {
  finish(1, `websocket error: ${ev?.message ?? ev}`);
});

ws.addEventListener('close', (ev) => {
  if (!settled) {
    finish(1, `connection closed before the turn finished (code=${ev.code}, reason=${ev.reason})`);
  }
});

ws.addEventListener('message', (ev) => {
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
    if (msg.error) entry.reject(new Error(`RPC error ${msg.error.code}: ${msg.error.message}`));
    else entry.resolve(msg.result);
  }
});

function handleNotification(n) {
  if (n.method !== 'session/update') return;
  const update = n.params?.update;
  if (!update?.sessionUpdate) return;
  seenUpdateKinds.push(update.sessionUpdate);
  console.log(`update: ${update.sessionUpdate}`);
}

function handleIncomingRequest(req) {
  if (req.method === 'session/request_permission') {
    // Nothing in this smoke turn should require a tool call, but answer
    // instead of hanging if the agent asks anyway: pick the first "allow"
    // option so the turn can complete unattended.
    const options = req.params?.options ?? [];
    const allow = options.find((o) => String(o.kind).startsWith('allow_')) ?? options[0];
    console.log(`session/request_permission received; auto-selecting ${allow?.optionId ?? '<none>'}`);
    if (allow) {
      respondResult(req.id, { outcome: { outcome: 'selected', optionId: allow.optionId } });
    } else {
      respondResult(req.id, { outcome: { outcome: 'cancelled' } });
    }
    return;
  }
  respondError(req.id, -32601, `not supported by driveRealTurn.mjs: ${req.method}`);
}

async function runTurn() {
  const initResult = await call('initialize', {
    protocolVersion: 1,
    clientCapabilities: {},
    clientInfo: { name: 'contenox-beam-acp-driveRealTurn', version: '0.0.0' },
  });
  console.log(
    `initialized: protocolVersion=${initResult.protocolVersion} agent=${initResult.agentInfo?.name ?? '<unknown>'}`,
  );

  const newSessionResult = await call('session/new', { cwd: process.cwd(), mcpServers: [] });
  sessionId = newSessionResult.sessionId;
  if (!sessionId) throw new Error('session/new response carried no sessionId');
  console.log(`session created: ${sessionId}`);

  console.log('sending prompt: "Reply with one word: ready"');
  const promptResult = await call('session/prompt', {
    sessionId,
    prompt: [{ type: 'text', text: 'Reply with one word: ready' }],
  });

  const summary = `OK stopReason=${promptResult.stopReason} updates=[${seenUpdateKinds.join(', ')}]`;
  finish(promptResult.stopReason ? 0 : 1, summary);
}

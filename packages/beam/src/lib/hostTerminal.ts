/**
 * The host-terminal bridge and its wire helpers — the bolt.diy xterm↔WebSocket
 * bridge, factored into pure, DOM-free pieces so the frame plumbing (input out,
 * output in, resize frame shape, read-only gating, take-over flip) is
 * unit-testable against a mock socket + mock terminal, exactly the parts the
 * component cannot exercise under the repo's no-DOM test harness.
 *
 * Backend: runtime/internal/terminalapi. Binary frames carry PTY bytes both
 * ways; an in-band TEXT-shaped JSON `{"type":"resize",cols,rows}` frame resizes
 * the PTY (see termConn.Read). The server may also send an in-band
 * `{"type":"error",message}` frame on attach failure.
 */

/**
 * DECIDED interactivity policy — READ-ONLY by default. FLIPPABLE.
 *
 * The terminalapi backend is a bare interactive host shell behind the serve
 * bearer token (ide-workflows.md, Arc 3, "OPEN DECISION"). Beam opens it
 * read-only (xterm `disableStdin: true` + this gate) and exposes an explicit,
 * honestly-labelled "Take over shell" affordance that flips the SAME view
 * interactive. Rationale: the terminal is a real shell on the host serving the
 * runtime, bounded only by the serve token, and it brushes against the
 * control-plane isolation invariant — so watching is the safe default and
 * taking the shell is a deliberate, named act, not the resting state.
 *
 * To change the product default, flip this constant (and the confirmation copy
 * stops being the only guard). It is a one-line policy switch by construction —
 * the read-only state and the interactive state are the SAME component, gated
 * here, not two code paths.
 */
export const TERMINAL_READ_ONLY_DEFAULT = true;

/** The in-band resize control frame the PTY reads (terminalapi.termConn.Read). */
export function resizeFrame(cols: number, rows: number): string {
  return JSON.stringify({ type: 'resize', cols, rows });
}

export interface LocationLike {
  protocol: string;
  host: string;
}

/**
 * Turns the server-authoritative `wsPath` (already `/api`-prefixed, e.g.
 * `/api/terminal/sessions/{id}/ws`) into a `ws(s)://<host><path>` URL, reusing
 * the same-origin idiom as the `/acp` socket (AcpWorkspaceProvider.buildAcpWsUrl).
 *
 * AUTH: none is added in JS. The HttpOnly `auth_token` session cookie
 * authenticates the upgrade automatically — it rides the same-origin WebSocket
 * handshake, which is exactly one of the forms terminalapi.requireToken accepts
 * (Authorization bearer / X-API-Key / `?token=` / the `auth_token` cookie). The
 * browser never handles the token itself, matching the rest of Beam's
 * cookie-BFF posture; a programmatic client that cannot send the cookie is the
 * only caller that would append `?token=`.
 */
export function buildTerminalWsUrl(wsPath: string, location?: LocationLike): string {
  const loc =
    location ?? (typeof window !== 'undefined' ? window.location : undefined);
  if (!loc) {
    throw new Error('buildTerminalWsUrl: no location available (pass one outside a browser)');
  }
  const protocol = loc.protocol === 'https:' ? 'wss' : 'ws';
  const path = wsPath.startsWith('/') ? wsPath : `/${wsPath}`;
  return `${protocol}://${loc.host}${path}`;
}

/** The slice of xterm's `Terminal` the bridge touches. */
export interface TerminalBridgeTerminal {
  onData(handler: (data: string) => void): { dispose(): void };
  write(data: string | Uint8Array): void;
}

/** The slice of the browser `WebSocket` the bridge touches. */
export interface TerminalBridgeSocket {
  send(data: string | ArrayBufferLike | ArrayBufferView): void;
  addEventListener(type: 'message', listener: (ev: { data: unknown }) => void): void;
  removeEventListener(type: 'message', listener: (ev: { data: unknown }) => void): void;
}

export interface HostTerminalBridge {
  /** Flip input gating — the "Take over shell" / return-to-read-only switch. */
  setReadOnly(readOnly: boolean): void;
  isReadOnly(): boolean;
  /** Send the in-band resize frame after a fit. */
  sendResize(cols: number, rows: number): void;
  /** Detach listeners; safe to call once. */
  dispose(): void;
}

export interface HostTerminalBridgeOptions {
  /** Initial gating; defaults to {@link TERMINAL_READ_ONLY_DEFAULT}. */
  readOnly?: boolean;
}

/**
 * Wires a terminal to a socket:
 *  - terminal input → `socket.send` ONLY when not read-only (the take-over gate);
 *  - socket binary/text frames → `terminal.write` (ArrayBuffer / typed-array /
 *    string all handled; the component sets `binaryType='arraybuffer'` so PTY
 *    bytes arrive as ArrayBuffer, never a Blob).
 * Read-only-ness is a live flag, so a take-over flips the SAME bridge without
 * reconnecting the socket or rebuilding the terminal.
 */
export function createHostTerminalBridge(
  terminal: TerminalBridgeTerminal,
  socket: TerminalBridgeSocket,
  options: HostTerminalBridgeOptions = {},
): HostTerminalBridge {
  let readOnly = options.readOnly ?? TERMINAL_READ_ONLY_DEFAULT;

  const dataSub = terminal.onData(data => {
    if (readOnly) return;
    socket.send(data);
  });

  const onMessage = (ev: { data: unknown }) => {
    const payload = ev.data;
    if (typeof payload === 'string') {
      terminal.write(payload);
      return;
    }
    if (payload instanceof ArrayBuffer) {
      terminal.write(new Uint8Array(payload));
      return;
    }
    if (ArrayBuffer.isView(payload)) {
      terminal.write(new Uint8Array((payload as ArrayBufferView).buffer));
      return;
    }
    // Nothing is silently dropped — stringify an unexpected shape.
    terminal.write(String(payload));
  };
  socket.addEventListener('message', onMessage);

  return {
    setReadOnly: value => {
      readOnly = value;
    },
    isReadOnly: () => readOnly,
    sendResize: (cols, rows) => socket.send(resizeFrame(cols, rows)),
    dispose: () => {
      dataSub.dispose();
      socket.removeEventListener('message', onMessage);
    },
  };
}

/**
 * The pluggable transport boundary. `AcpClient` (see client.ts) speaks JSON-RPC
 * text frames over whatever implements this interface — it never touches a
 * WebSocket, a subprocess, or any other I/O primitive directly. That keeps the
 * client core testable (see client.test.ts's MockTransport) and portable to a
 * future stdio/Node adapter without touching client.ts.
 *
 * Framing contract: each `send`/`onMessage` value is exactly one JSON-RPC
 * message serialized as a single string ("one frame == one message"), matching
 * the runtime's `/acp` WebSocket endpoint (see
 * runtime/contenoxcli/acp_ws.go): one TEXT frame per libacp NDJSON line.
 */
export interface Transport {
  /** Send one JSON-RPC message (already serialized) as a single frame. */
  send(text: string): void;
  /** Register a callback invoked once per received frame, in arrival order. */
  onMessage(cb: (text: string) => void): void;
  /** Register a callback invoked when the transport closes (locally or remotely). */
  onClose(cb: (err?: Error) => void): void;
  /** Close the transport. Safe to call more than once. */
  close(): void;
}

/** Constructor shape `WebSocketTransport` needs — matches the browser `WebSocket` global. */
export interface WebSocketLike {
  readyState: number;
  send(data: string): void;
  close(code?: number, reason?: string): void;
  addEventListener(type: 'open', listener: () => void): void;
  addEventListener(type: 'message', listener: (ev: { data: unknown }) => void): void;
  addEventListener(type: 'close', listener: (ev: { code?: number; reason?: string }) => void): void;
  addEventListener(type: 'error', listener: (ev: unknown) => void): void;
}

export interface WebSocketConstructorLike {
  new (url: string, protocols?: string | string[]): WebSocketLike;
}

export interface WebSocketTransportOptions {
  /** WebSocket subprotocols to request. */
  protocols?: string | string[];
  /**
   * WebSocket constructor to use. Defaults to `globalThis.WebSocket` (the
   * browser global). Pass a different implementation to run this transport
   * outside a browser (e.g. the `ws` package, or a test double).
   */
  WebSocketImpl?: WebSocketConstructorLike;
}

const READY_STATE_OPEN = 1;

/**
 * `Transport` over the browser `WebSocket` API: one ACP JSON-RPC message per
 * TEXT frame, exactly what `/acp` speaks. Messages sent before the socket
 * reaches `OPEN` are buffered and flushed on `open`, so callers don't have to
 * sequence `initialize()` after a manual open-handshake themselves.
 */
export class WebSocketTransport implements Transport {
  private readonly ws: WebSocketLike;
  private readonly messageCbs: Array<(text: string) => void> = [];
  private readonly closeCbs: Array<(err?: Error) => void> = [];
  private readonly sendQueue: string[] = [];
  private closedFired = false;

  constructor(url: string, options: WebSocketTransportOptions = {}) {
    const Impl = options.WebSocketImpl ?? (globalThis as { WebSocket?: WebSocketConstructorLike }).WebSocket;
    if (!Impl) {
      throw new Error(
        'WebSocketTransport: no WebSocket implementation available (pass options.WebSocketImpl outside a browser)',
      );
    }
    this.ws = new Impl(url, options.protocols);

    this.ws.addEventListener('open', () => {
      const queued = this.sendQueue.splice(0, this.sendQueue.length);
      for (const text of queued) {
        this.ws.send(text);
      }
    });
    this.ws.addEventListener('message', (ev) => {
      const text = typeof ev.data === 'string' ? ev.data : String(ev.data);
      for (const cb of this.messageCbs) cb(text);
    });
    this.ws.addEventListener('close', () => {
      this.fireClose();
    });
    this.ws.addEventListener('error', () => {
      // The 'close' event always follows 'error' for WebSocket, so closing is
      // handled there; this listener only exists so an unhandled-error is
      // never thrown by the underlying implementation.
    });
  }

  send(text: string): void {
    if (this.ws.readyState !== READY_STATE_OPEN) {
      this.sendQueue.push(text);
      return;
    }
    this.ws.send(text);
  }

  onMessage(cb: (text: string) => void): void {
    this.messageCbs.push(cb);
  }

  onClose(cb: (err?: Error) => void): void {
    this.closeCbs.push(cb);
  }

  close(): void {
    this.ws.close();
  }

  private fireClose(err?: Error): void {
    if (this.closedFired) return;
    this.closedFired = true;
    for (const cb of this.closeCbs) cb(err);
  }
}

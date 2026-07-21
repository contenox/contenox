import { describe, expect, it } from 'vitest';
import {
  buildTerminalWsUrl,
  createHostTerminalBridge,
  resizeFrame,
  TERMINAL_READ_ONLY_DEFAULT,
  type TerminalBridgeSocket,
  type TerminalBridgeTerminal,
} from './hostTerminal';

describe('resizeFrame', () => {
  it('produces the exact in-band control shape the PTY reads', () => {
    expect(resizeFrame(120, 40)).toBe('{"type":"resize","cols":120,"rows":40}');
    expect(JSON.parse(resizeFrame(80, 24))).toEqual({ type: 'resize', cols: 80, rows: 24 });
  });
});

describe('buildTerminalWsUrl', () => {
  it('uses ws:// for http and preserves the server-authoritative path', () => {
    expect(
      buildTerminalWsUrl('/api/terminal/sessions/abc/ws', { protocol: 'http:', host: 'lan:32125' }),
    ).toBe('ws://lan:32125/api/terminal/sessions/abc/ws');
  });

  it('uses wss:// for https', () => {
    expect(
      buildTerminalWsUrl('/api/terminal/sessions/x/ws', { protocol: 'https:', host: 'h' }),
    ).toBe('wss://h/api/terminal/sessions/x/ws');
  });

  it('normalises a path missing its leading slash', () => {
    expect(buildTerminalWsUrl('api/terminal/sessions/x/ws', { protocol: 'http:', host: 'h' })).toBe(
      'ws://h/api/terminal/sessions/x/ws',
    );
  });
});

/** Minimal fakes: a terminal capturing writes and exposing its onData sink, and a socket recording sends. */
function fakes() {
  const writes: Array<string | Uint8Array> = [];
  let dataHandler: ((d: string) => void) | undefined;
  const disposed = { data: false };
  const terminal: TerminalBridgeTerminal = {
    onData(handler) {
      dataHandler = handler;
      return {
        dispose() {
          disposed.data = true;
          dataHandler = undefined;
        },
      };
    },
    write(data) {
      writes.push(data);
    },
  };

  const sends: Array<string | ArrayBufferLike | ArrayBufferView> = [];
  let messageListener: ((ev: { data: unknown }) => void) | undefined;
  const socket: TerminalBridgeSocket = {
    send(data) {
      sends.push(data);
    },
    addEventListener(_type, listener) {
      messageListener = listener;
    },
    removeEventListener() {
      messageListener = undefined;
    },
  };

  return {
    terminal,
    socket,
    writes,
    sends,
    typeInput: (d: string) => dataHandler?.(d),
    emitMessage: (data: unknown) => messageListener?.({ data }),
    disposed,
    hasMessageListener: () => messageListener !== undefined,
  };
}

describe('createHostTerminalBridge — read-only gating and take-over flip', () => {
  it('defaults to read-only and drops terminal input (does not send)', () => {
    const f = fakes();
    const bridge = createHostTerminalBridge(f.terminal, f.socket);
    expect(TERMINAL_READ_ONLY_DEFAULT).toBe(true);
    expect(bridge.isReadOnly()).toBe(true);

    f.typeInput('ls\r');
    expect(f.sends).toHaveLength(0);
  });

  it('forwards terminal input to the socket after take-over', () => {
    const f = fakes();
    const bridge = createHostTerminalBridge(f.terminal, f.socket);

    bridge.setReadOnly(false); // "Take over shell"
    f.typeInput('whoami\r');
    expect(f.sends).toEqual(['whoami\r']);

    bridge.setReadOnly(true); // return to watching
    f.typeInput('rm -rf /\r');
    expect(f.sends).toEqual(['whoami\r']); // nothing new
  });

  it('honours an explicit interactive start option', () => {
    const f = fakes();
    createHostTerminalBridge(f.terminal, f.socket, { readOnly: false });
    f.typeInput('x');
    expect(f.sends).toEqual(['x']);
  });
});

describe('createHostTerminalBridge — output frames in', () => {
  it('writes a binary (ArrayBuffer) PTY frame as bytes to the terminal', () => {
    const f = fakes();
    createHostTerminalBridge(f.terminal, f.socket);
    const bytes = new Uint8Array([104, 105]); // "hi"
    f.emitMessage(bytes.buffer);
    expect(f.writes).toHaveLength(1);
    expect(Array.from(f.writes[0] as Uint8Array)).toEqual([104, 105]);
  });

  it('writes a typed-array frame as bytes', () => {
    const f = fakes();
    createHostTerminalBridge(f.terminal, f.socket);
    f.emitMessage(new Uint8Array([65]));
    expect(Array.from(f.writes[0] as Uint8Array)).toEqual([65]);
  });

  it('writes a string frame (e.g. an in-band error) verbatim', () => {
    const f = fakes();
    createHostTerminalBridge(f.terminal, f.socket);
    f.emitMessage('{"type":"error","message":"session not found"}');
    expect(f.writes[0]).toContain('session not found');
  });
});

describe('createHostTerminalBridge — resize and disposal', () => {
  it('sends the resize frame on demand', () => {
    const f = fakes();
    const bridge = createHostTerminalBridge(f.terminal, f.socket);
    bridge.sendResize(100, 30);
    expect(f.sends).toEqual([resizeFrame(100, 30)]);
  });

  it('detaches both listeners on dispose', () => {
    const f = fakes();
    const bridge = createHostTerminalBridge(f.terminal, f.socket);
    bridge.dispose();
    expect(f.disposed.data).toBe(true);
    expect(f.hasMessageListener()).toBe(false);
  });
});

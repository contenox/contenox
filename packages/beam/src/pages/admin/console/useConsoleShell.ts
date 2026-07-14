import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../../../lib/api';
import { getStoredApiToken } from '../../../lib/fetch';

/**
 * Inline shell for the console's `!command` bang-escape: one lazy PTY session
 * per console page, output streamed into scrollback entries. The PTY echoes
 * the prompt and the command itself, so entries render the raw (ANSI-stripped)
 * stream without any synthetic header.
 */

export type ShellEntry = {
  id: number;
  /** Key of the last agent turn at exec time; entries render after it. */
  anchorKey: string | null;
  text: string;
};

const COLS = 120;
const ROWS = 30;
/** Per-entry cap so a runaway command can't grow the DOM unbounded. */
const MAX_ENTRY_CHARS = 200_000;

// CSI/OSC/single-char escapes. PTYs emit these even for plain commands
// (colors, title updates, bracketed paste); the console renders plain text.
const ANSI_RE =
  // eslint-disable-next-line no-control-regex
  /\x1b(?:\[[0-9;?]*[ -/]*[@-~]|\][^\x07\x1b]*(?:\x07|\x1b\\)|[@-Z\\^_])/g;

function sanitizeChunk(raw: string): string {
  return raw
    .replace(ANSI_RE, '')
    .replace(/\r\n/g, '\n')
    // Lone \r is a redraw (progress bars); keep the text linear instead.
    .replace(/\r/g, '\n');
}

export function useConsoleShell() {
  const [entries, setEntries] = useState<ShellEntry[]>([]);
  const wsRef = useRef<WebSocket | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const connectingRef = useRef<Promise<WebSocket> | null>(null);
  const nextIdRef = useRef(1);
  const decoderRef = useRef(new TextDecoder());

  const appendToLast = useCallback((chunk: string) => {
    if (!chunk) return;
    setEntries(prev => {
      if (prev.length === 0) return prev;
      const last = prev[prev.length - 1];
      const text = (last.text + chunk).slice(-MAX_ENTRY_CHARS);
      return [...prev.slice(0, -1), { ...last, text }];
    });
  }, []);

  const connect = useCallback(async (): Promise<WebSocket> => {
    const existing = wsRef.current;
    if (existing && existing.readyState === WebSocket.OPEN) return existing;
    if (connectingRef.current) return connectingRef.current;

    const promise = (async () => {
      const created = await api.createTerminalSession({ cwd: '', cols: COLS, rows: ROWS });
      sessionIdRef.current = created.id;

      const url = new URL(created.wsPath, window.location.origin);
      url.protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
      const token = getStoredApiToken();
      if (token && !url.searchParams.has('token')) url.searchParams.set('token', token);

      const socket = new WebSocket(url.toString());
      socket.binaryType = 'arraybuffer';

      socket.onmessage = (event: MessageEvent) => {
        if (typeof event.data === 'string') {
          try {
            const msg = JSON.parse(event.data) as { type?: string; message?: string };
            if (msg.type === 'error') {
              appendToLast(`\n[terminal error: ${msg.message ?? 'attach failed'}]\n`);
            }
          } catch {
            /* not a control frame */
          }
          return;
        }
        if (event.data instanceof ArrayBuffer) {
          const raw = decoderRef.current.decode(new Uint8Array(event.data), { stream: true });
          appendToLast(sanitizeChunk(raw));
        }
      };
      socket.onclose = () => {
        if (wsRef.current === socket) {
          wsRef.current = null;
          appendToLast('\n[shell session ended]\n');
        }
      };

      await new Promise<void>((resolve, reject) => {
        socket.onopen = () => resolve();
        socket.onerror = () => reject(new Error('shell connection failed'));
      });
      socket.send(JSON.stringify({ type: 'resize', cols: COLS, rows: ROWS }));
      wsRef.current = socket;
      return socket;
    })();

    connectingRef.current = promise;
    try {
      return await promise;
    } finally {
      connectingRef.current = null;
    }
  }, [appendToLast]);

  const exec = useCallback(
    (command: string, anchorKey: string | null) => {
      setEntries(prev => [...prev, { id: nextIdRef.current++, anchorKey, text: '' }]);
      void (async () => {
        try {
          const ws = await connect();
          ws.send(new TextEncoder().encode(command + '\n'));
        } catch (err) {
          appendToLast(
            `[shell unavailable: ${err instanceof Error ? err.message : String(err)}]\n`,
          );
        }
      })();
    },
    [appendToLast, connect],
  );

  useEffect(
    () => () => {
      wsRef.current?.close();
      wsRef.current = null;
      const id = sessionIdRef.current;
      if (id) void api.deleteTerminalSession(id).catch(() => {});
    },
    [],
  );

  return { entries, exec };
}

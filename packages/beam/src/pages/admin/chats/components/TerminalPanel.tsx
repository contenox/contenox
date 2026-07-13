import { Button, InsetPanel, Span, Spinner } from '@contenox/ui';
import { useQueryClient } from '@tanstack/react-query';
import { Paperclip, RotateCcw, X } from 'lucide-react';
import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { t } from 'i18next';
import { XTerminal, type XTerminalHandle } from '../../../../components/XTerminal';
import {
  isTooManyTerminalSessionsError,
  useCreateTerminalSession,
  useDeleteTerminalSession,
  usePruneTerminalSessions,
} from '../../../../hooks/useTerminal';
import { api } from '../../../../lib/api';
import {
  Artifact,
  useArtifactSource,
  type ArtifactSource,
} from '../../../../lib/artifacts';
import { terminalKeys } from '../../../../lib/queryKeys';
import { useSlashCommand, type SlashCommand } from '../../../../lib/slashCommands';
import {
  getSharedTerminalSession,
  reuseOrCreateTerminalSession,
  setSharedTerminalSession,
} from '../../../../lib/terminalSessionSingleton';

const SESSION_KEY = 'beam_terminal_session_id';
const DISCONNECT_RECREATE_MS = 350;
/** Maximum consecutive connection failures before the retry loop stops. */
const MAX_CONNECT_FAILURES = 3;

function normalizeTerminalWsPath(path: string): string {
  const trimmed = path.trim();
  if (trimmed.startsWith('/api/') || trimmed.startsWith('ws://') || trimmed.startsWith('wss://')) {
    return trimmed;
  }
  if (trimmed.startsWith('/')) {
    return `/api${trimmed}`;
  }
  return `/api/${trimmed}`;
}

export function TerminalPanel({ className }: { className?: string }) {
  const queryClient = useQueryClient();
  const createSessionMutation = useCreateTerminalSession();
  const deleteSessionMutation = useDeleteTerminalSession();
  const pruneSessions = usePruneTerminalSessions();

  const sharedInit = getSharedTerminalSession();
  const [wsUrl, setWsUrl] = useState<string | null>(() => sharedInit?.wsUrl ?? null);
  const [initializing, setInitializing] = useState(() => !sharedInit);
  const [error, setError] = useState<string | null>(null);
  const sessionIdRef = useRef<string | null>(sharedInit?.sessionId ?? null);
  const createGenRef = useRef(0);
  const disconnectDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const connectFailuresRef = useRef(0);
  /** Imperative handle used to read the visible xterm buffer for context arming. */
  const xtermRef = useRef<XTerminalHandle | null>(null);
  /**
   * When the user clicks "Attach last output", we capture the current buffer
   * into state. The next send reads it, emits one artifact, and clears the
   * armed state (one-shot). This keeps terminal output explicit and avoids
   * leaking every byte of shell activity into every turn.
   */
  const [armedOutput, setArmedOutput] = useState<{ output: string; capturedAt: string } | null>(
    null,
  );

  /**
   * One-shot terminal_output source. Registered whenever armedOutput is set.
   * collect() returns the artifact and immediately clears the armed state so
   * a second send does not resend the same stale output.
   */
  const terminalSource = useMemo<ArtifactSource | null>(() => {
    if (!armedOutput) return null;
    return {
      id: 'terminal:last_output',
      label: t('terminal.attached_label'),
      collect: () => {
        const snapshot = armedOutput;
        // Defer the clear so React doesn't flush mid-render. Uses microtask
        // so the current send sees the artifact, the next render does not.
        queueMicrotask(() => setArmedOutput(null));
        return Artifact.terminalOutput({
          session_id: sessionIdRef.current ?? undefined,
          output: snapshot.output,
          captured_at: snapshot.capturedAt,
        });
      },
    };
  }, [armedOutput]);
  useArtifactSource(terminalSource);

  const handleAttachOutput = useCallback(() => {
    const snapshot = xtermRef.current?.captureRecentOutput(400);
    if (snapshot == null) return;
    const trimmed = snapshot.trimEnd();
    if (!trimmed) return;
    setArmedOutput({ output: trimmed, capturedAt: new Date().toISOString() });
  }, []);

  /**
   * `@terminal` mention. Same effect as the header paperclip button: captures
   * the current xterm buffer, arms a one-shot terminal_output source that
   * fires on the next send. Fails gracefully when the terminal isn't mounted
   * yet (e.g. user types `@terminal` before opening the workspace).
   */
  const terminalCommand = useMemo<SlashCommand>(
    () => ({
      trigger: '@',
      name: 'terminal',
      aliases: ['term'],
      description: 'Mention the current terminal output as context.',
      usage: '@terminal',
      execute: (ctx) => {
        const snapshot = xtermRef.current?.captureRecentOutput(400);
        if (snapshot == null) {
          ctx.notify('error', '@terminal: no terminal mounted in this workspace.');
          return;
        }
        const trimmed = snapshot.trimEnd();
        if (!trimmed) {
          ctx.notify('error', '@terminal: nothing to attach (terminal is empty).');
          return;
        }
        const capturedAt = new Date().toISOString();
        ctx.armArtifact(
          'mention:terminal:last_output',
          '@terminal',
          Artifact.terminalOutput({
            session_id: sessionIdRef.current ?? undefined,
            output: trimmed,
            captured_at: capturedAt,
          }),
        );
        ctx.notify('info', 'Terminal output attached (will send with your next message).');
      },
    }),
    [],
  );
  useSlashCommand(terminalCommand);

  const handleClearAttached = useCallback(() => {
    setArmedOutput(null);
  }, []);

  /** Persists session id to sessionStorage and syncs the in-memory singleton used across remounts. */
  const persist = useCallback((sessionId: string | null, wsUrlForCache?: string | null) => {
    sessionIdRef.current = sessionId;
    if (sessionId) {
      const ws =
        wsUrlForCache ?? `/api/terminal/sessions/${encodeURIComponent(sessionId)}/ws`;
      setSharedTerminalSession({ sessionId, wsUrl: ws });
    } else {
      setSharedTerminalSession(null);
    }
    try {
      if (sessionId) sessionStorage.setItem(SESSION_KEY, sessionId);
      else sessionStorage.removeItem(SESSION_KEY);
    } catch {
      /* quota */
    }
  }, []);

  const clearDisconnectDebounce = useCallback(() => {
    if (disconnectDebounceRef.current) {
      clearTimeout(disconnectDebounceRef.current);
      disconnectDebounceRef.current = null;
    }
  }, []);

  const deleteSession = useCallback(
    async (id: string) => {
      try {
        await deleteSessionMutation.mutateAsync(id);
      } catch {
        /* session may already be gone */
      }
    },
    [deleteSessionMutation],
  );

  const createSession = useCallback(
    async (retryAfterPrune = true) => {
      const gen = ++createGenRef.current;
      setInitializing(true);
      setError(null);
      setWsUrl(null);
      try {
        const session = await reuseOrCreateTerminalSession(async () => {
          try {
            const res = await createSessionMutation.mutateAsync({ cwd: '' });
            return { sessionId: res.id, wsUrl: normalizeTerminalWsPath(res.wsPath) };
          } catch (e) {
            if (isTooManyTerminalSessionsError(e) && retryAfterPrune) {
              await pruneSessions();
              const res = await createSessionMutation.mutateAsync({ cwd: '' });
              return { sessionId: res.id, wsUrl: normalizeTerminalWsPath(res.wsPath) };
            }
            throw e;
          }
        });
        if (gen !== createGenRef.current) return;
        persist(session.sessionId, session.wsUrl);
        setWsUrl(session.wsUrl);
        setError(null);
      } catch (e) {
        if (gen !== createGenRef.current) return;
        const msg = e instanceof Error ? e.message : 'Failed to create terminal session';
        setError(msg);
      } finally {
        if (gen === createGenRef.current) {
          setInitializing(false);
        }
      }
    },
    [createSessionMutation, persist, pruneSessions],
  );

  // On mount: reuse in-tab singleton if present (avoids duplicate network on remount); else try saved session, then create
  useEffect(() => {
    let cancelled = false;
    const reused = getSharedTerminalSession();
    if (reused) {
      sessionIdRef.current = reused.sessionId;
      setWsUrl(reused.wsUrl);
      setInitializing(false);
      return () => {
        createGenRef.current++;
      };
    }

    (async () => {
      const savedId = (() => {
        try {
          return sessionStorage.getItem(SESSION_KEY);
        } catch {
          return null;
        }
      })();

      if (savedId) {
        try {
          const session = await queryClient.fetchQuery({
            queryKey: terminalKeys.session(savedId),
            queryFn: () => api.getTerminalSession(savedId),
          });
          if (cancelled) return;
          if (session.status === 'active') {
            const nextWsUrl = `/api/terminal/sessions/${savedId}/ws`;
            sessionIdRef.current = savedId;
            persist(savedId, nextWsUrl);
            setWsUrl(nextWsUrl);
            setInitializing(false);
            return;
          }
        } catch {
          // Session gone
        }
        if (cancelled) return;
        persist(null);
      }

      if (!cancelled) {
        await createSession();
      }
    })();
    return () => {
      cancelled = true;
      createGenRef.current++;
    };
  }, [createSession, persist, queryClient]);

  useEffect(
    () => () => {
      clearDisconnectDebounce();
    },
    [clearDisconnectDebounce],
  );

  const handleDisconnect = useCallback(() => {
    const id = sessionIdRef.current;
    persist(null);
    setWsUrl(null);
    setInitializing(true);
    setError(null);
    clearDisconnectDebounce();
    disconnectDebounceRef.current = setTimeout(() => {
      disconnectDebounceRef.current = null;
      void (async () => {
        if (id) {
          await deleteSession(id);
        }
        await createSession();
      })();
    }, DISCONNECT_RECREATE_MS);
  }, [persist, createSession, clearDisconnectDebounce, deleteSession]);

  /** Recreate the session on connection failure, up to {@link MAX_CONNECT_FAILURES} consecutive attempts. */
  const handleConnectionFailed = useCallback(() => {
    connectFailuresRef.current += 1;
    if (connectFailuresRef.current > MAX_CONNECT_FAILURES) {
      persist(null);
      setWsUrl(null);
      setInitializing(false);
      setError(t('terminal.connect_failed'));
      return;
    }
    persist(null);
    void createSession();
  }, [persist, createSession]);

  /** Reset the failure counter on a successful connection. */
  const handleOpen = useCallback(() => {
    connectFailuresRef.current = 0;
  }, []);

  const handleRestart = useCallback(async () => {
    clearDisconnectDebounce();
    connectFailuresRef.current = 0;
    const oldId = sessionIdRef.current;
    persist(null);
    setWsUrl(null);
    if (oldId) {
      await deleteSession(oldId);
    }
    await createSession();
  }, [persist, createSession, clearDisconnectDebounce, deleteSession]);

  const handleClose = useCallback(async () => {
    clearDisconnectDebounce();
    const oldId = sessionIdRef.current;
    persist(null);
    setWsUrl(null);
    setInitializing(false);
    if (oldId) {
      await deleteSession(oldId);
    }
  }, [persist, clearDisconnectDebounce, deleteSession]);

  const handleOpenTerminal = useCallback(() => {
    clearDisconnectDebounce();
    connectFailuresRef.current = 0;
    void createSession();
  }, [createSession, clearDisconnectDebounce]);

  if (initializing) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner size="md" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-4">
        <Span variant="muted" className="text-sm">
          {error}
        </Span>
        <Button variant="secondary" size="sm" onClick={handleOpenTerminal}>
          {t('terminal.retry')}
        </Button>
      </div>
    );
  }

  if (!wsUrl) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 p-4">
        <Span variant="muted" className="text-sm">
          {t('terminal.no_session')}
        </Span>
        <Button variant="primary" size="sm" onClick={handleOpenTerminal}>
          {t('terminal.create')}
        </Button>
      </div>
    );
  }

  return (
    <div className={`flex h-full min-h-0 w-full min-w-0 flex-col ${className ?? ''}`}>
      {/* Title bar */}
      <InsetPanel tone="strip" className="flex-row items-center justify-between gap-2 px-2 py-1.5">
        <div className="flex items-center gap-2">
          <Span variant="muted" className="text-xs font-medium">
            {t('terminal.title')}
          </Span>
          {armedOutput && (
            <Button
              type="button"
              variant="ghost"
              size="xs"
              onClick={handleClearAttached}
              title={t('terminal.detach_attached')}
              className="bg-success/10 text-success hover:bg-success/20 gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium"
            >
              <Paperclip className="h-3 w-3" />
              {t('terminal.attached_pill')}
              <X className="h-3 w-3" />
            </Button>
          )}
        </div>
        <div className="flex items-center gap-1">
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={handleAttachOutput}
            title={t('terminal.attach_output')}
          >
            <Paperclip className="h-3.5 w-3.5" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={handleRestart}
            title={t('terminal.restart')}
          >
            <RotateCcw className="h-3.5 w-3.5" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="xs"
            onClick={handleClose}
            title={t('terminal.close')}
          >
            <X className="h-3.5 w-3.5" />
          </Button>
        </div>
      </InsetPanel>
      <div className="flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden">
        <XTerminal
          ref={xtermRef}
          className="min-h-0 min-w-0 flex-1"
          wsUrl={wsUrl}
          onOpen={handleOpen}
          onDisconnect={handleDisconnect}
          onConnectionFailed={handleConnectionFailed}
        />
      </div>
    </div>
  );
}
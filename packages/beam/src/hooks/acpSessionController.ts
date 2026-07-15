import { AcpClient, textContent, type SessionId } from '../lib/acp';
import type { AcpSessionAction } from './acpSessionState';

/**
 * Orchestration for one ACP session, kept free of React so it can be driven
 * directly in a test with a `MockTransport`-backed `AcpClient` (see
 * `useAcpSession.test.tsx`). `useAcpSession.ts` is the only caller in the app
 * proper — it supplies `useReducer`'s `dispatch` and re-renders on the
 * resulting `acpSessionReducer` state.
 */

export type Dispatch = (action: AcpSessionAction) => void;

export interface AcpSessionController {
  /** Runs `initialize()` then `session/new`, dispatching `ready`/`connection_error`. */
  connect(cwd: string): Promise<void>;
  /** No-ops while disposed, disconnected, or a prior prompt is still in flight. */
  sendPrompt(text: string): void;
  /** Resolves the in-flight `session/request_permission`, if any. */
  respondPermission(optionId: string): void;
  /** Fire-and-forget `session/cancel`. */
  cancel(): void;
  /** Tears down: further async continuations become no-ops, any pending permission is rejected. */
  dispose(): void;
}

let idCounter = 0;
/** Monotonic id local to this module — unique per browser tab, which is all a client-side message/turn id needs to be. */
function nextId(prefix: string): string {
  idCounter += 1;
  return `${prefix}-${idCounter}`;
}

export function createAcpSessionController(client: AcpClient, dispatch: Dispatch): AcpSessionController {
  let sessionId: SessionId | null = null;
  let disposed = false;
  let promptInFlight = false;
  let permissionResolve: ((optionId: string) => void) | null = null;
  let permissionReject: ((err: Error) => void) | null = null;

  async function connect(cwd: string): Promise<void> {
    dispatch({ type: 'connecting' });
    try {
      const init = await client.initialize();
      const session = await client.newSession(cwd);
      if (disposed) return;
      sessionId = session.sessionId;
      dispatch({ type: 'ready', sessionId: session.sessionId, agentName: init.agentInfo?.name ?? null });
    } catch (err) {
      if (disposed) return;
      dispatch({ type: 'connection_error', message: err instanceof Error ? err.message : String(err) });
    }
  }

  function sendPrompt(text: string): void {
    if (disposed || promptInFlight || !sessionId || !text.trim()) return;
    const currentSessionId = sessionId;
    // Fallback grouping key for chunks that arrive without a `messageId` — one
    // assistant message per turn, per the client core's documented contract.
    const turnMessageId = nextId('assistant');

    dispatch({ type: 'user_message', id: nextId('user'), text });
    dispatch({ type: 'prompt_start' });
    promptInFlight = true;

    client
      .prompt(currentSessionId, [textContent(text)], {
        onMessageChunk: (chunkText, messageId) => {
          dispatch({ type: 'message_chunk', id: messageId ?? turnMessageId, text: chunkText });
        },
        onToolCall: event => dispatch({ type: 'tool_call', event }),
        onPlan: entries => dispatch({ type: 'plan', entries }),
        onUsage: usage => dispatch({ type: 'usage', usage }),
        onPermissionRequest: request =>
          new Promise<string>((resolve, reject) => {
            permissionResolve = resolve;
            permissionReject = reject;
            dispatch({ type: 'permission_request', request });
          }),
      })
      .then(() => {
        promptInFlight = false;
        if (disposed) return;
        dispatch({ type: 'prompt_end' });
      })
      .catch((err: unknown) => {
        promptInFlight = false;
        if (disposed) return;
        dispatch({ type: 'prompt_error', message: err instanceof Error ? err.message : String(err) });
      });
  }

  function respondPermission(optionId: string): void {
    const resolve = permissionResolve;
    permissionResolve = null;
    permissionReject = null;
    if (!resolve) return;
    dispatch({ type: 'permission_resolved' });
    resolve(optionId);
  }

  function cancel(): void {
    if (!sessionId) return;
    client.cancel(sessionId);
  }

  function dispose(): void {
    disposed = true;
    const reject = permissionReject;
    permissionResolve = null;
    permissionReject = null;
    // Let AcpClient answer the agent's request with outcome "cancelled"
    // (see client.ts's `handlePermissionRequest` catch) instead of hanging.
    reject?.(new Error('acp: session controller disposed'));
  }

  return { connect, sendPrompt, respondPermission, cancel, dispose };
}

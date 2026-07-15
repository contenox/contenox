import { useCallback, useEffect, useReducer, useRef } from 'react';
import { AcpClient, WebSocketTransport } from '../lib/acp';
import { getStoredApiToken } from '../lib/fetch';
import { createAcpSessionController, type AcpSessionController } from './acpSessionController';
import { acpSessionReducer, initialAcpSessionState, type AcpSessionState } from './acpSessionState';

/** `ws(s)://<host>/acp[?token=...]` — the browser can't set a WS Authorization header, so the stored API token (if any) travels as a query param instead. */
function buildAcpWsUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  const base = `${protocol}://${window.location.host}/acp`;
  const token = getStoredApiToken();
  return token ? `${base}?token=${encodeURIComponent(token)}` : base;
}

export interface UseAcpSessionResult extends AcpSessionState {
  sendPrompt: (text: string) => void;
  respondPermission: (optionId: string) => void;
  cancel: () => void;
}

/**
 * Owns one ACP client/session lifecycle over the browser `/acp` WebSocket
 * endpoint: on mount, connects, calls `initialize()` then `session/new('/')`,
 * and exposes the running turn's state — messages, tool calls, plan, usage,
 * and the permission gate — plus the three actions a chat surface needs.
 *
 * All protocol logic lives in `src/lib/acp`; this hook only wires it up (via
 * the framework-free `acpSessionController`) and re-renders on
 * `acpSessionReducer` state. It has no contenox-API coupling beyond reading
 * the stored bearer token for the WS URL.
 */
export function useAcpSession(): UseAcpSessionResult {
  const [state, dispatch] = useReducer(acpSessionReducer, initialAcpSessionState);
  const controllerRef = useRef<AcpSessionController | null>(null);

  useEffect(() => {
    const transport = new WebSocketTransport(buildAcpWsUrl());
    const client = new AcpClient(transport);
    const controller = createAcpSessionController(client, dispatch);
    controllerRef.current = controller;
    void controller.connect('/');

    return () => {
      controller.dispose();
      client.close();
      controllerRef.current = null;
    };
  }, []);

  const sendPrompt = useCallback((text: string) => {
    controllerRef.current?.sendPrompt(text);
  }, []);

  const respondPermission = useCallback((optionId: string) => {
    controllerRef.current?.respondPermission(optionId);
  }, []);

  const cancel = useCallback(() => {
    controllerRef.current?.cancel();
  }, []);

  return { ...state, sendPrompt, respondPermission, cancel };
}

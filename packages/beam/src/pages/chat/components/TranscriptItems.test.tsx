import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import { initialAcpSessionState, type AcpSessionState } from '../../../hooks/acpSessionState';
import i18n from '../../../i18n';
import { ThemeProvider } from '../../../lib/ThemeProvider';
import { TranscriptItems } from './TranscriptItems';

// Pin the language so labels are deterministic (the app default is German).
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

/**
 * Static-markup render (no `@testing-library/react` in this package — see
 * PermissionCard.test.tsx). `ThemeProvider` is required by the assistant
 * avatar hook and renders fine server-side (its `useSyncExternalStore` has a
 * server snapshot).
 */
function render(session: AcpSessionState): string {
  return renderToStaticMarkup(
    createElement(
      ThemeProvider,
      null,
      createElement(TranscriptItems, {
        session,
        agentName: 'contenox',
        onRespondPermission: vi.fn(),
      }),
    ),
  );
}

describe('TranscriptItems: image parts', () => {
  it("renders a user message's image parts as constrained data-URI thumbnails after its text", () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [{ kind: 'message', id: 'u1' }],
      messages: {
        u1: {
          id: 'u1',
          role: 'user',
          text: 'what is on this screenshot?',
          images: [{ data: 'aGVsbG8=', mimeType: 'image/png' }],
        },
      },
    };
    const html = render(session);
    expect(html).toContain('what is on this screenshot?');
    expect(html).toContain('src="data:image/png;base64,aGVsbG8="');
    // Localized labels flow into the shared image attachment renderer.
    expect(html).toContain('alt="Attached image"');
    expect(html).toContain('aria-label="Show image full size"');
  });

  it('renders an image-only message (replayed/adopted image prompt with no text)', () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [{ kind: 'message', id: 'replay-0' }],
      messages: {
        'replay-0': {
          id: 'replay-0',
          role: 'user',
          text: '',
          images: [{ data: 'aW1n', mimeType: 'image/jpeg' }],
        },
      },
    };
    const html = render(session);
    expect(html).toContain('src="data:image/jpeg;base64,aW1n"');
  });

  it('renders no attachment strip for a plain text message', () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [{ kind: 'message', id: 'u1' }],
      messages: { u1: { id: 'u1', role: 'user', text: 'no pictures' } },
    };
    const html = render(session);
    expect(html).toContain('no pictures');
    expect(html).not.toContain('data:image/');
  });
});

describe('TranscriptItems: failed-turn cards', () => {
  const CWD_REQUIRED_MESSAGE =
    'acpsvc: start agent "claude" instance: agentinstance: start agent "claude": agenthost: sandbox external ACP agent "/home/naro/.contenox/claude-code-acp.sh": cwd is required to confine the agent (the wall needs a workspace; it will not default to the whole filesystem)';

  it('skips the in-transcript card for the CURRENT session.error — it is already shown by the top ExecutionErrorBanner (ChatSessionTab), so rendering it here too would show the same failure twice', () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [{ kind: 'error', id: 'error-0' }],
      errorCards: { 'error-0': { id: 'error-0', message: CWD_REQUIRED_MESSAGE } },
      error: CWD_REQUIRED_MESSAGE,
    };
    const html = render(session);
    expect(html).not.toContain('This agent needs a workspace');
    expect(html).not.toContain('cwd is required to confine the agent');
  });

  it('renders the card once session.error is cleared (next prompt_start/session_reset) — history is preserved, with a legible headline instead of the raw wrapped wire string', () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [{ kind: 'error', id: 'error-0' }],
      errorCards: { 'error-0': { id: 'error-0', message: CWD_REQUIRED_MESSAGE } },
      error: null,
    };
    const html = render(session);
    expect(html).toContain('This agent needs a workspace');
    // The raw wire string stays available, just tucked behind the collapsed
    // detail toggle rather than dumped as the headline.
    expect(html).toContain('cwd is required to confine the agent');
  });

  it('keeps every OLDER failed turn visible even while the latest one is deduped against the top banner', () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [
        { kind: 'error', id: 'error-0' },
        { kind: 'error', id: 'error-1' },
      ],
      errorCards: {
        'error-0': { id: 'error-0', message: 'hook "mailing-service" returned HTTP 500' },
        'error-1': { id: 'error-1', message: CWD_REQUIRED_MESSAGE },
      },
      error: CWD_REQUIRED_MESSAGE,
    };
    const html = render(session);
    expect(html).toContain('This request failed'); // generic headline for error-0
    expect(html).not.toContain('This agent needs a workspace'); // error-1 deduped against the banner
  });
});

describe('TranscriptItems: turn-grouped step trail', () => {
  it("groups a turn's tool calls into a collapsible ABOVE the answer, instead of interleaving them as flat siblings below the streaming text", () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [
        { kind: 'tool_call', id: 'tc-1', turnId: 'turn-0' },
        { kind: 'tool_call', id: 'tc-2', turnId: 'turn-0' },
        { kind: 'message', id: 'a1', turnId: 'turn-0' },
      ],
      toolCalls: {
        'tc-1': { toolCallId: 'tc-1', title: 'ls', status: 'completed' },
        'tc-2': { toolCallId: 'tc-2', title: 'cat file', status: 'completed' },
      },
      messages: { a1: { id: 'a1', role: 'assistant', text: 'Here is the answer.' } },
    };
    const html = render(session);
    expect(html).toContain('2 steps'); // turn_steps_toggle
    expect(html.indexOf('2 steps')).toBeLessThan(html.indexOf('Here is the answer.'));
    expect(html.indexOf('ls')).toBeLessThan(html.indexOf('Here is the answer.'));
  });

  it('renders a turn with no tool calls exactly like before (no collapsible wrapper)', () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      items: [{ kind: 'message', id: 'a1', turnId: 'turn-0' }],
      messages: { a1: { id: 'a1', role: 'assistant', text: 'plain answer, no tools' } },
    };
    const html = render(session);
    expect(html).toContain('plain answer, no tools');
    expect(html).not.toContain('acp_chat.turn_steps_toggle');
  });

  it('a mid-stream turn (tool calls only, no answer text yet) renders the steps flat, not wrapped', () => {
    const session: AcpSessionState = {
      ...initialAcpSessionState,
      sessionId: 'sess-1',
      isPrompting: true,
      currentTurnId: 'turn-0',
      items: [{ kind: 'tool_call', id: 'tc-1', turnId: 'turn-0' }],
      toolCalls: { 'tc-1': { toolCallId: 'tc-1', title: 'ls', status: 'in_progress' } },
    };
    const html = render(session);
    expect(html).toContain('ls');
    expect(html).not.toContain('turn_steps_toggle');
  });
});

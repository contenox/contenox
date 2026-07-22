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

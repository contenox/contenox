import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import { AGENT_META_KEY } from '../../../lib/acp';
import { ChatConnectionBadge } from './ChatConnectionBadge';

/**
 * SSR coverage for the content lifted out of `AcpChatPage`'s deleted header
 * strip. `@testing-library/react` isn't a dependency here (see
 * PermissionCard.test.tsx), so this renders to static markup — enough to prove
 * the badge shows the connection status + attributed agent, and that it is a
 * compact inline chip, NOT a page header (the title moved to the navbar brand,
 * so nothing here renders an <h2>/<header> or the "beam" label). Unlike
 * PermissionCard this component reads context rather than props, so its one data
 * source — `useAcpWorkspace` — is stubbed through a hoisted mock;
 * `useStagedAgent` returns a safe no-op default outside its provider.
 */
// `vi.mock` is hoisted above the imports by vitest; the shared holder is created
// with `vi.hoisted` so the (also-hoisted) factory can read it.
const mock = vi.hoisted(() => ({ workspace: null as unknown }));
vi.mock('../../../hooks/useAcpWorkspace', () => ({
  useAcpWorkspace: () => ({ workspace: mock.workspace }),
}));

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function render(): string {
  return renderToStaticMarkup(createElement(ChatConnectionBadge));
}

describe('ChatConnectionBadge — extracted chat header content', () => {
  it('renders the connection status and the active session’s own agent', () => {
    mock.workspace = {
      status: 'ready',
      agentName: 'contenox',
      activeSessionId: 'sess-1',
      sessions: [{ sessionId: 'sess-1', _meta: { [AGENT_META_KEY]: 'researcher' } }],
    };
    const html = render();
    expect(html).toContain('Connected'); // acp_chat.status_ready
    expect(html).toContain('researcher'); // the focused session's own agent wins
    expect(html).not.toContain('contenox'); // …over the workspace-level agent
  });

  it('falls back to the workspace agent on the empty surface (no active session)', () => {
    mock.workspace = {
      status: 'ready',
      agentName: 'contenox',
      activeSessionId: null,
      sessions: [],
    };
    const html = render();
    expect(html).toContain('Connected');
    expect(html).toContain('contenox');
  });

  it('is a compact chip, not a page header — no title, <h2>, or <header>', () => {
    mock.workspace = { status: 'ready', agentName: null, activeSessionId: null, sessions: [] };
    const html = render();
    expect(html).toContain('Connected');
    expect(html).not.toContain('beam'); // the surface title moved to the navbar brand
    expect(html).not.toContain('<h2');
    expect(html).not.toContain('<header');
  });

  it('reflects a non-ready status (e.g. reconnecting)', () => {
    mock.workspace = { status: 'reconnecting', agentName: null, activeSessionId: null, sessions: [] };
    const html = render();
    expect(html).toContain('Reconnecting'); // acp_chat.status_reconnecting
  });
});

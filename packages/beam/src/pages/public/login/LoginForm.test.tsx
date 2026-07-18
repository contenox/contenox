import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../../../i18n';
import { AuthContext, type AuthContextType } from '../../../lib/authContext';
import { AccessSettingsSection } from '../../admin/settings/AccessSettingsSection';
import AuthPage from './AuthPage';

// Pin the language so label assertions are deterministic (the app default is
// German). @testing-library/react is not a dependency here (see the sibling
// ApprovalViewTab.test.tsx) — these render to static markup, enough to prove
// the gate shows the token form and the logout affordance appears only when
// login is required.
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

function render(node: Parameters<typeof renderToStaticMarkup>[0]): string {
  const client = new QueryClient();
  return renderToStaticMarkup(createElement(QueryClientProvider, { client }, node));
}

function withAuth(value: Partial<AuthContextType>, node: React.ReactNode): React.ReactElement {
  const base: AuthContextType = {
    user: undefined,
    isLoading: false,
    isError: false,
    error: null,
    authRequired: false,
    authenticated: false,
    refresh: () => {},
  };
  return createElement(AuthContext.Provider, { value: { ...base, ...value } }, node);
}

describe('remote-access login gate', () => {
  it('AuthPage renders a masked Access token field and a submit button', () => {
    const html = render(createElement(AuthPage));
    expect(html).toContain('Access token');
    expect(html).toContain('type="password"');
    expect(html).toContain('Login');
  });
});

describe('AccessSettingsSection logout affordance', () => {
  it('renders a sign-out button when login is required', () => {
    const html = render(withAuth({ authRequired: true }, createElement(AccessSettingsSection)));
    expect(html).toContain('Sign out');
    expect(html).toContain('Remote access');
  });

  it('renders nothing when login is not required (local, no token)', () => {
    const html = render(withAuth({ authRequired: false }, createElement(AccessSettingsSection)));
    expect(html).toBe('');
  });
});

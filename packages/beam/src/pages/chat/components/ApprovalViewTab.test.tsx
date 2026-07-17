import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it, vi } from 'vitest';
import i18n from '../../../i18n';
import type { RequestPermissionRequest } from '../../../lib/acp';
import { ApprovalViewTab } from './ApprovalViewTab';

// Pin the language so banner-text assertions are deterministic (the app default
// is German). Option/title/diff text below come from the request, not i18n.
beforeAll(async () => {
  await i18n.changeLanguage('en');
});

/**
 * `@testing-library/react` is not a dependency of `packages/beam` (see
 * acpWorkspaceController.test.ts). These tests render the component to static
 * markup via `react-dom/server` — enough to prove the payload renders and that
 * the pending → resolved transition swaps the LIVE Allow/Deny actions for a
 * read-only banner, i.e. never a live Allow button on a dead request.
 */
function diffRequest(): RequestPermissionRequest {
  return {
    sessionId: 'sess-1',
    toolCall: {
      toolCallId: 'call-1',
      title: 'Edit config',
      kind: 'edit',
      content: [{ type: 'diff', path: 'src/config.ts', oldText: 'const a = 1;', newText: 'const a = 2;' }],
      rawInput: { path: 'src/config.ts' },
    },
    options: [
      { optionId: 'allow', name: 'Allow once', kind: 'allow_once' },
      { optionId: 'deny', name: 'Deny once', kind: 'reject_once' },
    ],
  };
}

function render(node: Parameters<typeof renderToStaticMarkup>[0]): string {
  return renderToStaticMarkup(node);
}

describe('ApprovalViewTab', () => {
  it('renders the diff payload, raw input, and the live Allow/Deny actions while pending', () => {
    const req = diffRequest();
    const html = render(
      createElement(ApprovalViewTab, {
        permission: req,
        pendingPermission: req, // live: this snapshot IS the pending request
        active: false,
        onRespond: vi.fn(),
        onClose: vi.fn(),
      }),
    );
    // Payload: title, the diff's old/new lines, the diff file path, raw input.
    expect(html).toContain('Edit config');
    expect(html).toContain('const a = 1;');
    expect(html).toContain('const a = 2;');
    expect(html).toContain('src/config.ts');
    // Live respond actions present.
    expect(html).toContain('Allow once');
    expect(html).toContain('Deny once');
    // No resolved banner yet.
    expect(html).not.toContain('This request is no longer pending');
  });

  it('renders a resolved (stale) banner and NO live Allow button once the request left the pending slot', () => {
    const req = diffRequest();
    const html = render(
      createElement(ApprovalViewTab, {
        permission: req,
        pendingPermission: null, // resolved elsewhere / cancelled
        active: false,
        onRespond: vi.fn(),
        onClose: vi.fn(),
      }),
    );
    // Still shows the payload for reading…
    expect(html).toContain('Edit config');
    expect(html).toContain('const a = 2;');
    // …but the live actions are gone, replaced by the resolved banner + Close.
    expect(html).not.toContain('Allow once');
    expect(html).not.toContain('Deny once');
    expect(html).toContain('This request is no longer pending');
    expect(html).toContain('Close');
  });

  it('renders a resolved (stale) banner when the pending slot holds a DIFFERENT tool call', () => {
    const req = diffRequest();
    const other: RequestPermissionRequest = { ...req, toolCall: { ...req.toolCall, toolCallId: 'call-2' } };
    const html = render(
      createElement(ApprovalViewTab, {
        permission: req,
        pendingPermission: other,
        active: false,
        onRespond: vi.fn(),
        onClose: vi.fn(),
      }),
    );
    expect(html).not.toContain('Allow once');
    expect(html).toContain('This request is no longer pending');
  });
});

import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../../i18n';
import type { WorkspaceRoot } from '../../lib/types';
import { RootChip } from './RootChip';
import { RootSelector } from './RootSelector';
import { WorkspaceBoundaryNotice } from './WorkspaceBoundaryNotice';

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

const roots: WorkspaceRoot[] = [
  { path: '/home/user/project', default: true },
  { path: '/tmp/scratch', default: false },
];

describe('RootChip', () => {
  it('renders the shortened path and the default marker for the default root', () => {
    const html = renderToStaticMarkup(createElement(RootChip, { root: roots[0] }));
    expect(html).toContain('project');
    expect(html).toContain('default');
  });

  it('renders nothing when there is no root (nil-gated)', () => {
    expect(renderToStaticMarkup(createElement(RootChip, { root: undefined }))).toBe('');
  });
});

describe('RootSelector', () => {
  it('degrades to a plain free-path input when the allowlist is absent', () => {
    const html = renderToStaticMarkup(
      createElement(RootSelector, { value: '', onChange: () => {}, roots: [], isAbsent: true }),
    );
    expect(html).toContain('Absolute path inside a permitted root');
    // No <select> when there is nothing to pick from.
    expect(html).not.toContain('<select');
  });

  it('offers each root plus a custom-path option when the allowlist is present', () => {
    const html = renderToStaticMarkup(
      createElement(RootSelector, { value: '', onChange: () => {}, roots, isAbsent: false }),
    );
    expect(html).toContain('<select');
    expect(html).toContain('project');
    expect(html).toContain('scratch');
    expect(html).toContain('Custom path…');
  });

  it('reveals the free-path input when seeded with a path outside the offered roots', () => {
    const html = renderToStaticMarkup(
      createElement(RootSelector, {
        value: '/some/other/place',
        onChange: () => {},
        roots,
        isAbsent: false,
      }),
    );
    expect(html).toContain('Absolute path inside a permitted root');
  });
});

describe('WorkspaceBoundaryNotice', () => {
  it('replaces the raw 422 with the designed refusal naming the allowed roots', () => {
    const html = renderToStaticMarkup(
      createElement(WorkspaceBoundaryNotice, {
        message: 'unprocessable entity: workspace root "/etc" is not permitted',
        roots,
        onRetry: () => {},
      }),
    );
    expect(html).toContain('outside the permitted roots');
    expect(html).toContain('/etc');
    expect(html).toContain('Allowed roots');
    expect(html).toContain('project');
    // The raw wire prefix is not shown.
    expect(html).not.toContain('unprocessable entity');
  });

  it('keeps the plain error notice for an unrelated failure', () => {
    const html = renderToStaticMarkup(
      createElement(WorkspaceBoundaryNotice, {
        message: 'network unreachable',
        roots,
        onRetry: () => {},
      }),
    );
    expect(html).toContain('network unreachable');
    expect(html).not.toContain('Allowed roots');
  });
});

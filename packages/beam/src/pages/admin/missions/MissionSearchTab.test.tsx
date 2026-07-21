import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { beforeAll, describe, expect, it } from 'vitest';
import i18n from '../../../i18n';
import type { MissionChangedFile } from '../../../lib/types';
import type { WorkspaceSearchState } from '../../../hooks/useWorkspaceSearch';
import { SearchTabView, type SearchTabViewProps } from './MissionSearchTab';

beforeAll(async () => {
  await i18n.changeLanguage('en');
});

const IDLE: WorkspaceSearchState = { status: 'idle', groups: [], matchCount: 0, truncated: false };

function render(state: WorkspaceSearchState, over: Partial<SearchTabViewProps> = {}): string {
  const props: SearchTabViewProps = {
    state,
    query: over.query ?? 'needle',
    onQueryChange: () => {},
    root: over.root,
    roots: over.roots ?? [],
    changedFiles: over.changedFiles ?? [],
    onOpenInChanges: () => {},
    onRetry: () => {},
    ...over,
  };
  return renderToStaticMarkup(createElement(SearchTabView, props));
}

describe('SearchTabView — terminal states', () => {
  it('renders the ripgrep teaching state for a 501 dependency refusal', () => {
    const html = render({ ...IDLE, status: 'dependency', errorMessage: 'needs rg' });
    expect(html).toContain('Search needs ripgrep');
  });

  it('renders the boundary notice for a 422 root refusal, reusing the designed copy', () => {
    const html = render({
      ...IDLE,
      status: 'refusal',
      refusalMessage: 'workspace root "/nope" is not permitted',
    });
    expect(html).toContain('outside the permitted roots');
  });

  it('offers "refine your search" when the result set was truncated', () => {
    const html = render({
      status: 'done',
      truncated: true,
      matchCount: 1,
      groups: [
        { path: 'src/a.ts', matches: [{ path: 'src/a.ts', line: 1, column: 0, length: 6, preview: 'needle' }] },
      ],
    });
    expect(html).toContain('refine your search');
  });

  it('says "no matches" honestly on an empty done', () => {
    expect(render({ ...IDLE, status: 'done' })).toContain('No matches');
  });

  it('prompts to type when idle', () => {
    expect(render(IDLE, { query: '' })).toContain('Type to search');
  });
});

describe('SearchTabView — results and click-through', () => {
  const groups = [
    {
      path: 'src/app.ts',
      matches: [{ path: 'src/app.ts', line: 12, column: 4, length: 6, preview: '    needle here' }],
    },
    {
      path: 'docs/x.md',
      matches: [{ path: 'docs/x.md', line: 1, column: 0, length: 6, preview: 'needle' }],
    },
  ];

  it('highlights the matched byte-range and shows per-file count badges', () => {
    const html = render({ status: 'done', truncated: false, matchCount: 2, groups });
    expect(html).toContain('<mark');
    expect(html).toContain('needle');
    // the line number is shown
    expect(html).toContain('12');
  });

  it('offers "Open in Changes" only for a hit that is one of the changed files', () => {
    const changedFiles: MissionChangedFile[] = [
      { path: '/repo/src/app.ts', status: 'modified', score: 9 },
    ];
    const html = render(
      { status: 'done', truncated: false, matchCount: 2, groups },
      { changedFiles, root: { path: '/repo', default: true } },
    );
    // src/app.ts is a changed file → the affordance appears exactly once.
    expect(html.match(/Open in Changes/g)?.length).toBe(1);
  });
});

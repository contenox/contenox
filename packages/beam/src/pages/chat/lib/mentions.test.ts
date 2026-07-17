import { describe, expect, it } from 'vitest';
import {
  activeMentions,
  applyMention,
  applyMentionAscend,
  applyMentionFolder,
  deriveMentionQuery,
  filterMentionFiles,
  mentionCandidatesForScope,
  mentionMenuKeyFromEvent,
  mentionPreviewPath,
  mentionToken,
  promptBlocksFromDraft,
  splitMentionQuery,
  type MentionCandidate,
  type WorkspaceFileRef,
} from './mentions';

const files: WorkspaceFileRef[] = [
  { path: 'main.go', name: 'main.go' },
  { path: 'src/app.ts', name: 'app.ts' },
  { path: 'README.md', name: 'README.md' },
];

describe('deriveMentionQuery', () => {
  it('activates on a bare @ at the start', () => {
    expect(deriveMentionQuery('@', 1)).toEqual({ active: true, query: '', start: 0, end: 1 });
  });
  it('captures the query after @', () => {
    const q = deriveMentionQuery('look at @src/ap', 15);
    expect(q).toMatchObject({ active: true, query: 'src/ap', start: 8, end: 15 });
  });
  it('does not activate mid-word (no whitespace before @)', () => {
    expect(deriveMentionQuery('email@host', 10).active).toBe(false);
  });
  it('closes once a space follows the token', () => {
    expect(deriveMentionQuery('@main.go ', 9).active).toBe(false);
  });
});

describe('filterMentionFiles', () => {
  it('returns all for an empty query', () => {
    expect(filterMentionFiles(files, '')).toHaveLength(3);
  });
  it('ranks name-prefix matches first', () => {
    const out = filterMentionFiles(files, 'app');
    expect(out[0].path).toBe('src/app.ts');
  });
  it('matches by path segment', () => {
    expect(filterMentionFiles(files, 'src/')).toEqual([{ path: 'src/app.ts', name: 'app.ts' }]);
  });
});

describe('applyMention', () => {
  it('replaces the query span with the token and a trailing space', () => {
    const text = 'see @ap';
    const q = deriveMentionQuery(text, text.length);
    const res = applyMention(text, q, { path: 'src/app.ts', name: 'app.ts' });
    expect(res.text).toBe('see @src/app.ts ');
    expect(res.caret).toBe(res.text.length);
  });
});

describe('activeMentions', () => {
  it('keeps only mentions whose token is still present, de-duplicated', () => {
    const selected: WorkspaceFileRef[] = [
      { path: 'main.go', name: 'main.go' },
      { path: 'main.go', name: 'main.go' },
      { path: 'src/app.ts', name: 'app.ts' },
    ];
    const out = activeMentions('check @main.go now', selected);
    expect(out).toEqual([{ path: 'main.go', name: 'main.go' }]);
  });
});

describe('promptBlocksFromDraft', () => {
  it('emits a text block plus one resource_link per mention, no embeds', () => {
    const blocks = promptBlocksFromDraft('review @main.go', [{ path: 'main.go', name: 'main.go' }]);
    expect(blocks).toEqual([
      { type: 'text', text: 'review @main.go' },
      { type: 'resource_link', name: 'main.go', uri: 'main.go' },
    ]);
    // Reference-only: never an embedded resource block.
    expect(blocks.some(b => b.type === 'resource')).toBe(false);
  });
  it('de-duplicates mentions by path', () => {
    const blocks = promptBlocksFromDraft('x', [
      { path: 'a.txt', name: 'a.txt' },
      { path: 'a.txt', name: 'a.txt' },
    ]);
    expect(blocks.filter(b => b.type === 'resource_link')).toHaveLength(1);
  });
  it('omits the text block when the draft is blank but keeps mentions', () => {
    const blocks = promptBlocksFromDraft('   ', [{ path: 'a.txt', name: 'a.txt' }]);
    expect(blocks).toEqual([{ type: 'resource_link', name: 'a.txt', uri: 'a.txt' }]);
  });
});

describe('mentionToken', () => {
  it('prefixes the path with @', () => {
    expect(mentionToken({ path: 'src/app.ts', name: 'app.ts' })).toBe('@src/app.ts');
  });
});

describe('mentionMenuKeyFromEvent — file-tree keyboard nav', () => {
  it('maps vertical arrows to selection moves', () => {
    expect(mentionMenuKeyFromEvent('ArrowDown')).toEqual({ type: 'move', delta: 1 });
    expect(mentionMenuKeyFromEvent('ArrowUp')).toEqual({ type: 'move', delta: -1 });
  });
  it('maps → to drill (into a folder) and ← to ascend (to the parent)', () => {
    expect(mentionMenuKeyFromEvent('ArrowRight')).toEqual({ type: 'drill' });
    expect(mentionMenuKeyFromEvent('ArrowLeft')).toEqual({ type: 'ascend' });
  });
  it('maps Enter/Tab to accept and Escape to close', () => {
    expect(mentionMenuKeyFromEvent('Enter')).toEqual({ type: 'accept' });
    expect(mentionMenuKeyFromEvent('Tab')).toEqual({ type: 'accept' });
    expect(mentionMenuKeyFromEvent('Escape')).toEqual({ type: 'close' });
  });
  it('ignores other keys so the composer handles them', () => {
    expect(mentionMenuKeyFromEvent('a')).toBeNull();
    expect(mentionMenuKeyFromEvent('Home')).toBeNull();
  });
});

describe('applyMentionAscend', () => {
  it('goes from a subdirectory up to its parent, dropping the leaf filter', () => {
    const draft = 'x @src/components/ind';
    const q = deriveMentionQuery(draft, draft.length);
    const { text, caret } = applyMentionAscend(draft, q);
    expect(text).toBe('x @src/');
    expect(splitMentionQuery(deriveMentionQuery(text, caret).query)).toEqual({ dir: 'src', leaf: '' });
  });
  it('ascends from a top-level directory to the root (bare @)', () => {
    const draft = 'x @src/comp';
    const q = deriveMentionQuery(draft, draft.length);
    const { text } = applyMentionAscend(draft, q);
    expect(text).toBe('x @');
  });
  it('keeps quoting when the parent path has spaces', () => {
    const draft = 'x @"my dir/sub/leaf';
    const q = deriveMentionQuery(draft, draft.length);
    const { text } = applyMentionAscend(draft, q);
    expect(text).toBe('x @"my dir/');
  });
});

describe('deriveMentionQuery — quoted paths with spaces', () => {
  it('is active while typing inside an open quote, query excludes the quote', () => {
    const draft = 'look at @"my fil';
    const q = deriveMentionQuery(draft, draft.length);
    expect(q.active).toBe(true);
    expect(q.query).toBe('my fil');
    expect(draft.slice(q.start, q.end)).toBe('@"my fil');
  });

  it('goes inactive once the quote is closed (typing past it does not re-open)', () => {
    const draft = 'look at @"my file.txt"';
    expect(deriveMentionQuery(draft, draft.length).active).toBe(false);
  });

  it('splits a quoted spaced directory query for browsing', () => {
    const draft = 'x @"my dir/';
    const q = deriveMentionQuery(draft, draft.length);
    expect(q.active).toBe(true);
    expect(splitMentionQuery(q.query)).toEqual({ dir: 'my dir', leaf: '' });
  });
});

describe('mentionToken — quoting', () => {
  it('leaves a space-free path bare', () => {
    expect(mentionToken({ path: 'src/app.ts', name: 'app.ts' })).toBe('@src/app.ts');
  });
  it('quotes a path containing a space', () => {
    expect(mentionToken({ path: 'my docs/plan a.md', name: 'plan a.md' })).toBe('@"my docs/plan a.md"');
  });
});

describe('applyMention / applyMentionFolder — spaced paths', () => {
  it('inserts a quoted closed token for a spaced file, so activeMentions still finds it', () => {
    const file = { path: 'my docs/plan a.md', name: 'plan a.md' };
    const draft = 'see @"my docs/pl';
    const q = deriveMentionQuery(draft, draft.length);
    const { text } = applyMention(draft, q, file);
    expect(text).toBe('see @"my docs/plan a.md" ');
    expect(activeMentions(text, [file])).toEqual([file]);
    expect(promptBlocksFromDraft(text, activeMentions(text, [file]))).toEqual([
      { type: 'text', text },
      { type: 'resource_link', name: 'my docs/plan a.md', uri: 'my docs/plan a.md' },
    ]);
  });

  it('drills a spaced directory with an open quote so browsing continues inside it', () => {
    const draft = 'x @my';
    const q = deriveMentionQuery(draft, draft.length);
    const { text, caret } = applyMentionFolder(draft, q, { path: 'my dir', name: 'my dir' });
    expect(text).toBe('x @"my dir/');
    const q2 = deriveMentionQuery(text, caret);
    expect(q2.active).toBe(true);
    expect(splitMentionQuery(q2.query)).toEqual({ dir: 'my dir', leaf: '' });
  });
});

describe('splitMentionQuery', () => {
  it('treats a slash-free query as a root-scoped leaf filter', () => {
    expect(splitMentionQuery('READ')).toEqual({ dir: '', leaf: 'READ' });
  });
  it('splits a dir/leaf query at the last slash', () => {
    expect(splitMentionQuery('src/comp')).toEqual({ dir: 'src', leaf: 'comp' });
  });
  it('treats a trailing slash as browsing a directory with no filter', () => {
    expect(splitMentionQuery('src/')).toEqual({ dir: 'src', leaf: '' });
  });
  it('handles nested directories', () => {
    expect(splitMentionQuery('src/components/ind')).toEqual({ dir: 'src/components', leaf: 'ind' });
  });
});

describe('mentionCandidatesForScope', () => {
  const entries: MentionCandidate[] = [
    { path: 'src/z.ts', name: 'z.ts', isDirectory: false },
    { path: 'src/components', name: 'components', isDirectory: true },
    { path: 'src/app.ts', name: 'app.ts', isDirectory: false },
    { path: 'src/assets', name: 'assets', isDirectory: true },
  ];

  it('lists directories before files, each group alphabetical, for an empty leaf', () => {
    expect(mentionCandidatesForScope(entries, '').map(e => e.name)).toEqual(['assets', 'components', 'app.ts', 'z.ts']);
  });

  it('filters by leaf across both groups, prefix-matches ranked first', () => {
    expect(mentionCandidatesForScope(entries, 'a').map(e => e.name)).toEqual(['assets', 'app.ts']);
  });

  it('returns nothing when the leaf matches neither files nor dirs', () => {
    expect(mentionCandidatesForScope(entries, 'zzz')).toEqual([]);
  });
});

describe('mentionPreviewPath', () => {
  const entries: MentionCandidate[] = [
    { path: 'src', name: 'src', isDirectory: true },
    { path: 'README.md', name: 'README.md', isDirectory: false },
  ];
  it('returns the highlighted file path', () => {
    expect(mentionPreviewPath(entries, 1)).toBe('README.md');
  });
  it('returns null when the highlighted entry is a directory', () => {
    expect(mentionPreviewPath(entries, 0)).toBeNull();
  });
  it('returns null when the index is out of range', () => {
    expect(mentionPreviewPath(entries, 5)).toBeNull();
    expect(mentionPreviewPath([], 0)).toBeNull();
  });
});

describe('applyMentionFolder', () => {
  it('rewrites the active @query to `@dir/` with no trailing space so browsing continues', () => {
    const draft = 'look at @sr';
    const q = deriveMentionQuery(draft, draft.length);
    const { text, caret } = applyMentionFolder(draft, q, { path: 'src', name: 'src' });
    expect(text).toBe('look at @src/');
    expect(caret).toBe(text.length);
    // the new draft still parses as an active query scoped into src
    const q2 = deriveMentionQuery(text, caret);
    expect(splitMentionQuery(q2.query)).toEqual({ dir: 'src', leaf: '' });
  });

  it('uses the full nested path when drilling deeper', () => {
    const draft = 'x @src/comp';
    const q = deriveMentionQuery(draft, draft.length);
    const { text } = applyMentionFolder(draft, q, { path: 'src/components', name: 'components' });
    expect(text).toBe('x @src/components/');
  });
});

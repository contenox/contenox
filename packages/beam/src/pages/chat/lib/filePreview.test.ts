import { describe, expect, it } from 'vitest';
import { previewEntryFromPeek, selectPreview, type FilePreviewCache } from './filePreview';

describe('previewEntryFromPeek', () => {
  it('maps a text peek to a text entry', () => {
    expect(previewEntryFromPeek({ text: 'hello', isBinary: false })).toEqual({ kind: 'text', text: 'hello' });
  });
  it('maps a binary peek to a binary entry, dropping any text', () => {
    expect(previewEntryFromPeek({ text: '', isBinary: true })).toEqual({ kind: 'binary' });
  });
});

describe('selectPreview', () => {
  const cache: FilePreviewCache = {
    'a.txt': { kind: 'text', text: 'aaa' },
    'img.png': { kind: 'binary' },
    'boom.txt': { kind: 'error' },
  };

  it('is hidden when nothing is highlighted', () => {
    expect(selectPreview(null, cache)).toEqual({ status: 'hidden' });
  });
  it('is loading while the highlighted path has not resolved yet', () => {
    expect(selectPreview('pending.txt', cache)).toEqual({ status: 'loading', path: 'pending.txt' });
  });
  it('renders resolved text', () => {
    expect(selectPreview('a.txt', cache)).toEqual({ status: 'text', path: 'a.txt', text: 'aaa' });
  });
  it('renders a binary state', () => {
    expect(selectPreview('img.png', cache)).toEqual({ status: 'binary', path: 'img.png' });
  });
  it('renders an error state', () => {
    expect(selectPreview('boom.txt', cache)).toEqual({ status: 'error', path: 'boom.txt' });
  });

  it('only the highlighted path drives the display (out-of-order fetches are inert)', () => {
    // A late fetch for a file the user already left writes its own key but does
    // not change what is shown for the currently highlighted path.
    const withLate: FilePreviewCache = { ...cache, 'late.txt': { kind: 'text', text: 'late' } };
    expect(selectPreview('a.txt', withLate)).toEqual({ status: 'text', path: 'a.txt', text: 'aaa' });
  });
});

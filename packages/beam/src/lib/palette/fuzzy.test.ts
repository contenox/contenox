import { describe, expect, it } from 'vitest';
import { buildNormalized, createSearcher, isCaseSensitive, type WeightedField } from './fuzzy';

type Row = { title: string; id: string; keyword?: string };

const toFields = (r: Row): WeightedField[] => {
  const fields: WeightedField[] = [{ text: r.title, weight: 4, highlight: true }];
  if (r.keyword) fields.push({ text: r.keyword, weight: 3 });
  fields.push({ text: r.id, weight: 1 });
  return fields;
};

const searchTitles = (rows: Row[], query: string) =>
  createSearcher(rows, toFields)
    .search(query)
    .map(m => m.item.title);

describe('buildNormalized', () => {
  it('strips diacritics while keeping a 1:1 index map for ASCII', () => {
    const { norm, map } = buildNormalized('Café');
    expect(norm).toBe('Cafe');
    expect(map).toEqual([0, 1, 2, 3]);
  });

  it('preserves case (case is decided at query time, not index time)', () => {
    expect(buildNormalized('München').norm).toBe('Munchen');
  });
});

describe('isCaseSensitive (smart-case)', () => {
  it('is case-insensitive for an all-lowercase query', () => {
    expect(isCaseSensitive('researcher')).toBe(false);
  });
  it('is case-sensitive once any uppercase letter appears', () => {
    expect(isCaseSensitive('Researcher')).toBe(true);
  });
});

describe('createSearcher — field weights', () => {
  it('ranks a title match above an id-only match for the same query', () => {
    const rows: Row[] = [
      { title: 'zzz', id: 'Deploy' }, // matches only via the low-weight id
      { title: 'Deploy', id: 'zzz' }, // matches via the high-weight title
    ];
    expect(searchTitles(rows, 'Deploy')[0]).toBe('Deploy');
  });
});

describe('createSearcher — multi-token AND', () => {
  const rows: Row[] = [{ title: 'nightly build failure', id: 'm1' }];

  it('matches when every token is found somewhere', () => {
    expect(searchTitles(rows, 'nightly failure')).toEqual(['nightly build failure']);
  });

  it('excludes the item when any token matches nothing', () => {
    expect(searchTitles(rows, 'nightly zzzz')).toEqual([]);
  });

  it('sums token scores (a keyword token still counts toward AND)', () => {
    const withKeyword: Row[] = [{ title: 'build', id: 'm2', keyword: 'nightly' }];
    expect(searchTitles(withKeyword, 'build nightly')).toEqual(['build']);
  });
});

describe('createSearcher — smart-case', () => {
  const rows: Row[] = [{ title: 'MyAgent', id: 'a1' }];

  it('matches an all-lowercase query case-insensitively', () => {
    expect(searchTitles(rows, 'ma')).toEqual(['MyAgent']);
  });

  it('matches a correctly-cased query', () => {
    expect(searchTitles(rows, 'MA')).toEqual(['MyAgent']);
  });

  it('rejects a mixed-case query whose case does not line up', () => {
    // 'Ma' is case-sensitive (has an uppercase M); the 'a' cannot match the 'A'.
    expect(searchTitles(rows, 'Ma')).toEqual([]);
  });
});

describe('createSearcher — diacritics', () => {
  const rows: Row[] = [{ title: 'München', id: 'city' }];

  it('reaches an accented title from an unaccented, lowercase query', () => {
    expect(searchTitles(rows, 'munchen')).toEqual(['München']);
  });

  it('honors smart-case against the stripped form', () => {
    expect(searchTitles(rows, 'Munchen')).toEqual(['München']);
  });
});

describe('createSearcher — highlight indexes', () => {
  it('returns the matched title positions', () => {
    const rows: Row[] = [{ title: 'Researcher', id: 'r' }];
    const [match] = createSearcher(rows, toFields).search('res');
    expect(match.matchIndexes).toEqual([0, 1, 2]);
  });

  it('remaps highlight positions across a stripped diacritic', () => {
    const rows: Row[] = [{ title: 'Café', id: 'c' }];
    const [match] = createSearcher(rows, toFields).search('cafe');
    // The 'e' matched the normalized index that maps back to 'é' at 3.
    expect(match.matchIndexes).toContain(3);
  });

  it('does not highlight when only a non-title field matched', () => {
    const rows: Row[] = [{ title: 'zzz', id: 'deploy' }];
    const [match] = createSearcher(rows, toFields).search('deploy');
    expect(match.matchIndexes).toEqual([]);
  });
});

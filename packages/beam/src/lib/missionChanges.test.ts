import { describe, expect, it } from 'vitest';
import {
  basename,
  CHANGE_STATUS_BADGE_VARIANT,
  CHANGE_STATUS_LABEL_KEY,
  dirname,
  languageForPath,
  matchChangedFile,
} from './missionChanges';
import type { MissionChangedFile } from './types';

describe('basename / dirname', () => {
  it('splits an absolute path into file and directory', () => {
    expect(basename('/home/x/src/app.tsx')).toBe('app.tsx');
    expect(dirname('/home/x/src/app.tsx')).toBe('/home/x/src');
  });
  it('handles a bare file name and trailing slashes', () => {
    expect(basename('README.md')).toBe('README.md');
    expect(dirname('README.md')).toBe('');
    expect(basename('/a/b/')).toBe('b');
  });
});

describe('languageForPath', () => {
  it('maps common extensions to Monaco language ids', () => {
    expect(languageForPath('/x/app.tsx')).toBe('typescript');
    expect(languageForPath('main.go')).toBe('go');
    expect(languageForPath('/etc/config.yaml')).toBe('yaml');
    expect(languageForPath('Dockerfile')).toBe('dockerfile');
  });
  it('falls back to plaintext for unknown or extensionless files', () => {
    expect(languageForPath('LICENSE')).toBe('plaintext');
    expect(languageForPath('/x/data.unknownext')).toBe('plaintext');
    expect(languageForPath('/x/.gitignore')).toBe('plaintext');
  });
});

describe('status maps', () => {
  it('gives each status a distinct badge variant and a label key', () => {
    expect(CHANGE_STATUS_BADGE_VARIANT.added).toBe('success');
    expect(CHANGE_STATUS_BADGE_VARIANT.modified).toBe('warning');
    expect(CHANGE_STATUS_BADGE_VARIANT.deleted).toBe('error');
    expect(CHANGE_STATUS_LABEL_KEY.deleted).toBe('changes.status_deleted');
  });
});

describe('matchChangedFile', () => {
  const files: MissionChangedFile[] = [
    { path: '/repo/src/app.tsx', status: 'modified', score: 9 },
    { path: '/repo/README.md', status: 'added', score: 3 },
  ];

  it('resolves a root-relative hit to the changed file by joining the root', () => {
    expect(matchChangedFile(files, 'src/app.tsx', '/repo')?.path).toBe('/repo/src/app.tsx');
    // A leading "./" on the hit path is tolerated.
    expect(matchChangedFile(files, './README.md', '/repo')?.path).toBe('/repo/README.md');
  });

  it('falls back to a suffix match when the root is unknown', () => {
    expect(matchChangedFile(files, 'src/app.tsx')?.path).toBe('/repo/src/app.tsx');
  });

  it('returns undefined for a hit that is not one of the changed files (inline context only)', () => {
    expect(matchChangedFile(files, 'src/other.ts', '/repo')).toBeUndefined();
  });
});

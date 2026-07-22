import type { TranslationKey } from '../i18n';
import type { MissionChangeStatus, MissionChangedFile } from './types';

/**
 * Pure, DOM-free helpers for the mission Changes tab (Arc 1 of
 * ide-workflows.md) — the changed-files list ordered by Degree-of-Interest and
 * its per-file diffs. Kept out of the components so the presentation decisions
 * (how a status reads, which Monaco language a path maps to, whether a search
 * hit is one of the changed files) are unit-testable without a component, a
 * live server, or Monaco (matches unitStatus.ts / workspaceRoots.ts).
 */

/** Changed-file status → its badge variant. Deleted reads loudest, added as a
 *  success-ish note, modified neutral — never mistakable at a glance. */
export const CHANGE_STATUS_BADGE_VARIANT: Record<
  MissionChangeStatus,
  'success' | 'warning' | 'error'
> = {
  added: 'success',
  modified: 'warning',
  deleted: 'error',
};

export const CHANGE_STATUS_LABEL_KEY: Record<MissionChangeStatus, TranslationKey> = {
  added: 'changes.status_added',
  modified: 'changes.status_modified',
  deleted: 'changes.status_deleted',
};

/** The last path segment of an absolute or relative path (the file name). */
export function basename(path: string): string {
  const trimmed = path.replace(/\/+$/, '');
  const idx = trimmed.lastIndexOf('/');
  return idx === -1 ? trimmed : trimmed.slice(idx + 1);
}

/** The directory portion of a path (everything before the file name), or '' at the root. */
export function dirname(path: string): string {
  const trimmed = path.replace(/\/+$/, '');
  const idx = trimmed.lastIndexOf('/');
  return idx <= 0 ? '' : trimmed.slice(0, idx);
}

/**
 * A Monaco language id inferred from a path's extension, for syntax colouring in
 * the diff view. Deliberately a small, common map — an unknown extension yields
 * `plaintext`, which Monaco renders fine; this is cosmetic enhancement, never a
 * correctness dependency (the instant-feel law: the diff is legible plain).
 */
const EXT_LANGUAGE: Record<string, string> = {
  ts: 'typescript',
  tsx: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  json: 'json',
  go: 'go',
  py: 'python',
  rb: 'ruby',
  rs: 'rust',
  java: 'java',
  kt: 'kotlin',
  c: 'c',
  h: 'c',
  cc: 'cpp',
  cpp: 'cpp',
  hpp: 'cpp',
  cs: 'csharp',
  php: 'php',
  swift: 'swift',
  sh: 'shell',
  bash: 'shell',
  zsh: 'shell',
  yml: 'yaml',
  yaml: 'yaml',
  toml: 'toml',
  ini: 'ini',
  md: 'markdown',
  markdown: 'markdown',
  html: 'html',
  htm: 'html',
  css: 'css',
  scss: 'scss',
  less: 'less',
  sql: 'sql',
  xml: 'xml',
  dockerfile: 'dockerfile',
};

export function languageForPath(path: string): string {
  const name = basename(path).toLowerCase();
  if (name === 'dockerfile') return 'dockerfile';
  const dot = name.lastIndexOf('.');
  if (dot <= 0) return 'plaintext';
  return EXT_LANGUAGE[name.slice(dot + 1)] ?? 'plaintext';
}

/**
 * Resolve a root-relative search-hit path to the changed file it names, if any.
 * The changed-files list carries ABSOLUTE paths; a search match carries a
 * root-relative one under the same `root`. The join match is preferred; a
 * suffix match is the tolerant fallback for when the exact root is unknown (the
 * mission record carries no cwd, so `root` may be the default). Returns the
 * matched {@link MissionChangedFile} (whose absolute `path` the diff view
 * consumes) or undefined — undefined means "show inline context only", never a
 * general file viewer.
 */
export function matchChangedFile(
  files: readonly MissionChangedFile[],
  relPath: string,
  root?: string,
): MissionChangedFile | undefined {
  const rel = relPath.replace(/^\.?\/+/, '');
  if (root) {
    const joined = `${root.replace(/\/+$/, '')}/${rel}`;
    const exact = files.find(f => f.path === joined);
    if (exact) return exact;
  }
  return files.find(f => f.path === rel || f.path.endsWith(`/${rel}`));
}

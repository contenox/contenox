/**
 * Pure logic for the composer's `@`-mention feature: detecting an in-progress
 * `@query` at the caret, filtering the workspace file list, inserting the chosen
 * file as a visible token, and serializing the final draft into ACP content
 * blocks (a text block plus one `resource_link` per mention — reference only, no
 * embedded/attached content: the agent reads the file through its own tools).
 * No React, no DOM — the component (MentionMenu.tsx) wires this to state and
 * keydown handlers; this module is what the tests exercise.
 */
import { imageContent, textContent, type ContentBlock } from '../../../lib/acp';
import type { PendingImageAttachment } from './imageAttachments';

/** A workspace file the user can mention: its root-relative path and display name. */
export interface WorkspaceFileRef {
  /** Path relative to the session workspace root (what `local_fs` resolves). */
  path: string;
  /** Display name (basename). */
  name: string;
}

/**
 * One entry the `@`-menu can show: a file (insertable as a mention) or a
 * directory (navigable — picking it drills the menu into that folder rather
 * than inserting anything). Directories are how the menu browses the tree.
 */
export interface MentionCandidate extends WorkspaceFileRef {
  isDirectory: boolean;
}

export interface MentionQuery {
  /** Whether an `@query` is active at the caret. */
  active: boolean;
  /** The text typed after `@` up to the caret. */
  query: string;
  /** Index of the `@` in the draft. */
  start: number;
  /** Caret index (end of the query span). */
  end: number;
}

// An active mention at the caret is either a QUOTED form `@"…` (open quote —
// still typing; may contain spaces and any path char) or a BARE form `@…`
// (unquoted path chars: letters, digits, `. / _ -`, whitespace ends it). A
// CLOSED quoted token `@"…"` is complete and matches neither, so typing past
// it does not re-open the menu. The `@` must start the draft or follow
// whitespace so email-like `a@b` never triggers.
const MENTION_TOKEN_RE = /(^|\s)@(?:"([^"]*)|([\w./-]*))$/;

/**
 * Derives the active `@query` from the draft text up to the caret. Handles both
 * the quoted form (paths with spaces: `@"my dir/`) and the bare form. Returns
 * the query WITHOUT quotes, plus the span from `@` to the caret so callers can
 * replace it wholesale.
 */
export function deriveMentionQuery(text: string, caret: number): MentionQuery {
  const upto = text.slice(0, Math.max(0, Math.min(caret, text.length)));
  const m = MENTION_TOKEN_RE.exec(upto);
  if (!m) return { active: false, query: '', start: caret, end: caret };
  const quoted = m[2] !== undefined;
  const query = (quoted ? m[2] : m[3]) ?? '';
  // Token span: '@' + ('"' if quoted) + query, ending at the caret.
  const start = upto.length - query.length - (quoted ? 2 : 1);
  return { active: true, query, start, end: upto.length };
}

/** True when a path needs quoting in a mention token (contains whitespace). */
function needsQuoting(path: string): boolean {
  return /\s/.test(path);
}

/**
 * The token inserted into the draft for a mention — `@path`, or `@"path"` when
 * the path contains whitespace so the reference stays one lexeme. Serialization
 * (`activeMentions`) matches this exact form.
 */
export function mentionToken(file: WorkspaceFileRef): string {
  return needsQuoting(file.path) ? `@"${file.path}"` : `@${file.path}`;
}

/**
 * Splits an active `@query` into the directory being browsed and the leaf
 * filter within it. Everything up to the last `/` is the directory scope
 * (root is `''`); the rest filters that directory's entries. So `src/comp`
 * browses `src` filtering by `comp`, `src/` browses `src` with no filter, and
 * `READ` browses the root filtering by `READ`.
 */
export function splitMentionQuery(query: string): { dir: string; leaf: string } {
  const slash = query.lastIndexOf('/');
  if (slash === -1) return { dir: '', leaf: query };
  return { dir: query.slice(0, slash), leaf: query.slice(slash + 1) };
}

/**
 * The candidates shown for a browse scope: the directory's entries whose name
 * starts-with, then contains, the leaf filter — directories first (they are
 * navigation), files second, each group alphabetical. An empty leaf returns
 * the whole directory.
 */
export function mentionCandidatesForScope(entries: MentionCandidate[], leaf: string): MentionCandidate[] {
  const q = leaf.trim().toLowerCase();
  const scored = entries
    .map(e => {
      const name = e.name.toLowerCase();
      if (q === '') return { e, rank: 0 };
      if (name.startsWith(q)) return { e, rank: 0 };
      if (name.includes(q)) return { e, rank: 1 };
      return null;
    })
    .filter((x): x is { e: MentionCandidate; rank: number } => x !== null);
  scored.sort(
    (a, b) =>
      Number(b.e.isDirectory) - Number(a.e.isDirectory) || // directories first
      a.rank - b.rank ||
      a.e.name.localeCompare(b.e.name),
  );
  return scored.map(x => x.e);
}

/** The scope (directory path) an `@query` is browsing — its portion up to the last `/`. */
function queryScope(query: string): string {
  return splitMentionQuery(query).dir;
}

/** The parent of a directory path (`src/components` → `src`, `src` → root `''`). */
function parentDir(dir: string): string {
  const slash = dir.lastIndexOf('/');
  return slash === -1 ? '' : dir.slice(0, slash);
}

/** Builds a browse token (`@dir/`, quoted when needed, or bare `@` at root). */
function browseToken(dir: string): string {
  if (dir === '') return '@';
  const inner = `${dir}/`;
  return needsQuoting(inner) ? `@"${inner}` : `@${inner}`;
}

/**
 * Drills the draft into a directory: replaces the active `@query` span with
 * `@<dir>/` — quoted (`@"<dir>/`, open quote, no closing) when the directory
 * path contains whitespace, so browsing continues inside a spaced path. No
 * trailing space either way, so the menu stays open scoped to that directory.
 */
export function applyMentionFolder(text: string, q: MentionQuery, dir: WorkspaceFileRef): { text: string; caret: number } {
  const token = browseToken(dir.path);
  const next = text.slice(0, q.start) + token + text.slice(q.end);
  return { text: next, caret: q.start + token.length };
}

/**
 * Ascends the browse one directory: rewrites the active `@query` to browse the
 * PARENT of the directory currently in scope (dropping any leaf filter). At the
 * root this is a no-op (`@` stays `@`). The counterpart to `applyMentionFolder`
 * for keyboard tree navigation (← / Backspace).
 */
export function applyMentionAscend(text: string, q: MentionQuery): { text: string; caret: number } {
  const token = browseToken(parentDir(queryScope(q.query)));
  const next = text.slice(0, q.start) + token + text.slice(q.end);
  return { text: next, caret: q.start + token.length };
}

/**
 * Keyboard actions for the `@`-mention FILE BROWSER. Distinct from the slash
 * menu's mapping (`slashMenuKeyFromEvent`) because a tree needs directional
 * traversal the flat command list does not: → drills into the highlighted
 * folder, ← ascends to the parent. ↑/↓ move, Enter/Tab accept (folder drills,
 * file inserts), Escape closes. Returning null lets the composer handle the key
 * normally (e.g. ← when there is nothing to ascend into is decided by the hook).
 */
export type MentionMenuKeyAction =
  | { type: 'move'; delta: 1 | -1 }
  | { type: 'drill' }
  | { type: 'ascend' }
  | { type: 'accept' }
  | { type: 'close' };

export function mentionMenuKeyFromEvent(key: string): MentionMenuKeyAction | null {
  switch (key) {
    case 'ArrowDown':
      return { type: 'move', delta: 1 };
    case 'ArrowUp':
      return { type: 'move', delta: -1 };
    case 'ArrowRight':
      return { type: 'drill' };
    case 'ArrowLeft':
      return { type: 'ascend' };
    case 'Tab':
    case 'Enter':
      return { type: 'accept' };
    case 'Escape':
      return { type: 'close' };
    default:
      return null;
  }
}

/**
 * The path that should be live-previewed while browsing the `@`-menu: the
 * highlighted candidate's path when it is a FILE, otherwise `null` (directories
 * are navigation, not content, so they get no preview). Pure selector so the
 * "should preview + which path" decision is testable without React.
 */
export function mentionPreviewPath(entries: MentionCandidate[], activeIndex: number): string | null {
  const entry = entries[activeIndex];
  if (!entry || entry.isDirectory) return null;
  return entry.path;
}

/**
 * Case-insensitive filter of the workspace file list by the query, matched
 * against both the display name and the full path. An empty query returns the
 * list unchanged. Name-prefix matches are ranked ahead of looser matches.
 */
export function filterMentionFiles(files: WorkspaceFileRef[], query: string): WorkspaceFileRef[] {
  const q = query.trim().toLowerCase();
  if (q === '') return files;
  const scored = files
    .map(f => {
      const name = f.name.toLowerCase();
      const path = f.path.toLowerCase();
      if (name.startsWith(q)) return { f, rank: 0 };
      if (path.startsWith(q)) return { f, rank: 1 };
      if (name.includes(q) || path.includes(q)) return { f, rank: 2 };
      return null;
    })
    .filter((x): x is { f: WorkspaceFileRef; rank: number } => x !== null);
  scored.sort((a, b) => a.rank - b.rank || a.f.path.localeCompare(b.f.path));
  return scored.map(x => x.f);
}

/**
 * Replaces the active `@query` span with the chosen file's token followed by a
 * space, returning the new draft and the caret position after the inserted
 * token.
 */
export function applyMention(text: string, q: MentionQuery, file: WorkspaceFileRef): { text: string; caret: number } {
  const token = `${mentionToken(file)} `;
  const next = text.slice(0, q.start) + token + text.slice(q.end);
  return { text: next, caret: q.start + token.length };
}

/**
 * Returns the subset of `selected` mentions whose token still appears in the
 * final draft (the user may have deleted a token after selecting it),
 * de-duplicated by path. This is what actually gets serialized on submit.
 */
export function activeMentions(text: string, selected: WorkspaceFileRef[]): WorkspaceFileRef[] {
  const seen = new Set<string>();
  const out: WorkspaceFileRef[] = [];
  for (const f of selected) {
    if (seen.has(f.path)) continue;
    if (!text.includes(mentionToken(f))) continue;
    seen.add(f.path);
    out.push(f);
  }
  return out;
}

/**
 * Serializes a draft into ACP prompt content blocks: the text block (the draft
 * as typed, tokens included), one `resource_link` block per mention, then one
 * `image` block per pending attachment (base64 `data` + `mimeType`, exactly
 * the libacp `ContentBlock` wire form — no `data:` URI prefix).
 * `resource_link` is reference-only — the agent must read the file through its
 * tools; images are the ONE embedded/attached content kind, since a pasted
 * screenshot has no workspace path the agent could read instead.
 */
export function promptBlocksFromDraft(
  text: string,
  mentions: WorkspaceFileRef[],
  images: PendingImageAttachment[] = [],
): ContentBlock[] {
  const blocks: ContentBlock[] = [];
  if (text.trim() !== '') blocks.push(textContent(text));
  const seen = new Set<string>();
  for (const m of mentions) {
    if (seen.has(m.path)) continue;
    seen.add(m.path);
    blocks.push({ type: 'resource_link', name: m.path, uri: m.path });
  }
  for (const img of images) {
    blocks.push(imageContent(img.data, img.mimeType));
  }
  return blocks;
}

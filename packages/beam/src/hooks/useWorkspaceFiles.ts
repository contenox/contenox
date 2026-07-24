import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { apiFetch } from '../lib/fetch';
import {
  flattenFiles,
  ROOT_DIR,
  type DirCache,
  type WorkspaceEntry,
} from '../pages/chat/lib/workspaceTree';
import type { WorkspaceFileRef } from '../pages/chat/lib/mentions';

/**
 * The Default-policy sentinel, re-exported from its canonical home in
 * `lib/workspaceAccess` for this hook's existing importers.
 */
export { HITL_POLICY_DEFAULT_VALUE } from '../lib/workspaceAccess';

/** One entry from the `/files` list endpoint (localfileservice.Entry). Now a RAW listing. */
interface FilesListEntry {
  path: string;
  name: string;
  isDirectory: boolean;
  size: number;
}

/** The `/files/content` response (localfileapi.fileContentResponse). */
interface FileContentResponse {
  path: string;
  content: string;
  contentBase64?: string;
  encoding: string;
}

/** Peeked file content, normalized for presentation. */
export interface WorkspaceFilePeek {
  path: string;
  /** Decoded text, or '' for a binary file. */
  text: string;
  isBinary: boolean;
}

export interface UseWorkspaceFilesResult {
  /** The per-directory listing cache (root keyed by `''`). */
  cache: DirCache;
  /** Flat, de-duplicated file list across every loaded directory — the `@`-mention candidates. */
  files: WorkspaceFileRef[];
  /** True while the root listing is loading. */
  rootLoading: boolean;
  /** Whether a given directory path is currently loading. */
  isLoading: (dir: string) => boolean;
  /** Listing error, if the last fetch failed. */
  error: string | null;
  /** Lazily loads a directory's children (idempotent; no-op once loaded or in flight). */
  ensureLoaded: (dir: string) => void;
  /** Clears the cache and reloads the root listing. */
  refresh: () => void;
  /** Reads a file's content for peek. */
  readFile: (path: string) => Promise<WorkspaceFilePeek>;
  /** Whether the agent-view policy overlay is active (drives the /workspace/access batch). */
  agentView: boolean;
  /** Toggles the agent-view overlay. Does NOT re-list files — the overlay is a separate call. */
  setAgentView: (on: boolean) => void;
  /** The session's HITL policy the agent-view overlay evaluates against (for the access batch). */
  hitlPolicyName?: string | null;
}

/**
 * Root-relative `/files` browse URL. A RAW listing: agent-view verdicts no longer
 * ride on this endpoint (they come from a separate `POST /workspace/access` batch),
 * so the listing is byte-identical regardless of the overlay — toggling agent view
 * never re-lists the tree.
 */
export function filesUrl(path: string, root: string): string {
  const params = new URLSearchParams({ path: path === ROOT_DIR ? '.' : path, root });
  return `/api/files?${params.toString()}`;
}

function contentUrl(path: string, root: string): string {
  const params = new URLSearchParams({ path, root });
  return `/api/files/content?${params.toString()}`;
}

/**
 * Data hook for the session workspace's file tree: fetches per-directory
 * listings from the `/files` browse API (rooted at `root`, validated
 * server-side against the workspace allowlist), caches them by path, and
 * lazily loads a directory's children on demand. Resets only when `root`
 * changes — the agent-view overlay is a separate `/workspace/access` batch
 * (see `useWorkspaceAccess`), so toggling it or switching HITL policy re-colors
 * the tree WITHOUT re-listing it. Kept free of tree/serialization logic — that
 * lives in the pure `workspaceTree.ts` helpers this composes.
 *
 * `hitlPolicyName` is threaded through unchanged (exposed on the result) so the
 * panel can pin the access-batch evaluation to the same policy the live agent
 * gates under.
 */
export function useWorkspaceFiles(
  root: string | null,
  hitlPolicyName?: string | null,
): UseWorkspaceFilesResult {
  const [cache, setCache] = useState<DirCache>({});
  const [loading, setLoading] = useState<Record<string, boolean>>({});
  const [error, setError] = useState<string | null>(null);
  // Agent-view policy overlay. OFF by default so the raw filesystem tree renders
  // until the user opts in; flipping it only toggles the separate verdict batch.
  const [agentView, setAgentView] = useState(false);
  // Guards against setState after the root changed (or unmount) mid-fetch.
  const rootRef = useRef(root);
  rootRef.current = root;
  // Directories already requested for the current root, so ensureLoaded stays
  // idempotent without reading render state.
  const requestedRef = useRef<Set<string>>(new Set());

  const load = useCallback(
    async (dir: string) => {
      if (!root) return;
      requestedRef.current.add(dir);
      setLoading(prev => ({ ...prev, [dir]: true }));
      try {
        const entries = await apiFetch<FilesListEntry[]>(filesUrl(dir, root));
        if (rootRef.current !== root) return;
        const mapped: WorkspaceEntry[] = entries.map(e => ({
          path: e.path,
          name: e.name,
          isDirectory: e.isDirectory,
        }));
        setCache(prev => ({ ...prev, [dir]: mapped }));
        setError(null);
      } catch (err) {
        if (rootRef.current !== root) return;
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (rootRef.current === root) setLoading(prev => ({ ...prev, [dir]: false }));
      }
    },
    [root],
  );

  // (Re)load the root whenever the workspace root changes. The listing no longer
  // depends on the overlay/policy (those are a separate verdict call), so the
  // cache and expansion survive an agent-view toggle.
  useEffect(() => {
    requestedRef.current = new Set();
    setCache({});
    setLoading({});
    setError(null);
    if (root) void load(ROOT_DIR);
  }, [root, load]);

  const ensureLoaded = useCallback(
    (dir: string) => {
      if (requestedRef.current.has(dir)) return;
      void load(dir);
    },
    [load],
  );

  const refresh = useCallback(() => {
    requestedRef.current = new Set();
    setCache({});
    setLoading({});
    setError(null);
    if (root) void load(ROOT_DIR);
  }, [root, load]);

  const readFile = useCallback(
    async (path: string): Promise<WorkspaceFilePeek> => {
      if (!root) return { path, text: '', isBinary: false };
      const resp = await apiFetch<FileContentResponse>(contentUrl(path, root));
      if (resp.encoding === 'base64') {
        return { path: resp.path, text: '', isBinary: true };
      }
      return { path: resp.path, text: resp.content ?? '', isBinary: false };
    },
    [root],
  );

  const files = useMemo(() => flattenFiles(cache), [cache]);
  const isLoading = useCallback((dir: string) => !!loading[dir], [loading]);

  return {
    cache,
    files,
    rootLoading: !!loading[ROOT_DIR],
    isLoading,
    error,
    ensureLoaded,
    refresh,
    readFile,
    agentView,
    setAgentView,
    hitlPolicyName,
  };
}

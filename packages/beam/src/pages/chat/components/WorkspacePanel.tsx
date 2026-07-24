/**
 * i18n keys referenced in this file (namespace `workspace`; add to i18n.ts):
 *   workspace.panel_title              = "Workspace"
 *   workspace.refresh                  = "Refresh"
 *   workspace.loading                  = "Loading…"
 *   workspace.empty                    = "This workspace is empty"
 *   workspace.agent_view               = "Agent view"
 *   workspace.legend_allowed           = "allowed"
 *   workspace.legend_approval          = "needs approval"
 *   workspace.legend_blocked           = "blocked"
 *   workspace.legend_unreachable       = "unreachable"
 *   workspace.access_evaluating        = "Evaluating access…"
 *   workspace.access_error             = "Could not evaluate access"
 *   workspace.access_unreachable       = "Outside the workspace boundary"
 *   workspace.access_read              = "Read"
 *   workspace.access_write             = "Write"
 *   workspace.access_reason_rule       = "policy rule"
 *   workspace.access_reason_default    = "default policy"
 *   workspace.access_tooltip           = "{{dim}}: {{action}} — {{reason}}"
 *   workspace.filter_toggle            = "Filter files"
 *   workspace.filter_label             = "Filter files"
 *   workspace.filter_type              = "Filter type"
 *   workspace.filter_type_ext          = "Extension"
 *   workspace.filter_type_glob         = "Glob"
 *   workspace.filter_type_name         = "Name / path"
 *   workspace.filter_type_access_read  = "Read access"
 *   workspace.filter_type_access_write = "Write access"
 *   workspace.filter_placeholder_ext   = "md, ts, go"
 *   workspace.filter_placeholder_glob  = "*.md"
 *   workspace.filter_placeholder_name  = "name or path…"
 *   workspace.filter_searching         = "Searching…"
 *   workspace.filter_no_matches        = "No files match"
 *   workspace.filter_truncated         = "Showing first {{count}} — narrow your filter"
 */
import { Button, FileTree, SearchBar, Select, type FileTreeIndicator, type FileTreeNode } from '@contenox/ui';
import { useQueryClient } from '@tanstack/react-query';
import { Eye, Filter, Pencil, RefreshCw, ShieldCheck } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RootChip } from '../../../components/workspace/RootChip';
import { WorkspaceBoundaryNotice } from '../../../components/workspace/WorkspaceBoundaryNotice';
import { usePersistentToggle } from '../../../hooks/usePersistentToggle';
import { useWorkspaceAccess } from '../../../hooks/useWorkspaceAccess';
import { useWorkspaceRoots } from '../../../hooks/useWorkspaceRoots';
import { type UseWorkspaceFilesResult } from '../../../hooks/useWorkspaceFiles';
import { useWorkspaceFind } from '../../../hooks/useWorkspaceFind';
import { workspaceAccessKeys } from '../../../lib/queryKeys';
import type { WorkspaceRoot } from '../../../lib/types';
import { availableFilterTypes, buildTreeFromMatches, type WorkspaceFilterType } from '../lib/workspaceFilter';
import {
  toFileTreeNodes,
  verdictToAccess,
  type AccessLabels,
  type NodeAccess,
  type NodeDecoration,
  type NodeDecorator,
} from '../lib/workspaceTree';

export interface WorkspacePanelProps {
  /** The session workspace root; `null` when there is nothing to show yet. */
  root: string | null;
  /** Shared `useWorkspaceFiles(root)` result — owned by the page, also fed to the mention menu. */
  files: UseWorkspaceFilesResult;
  /** Opens a file as a read-only canvas tab (files no longer preview inline in this sidebar). */
  onOpenFile: (path: string) => void;
  /** The path of the file whose canvas tab is currently active, for row highlight. */
  selectedFilePath?: string | null;
}

/**
 * Maps the icon-free two-axis {@link NodeAccess} onto `FileTree` indicators: an
 * eye for read, a pencil for write — each tinted by its own verdict severity. An
 * unreachable path also dims the row and carries the boundary tooltip.
 */
/** i18n keys for the access-axis filter option values (allow/approve/deny/unreachable). */
const ACCESS_OPTION_LABEL_KEY: Record<string, string> = {
  allow: 'workspace.legend_allowed',
  approve: 'workspace.legend_approval',
  deny: 'workspace.legend_blocked',
  unreachable: 'workspace.legend_unreachable',
};

function accessDecoration(a: NodeAccess): NodeDecoration {
  const indicators: FileTreeIndicator[] = [];
  if (a.read) {
    indicators.push({ key: 'read', icon: <Eye className="h-3.5 w-3.5" />, status: a.read.status, title: a.read.title });
  }
  if (a.write) {
    indicators.push({ key: 'write', icon: <Pencil className="h-3.5 w-3.5" />, status: a.write.status, title: a.write.title });
  }
  return {
    ...(indicators.length ? { indicators } : {}),
    ...(a.dimmed ? { dimmed: true, title: a.read?.title ?? a.write?.title } : {}),
  };
}

/**
 * IDE-style file explorer for the session workspace: a lazily-loaded directory
 * tree backed by `useWorkspaceFiles`. Clicking a file opens it as a read-only
 * canvas tab (no inline preview lives here anymore). An optional "agent view"
 * overlays the active HITL policy's per-entry verdict as TWO independent trailing
 * indicators — an eye (read) and a pencil (write), each tinted allow/approve/deny,
 * with unreachable rows dimmed. The listing (`/files`, `/workspace/find`) is raw;
 * the verdicts come from a separate `POST /workspace/access` batch
 * (`useWorkspaceAccess`) and are merged in by path. Pure presentation — all
 * fetching/caching lives in the hooks and the pure `workspaceTree` helpers.
 *
 * The panel's visibility is governed SOLELY by the chat toolbar's "Files"
 * toggle (a shared persistent toggle); it carries no collapse affordance of its
 * own, so there is exactly one open/close mechanism.
 */
export function WorkspacePanel({ root, files, onOpenFile, selectedFilePath }: WorkspacePanelProps) {
  const { t } = useTranslation();
  // The filter registry carries i18n keys as plain strings (it must not depend
  // on the generated key union); resolve those dynamic keys through a loosened t.
  const tk = t as (key: string) => string;
  const queryClient = useQueryClient();
  const { roots } = useWorkspaceRoots();
  const { agentView, setAgentView, cache, ensureLoaded } = files;

  const handleNodeSelect = useCallback(
    (node: FileTreeNode) => {
      const path = node.path ?? node.id;
      if (node.isDirectory) {
        ensureLoaded(path);
        return;
      }
      onOpenFile(path);
    },
    [ensureLoaded, onOpenFile],
  );

  // Localized phrases for the two-axis tooltips, assembled from the STRUCTURED
  // verdict codes (not English server strings).
  const accessLabels = useMemo<AccessLabels>(
    () => ({
      read: t('workspace.access_read'),
      write: t('workspace.access_write'),
      unreachable: t('workspace.access_unreachable'),
      actionAllow: t('workspace.legend_allowed'),
      actionApprove: t('workspace.legend_approval'),
      actionDeny: t('workspace.legend_blocked'),
      reasonRule: t('workspace.access_reason_rule'),
      reasonDefault: t('workspace.access_reason_default'),
      format: (dim, action, reason) => t('workspace.access_tooltip', { dim, action, reason }),
    }),
    [t],
  );

  // Every loaded path (files AND directories) — the union the verdict batch
  // evaluates under the lazy tree; grows as directories expand.
  const cachePaths = useMemo(() => {
    const out: string[] = [];
    for (const entries of Object.values(cache)) {
      if (!entries) continue;
      for (const e of entries) out.push(e.path);
    }
    return out;
  }, [cache]);

  // --- Filter facility (extensible; types live in `workspaceFilter.ts`). ---
  // The whole filter section is collapsible behind a header toggle; when hidden
  // it is INACTIVE (no streamed find runs, the ordinary lazy tree shows). The
  // open/closed choice is a workspace-wide, persisted preference. Default =
  // collapsed, so the panel is clean until the user reaches for filtering.
  const filterPanel = usePersistentToggle('workspace.filterOpen');
  const filterTypes = useMemo(() => availableFilterTypes({ agentView }), [agentView]);
  const [filterTypeId, setFilterTypeId] = useState(filterTypes[0]?.id ?? 'ext');
  const [filterValue, setFilterValue] = useState('');
  // Debounce the *applied* value so typing stays instant while the pruned tree
  // (and the per-query FileTree remount) only churns after a short pause.
  const [appliedValue, setAppliedValue] = useState('');
  useEffect(() => {
    const id = setTimeout(() => setAppliedValue(filterValue), 200);
    return () => clearTimeout(id);
  }, [filterValue]);

  // The selected type, kept valid as the available set changes (agent view
  // toggles the access axes in/out). References are the stable module-level objects.
  const activeType: WorkspaceFilterType | undefined =
    filterTypes.find(ft => ft.id === filterTypeId) ?? filterTypes[0];
  const inputSpec = activeType?.input;
  const filterPlaceholder = inputSpec?.kind === 'text' ? tk(inputSpec.placeholderKey) : '';
  const optionValues = inputSpec?.kind === 'options' ? inputSpec.options : [];
  const query = useMemo(() => activeType?.toQuery(appliedValue) ?? null, [activeType, appliedValue]);
  // A collapsed filter is inactive regardless of any pending value, so hiding it
  // clears the filter and the tree shows normally.
  const filterActive = filterPanel.open && query !== null;

  // A filter runs a SERVER-SIDE recursive find: one streamed request returns
  // every matching file across the tree (the per-directory client walk is gone).
  // An empty `globs` (the access axes) means "walk everything" → `*`. The find is
  // a RAW listing now — verdicts come from the separate /workspace/access batch.
  const findGlobs = useMemo(
    () => (filterActive && query ? (query.globs.length > 0 ? query.globs : ['*']) : []),
    [filterActive, query],
  );
  const find = useWorkspaceFind({
    globs: findGlobs,
    root: root ?? undefined,
  });

  // The verdict source. Under agent view we evaluate the loaded-path union (lazy
  // tree) or the current match set (filter active); the resulting `path → verdict`
  // map is merged into the raw listing by path. Keyed/cached in react-query so
  // expands don't thrash (see useWorkspaceAccess).
  const accessPaths = useMemo(
    () => (filterActive ? find.entries.map(e => e.path) : cachePaths),
    [filterActive, find.entries, cachePaths],
  );
  const access = useWorkspaceAccess({
    root,
    policy: files.hitlPolicyName,
    paths: accessPaths,
    enabled: agentView,
  });

  // Per-path decorator: verdict → two-axis view → eye/pencil indicators. Only
  // wired under agent view (baseNodes/filteredNodes pass `undefined` otherwise).
  const decorate = useCallback<NodeDecorator>(
    path => {
      const verdict = access.verdicts.get(path);
      if (!verdict) return undefined;
      return accessDecoration(verdictToAccess(verdict, accessLabels));
    },
    [access.verdicts, accessLabels],
  );

  const baseNodes = useMemo(
    () => toFileTreeNodes(cache, undefined, agentView ? decorate : undefined),
    [cache, agentView, decorate],
  );

  // The type's optional client-side refinement (e.g. an access axis, which reads
  // the per-path verdict), then the flat matches assembled into a FileTree with
  // the same agent-view overlay.
  const filteredNodes = useMemo(() => {
    if (!query) return [];
    const refined = query.refine
      ? find.entries.filter(m => query.refine!(m, access.verdicts.get(m.path)))
      : find.entries;
    return buildTreeFromMatches(refined, agentView ? decorate : undefined);
  }, [query, find.entries, agentView, decorate, access.verdicts]);

  const nodes = filterActive ? filteredNodes : baseNodes;

  const handleRefresh = useCallback(() => {
    files.refresh();
    void queryClient.invalidateQueries({ queryKey: workspaceAccessKeys.all });
  }, [files, queryClient]);

  if (!root) return null;

  const isEmptyRoot = !files.rootLoading && !files.error && baseNodes.length === 0;
  // Prefer the allowlisted root (so the chip can flag the default); fall back to
  // a plain chip for the session's own root when the allowlist is absent.
  const activeRoot: WorkspaceRoot = roots.find(r => r.path === root) ?? { path: root, default: false };

  return (
    <div className="border-surface-200 bg-surface-50 dark:border-dark-surface-600 dark:bg-dark-surface-100 flex h-full w-64 min-w-0 shrink-0 flex-col border-r sm:w-72">
      <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 items-center justify-between gap-2 border-b px-3 py-2">
        <span className="text-text dark:text-dark-text truncate text-sm font-medium">{t('workspace.panel_title')}</span>
        <div className="flex shrink-0 items-center gap-1">
          <Button
            type="button"
            variant={agentView ? 'primary' : 'ghost'}
            palette="neutral"
            size="icon"
            aria-pressed={agentView}
            aria-label={t('workspace.agent_view')}
            title={t('workspace.agent_view')}
            onClick={() => setAgentView(!agentView)}>
            <ShieldCheck className="h-3.5 w-3.5" />
          </Button>
          <Button
            type="button"
            variant={filterPanel.open ? 'primary' : 'ghost'}
            palette="neutral"
            size="icon"
            aria-pressed={filterPanel.open}
            aria-label={t('workspace.filter_toggle')}
            title={t('workspace.filter_toggle')}
            onClick={() => filterPanel.toggle()}>
            <Filter className="h-3.5 w-3.5" />
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('workspace.refresh')}
            onClick={handleRefresh}
          >
            <RefreshCw className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>

      <div className="border-surface-200 dark:border-dark-surface-600 shrink-0 border-b px-3 py-1.5">
        <RootChip root={activeRoot} />
      </div>

      {filterPanel.open && (
        <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-col gap-1.5 border-b px-3 py-2">
          <Select
            aria-label={t('workspace.filter_type')}
            value={activeType?.id ?? ''}
            onChange={e => {
              setFilterTypeId(e.target.value);
              setFilterValue('');
            }}
            options={filterTypes.map(ft => ({ value: ft.id, label: tk(ft.labelKey) }))}
            className="w-full"
          />
          {inputSpec?.kind === 'options' ? (
            <Select
              aria-label={t('workspace.filter_label')}
              value={filterValue}
              onChange={e => setFilterValue(e.target.value)}
              placeholder={t('workspace.filter_label')}
              options={optionValues.map(o => ({ value: o, label: ACCESS_OPTION_LABEL_KEY[o] ? tk(ACCESS_OPTION_LABEL_KEY[o]) : o }))}
              className="w-full"
            />
          ) : (
            <SearchBar
              aria-label={t('workspace.filter_label')}
              value={filterValue}
              onChange={e => setFilterValue(e.target.value)}
              onClear={() => setFilterValue('')}
              placeholder={filterPlaceholder}
            />
          )}
          {filterActive && find.status === 'searching' && (
            <span className="text-text-muted dark:text-dark-text-muted px-0.5 text-[11px]">
              {t('workspace.filter_searching')}
            </span>
          )}
          {filterActive && find.truncated && (
            <span className="text-text-muted dark:text-dark-text-muted px-0.5 text-[11px]">
              {t('workspace.filter_truncated', { count: find.count })}
            </span>
          )}
        </div>
      )}

      {agentView && (
        <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-col gap-1 border-b px-3 py-1.5 text-[11px] text-text-muted dark:text-dark-text-muted">
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <span className="inline-flex items-center gap-1">
              <Eye className="h-3 w-3 shrink-0" aria-hidden="true" />
              {t('workspace.access_read')}
            </span>
            <span className="inline-flex items-center gap-1">
              <Pencil className="h-3 w-3 shrink-0" aria-hidden="true" />
              {t('workspace.access_write')}
            </span>
            {access.error ? (
              <span className="text-error-600 dark:text-dark-error-500">{t('workspace.access_error')}</span>
            ) : access.isLoading ? (
              <span>{t('workspace.access_evaluating')}</span>
            ) : null}
          </div>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <LegendItem dotClass="ring-1 ring-inset ring-success-500/60" label={t('workspace.legend_allowed')} />
            <LegendItem dotClass="bg-warning-500 dark:bg-dark-warning-500" label={t('workspace.legend_approval')} />
            <LegendItem dotClass="bg-error-500 dark:bg-dark-error-500" label={t('workspace.legend_blocked')} />
            <LegendItem dotClass="bg-text-muted dark:bg-dark-text-muted opacity-50" label={t('workspace.legend_unreachable')} />
          </div>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {files.error ? (
          <WorkspaceBoundaryNotice
            message={files.error}
            roots={roots}
            onRetry={handleRefresh}
          />
        ) : null}

        {files.rootLoading && baseNodes.length === 0 ? (
          <span className="text-text-muted dark:text-dark-text-muted block px-1 py-2 text-xs">{t('workspace.loading')}</span>
        ) : isEmptyRoot ? (
          <span className="text-text-muted dark:text-dark-text-muted block px-1 py-2 text-xs">{t('workspace.empty')}</span>
        ) : filterActive ? (
          find.status === 'refusal' || find.status === 'error' ? (
            <span className="text-error-600 dark:text-dark-error-500 block px-1 py-2 text-xs">
              {find.refusalMessage ?? find.errorMessage}
            </span>
          ) : nodes.length === 0 ? (
            <span className="text-text-muted dark:text-dark-text-muted block px-1 py-2 text-xs">
              {find.status === 'searching' ? t('workspace.filter_searching') : t('workspace.filter_no_matches')}
            </span>
          ) : (
            <FileTree
              key={`f:${activeType?.id}:${appliedValue}`}
              nodes={nodes}
              directoryClickMode="expand"
              defaultExpanded={undefined}
              selectedId={selectedFilePath ?? undefined}
              onNodeSelect={handleNodeSelect}
            />
          )
        ) : (
          <FileTree
            key="all"
            nodes={baseNodes}
            directoryClickMode="expand"
            defaultExpanded={new Set<string>()}
            selectedId={selectedFilePath ?? undefined}
            onNodeSelect={handleNodeSelect}
          />
        )}
      </div>
    </div>
  );
}

function LegendItem({ dotClass, label }: { dotClass: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1">
      <span aria-hidden="true" className={`h-2 w-2 shrink-0 rounded-full ${dotClass}`} />
      {label}
    </span>
  );
}

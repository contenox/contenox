/**
 * i18n keys referenced in this file (namespace `workspace`; add to i18n.ts):
 *   workspace.panel_title          = "Workspace"
 *   workspace.refresh              = "Refresh"
 *   workspace.loading              = "Loading…"
 *   workspace.empty                = "This workspace is empty"
 *   workspace.agent_view           = "Agent view"
 *   workspace.legend_allowed       = "allowed"
 *   workspace.legend_approval      = "needs approval"
 *   workspace.legend_blocked       = "blocked"
 *   workspace.legend_unreachable   = "unreachable"
 *   workspace.access_unreachable   = "Outside the workspace boundary"
 *   workspace.access_read          = "Read"
 *   workspace.access_write         = "Write"
 *   workspace.filter_toggle        = "Filter files"
 *   workspace.filter_label         = "Filter files"
 *   workspace.filter_type          = "Filter type"
 *   workspace.filter_type_ext      = "Extension"
 *   workspace.filter_type_glob     = "Glob"
 *   workspace.filter_type_name     = "Name / path"
 *   workspace.filter_type_access   = "Access"
 *   workspace.filter_placeholder_ext  = "md, ts, go"
 *   workspace.filter_placeholder_glob = "*.md"
 *   workspace.filter_placeholder_name = "name or path…"
 *   workspace.filter_searching     = "Searching…"
 *   workspace.filter_no_matches    = "No files match"
 *   workspace.filter_truncated     = "Showing first {{count}} — narrow your filter"
 */
import { Button, FileTree, SearchBar, Select, type FileTreeNode } from '@contenox/ui';
import { Filter, RefreshCw, ShieldCheck } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { RootChip } from '../../../components/workspace/RootChip';
import { WorkspaceBoundaryNotice } from '../../../components/workspace/WorkspaceBoundaryNotice';
import { usePersistentToggle } from '../../../hooks/usePersistentToggle';
import { useWorkspaceRoots } from '../../../hooks/useWorkspaceRoots';
import { type UseWorkspaceFilesResult } from '../../../hooks/useWorkspaceFiles';
import { useWorkspaceFind } from '../../../hooks/useWorkspaceFind';
import type { WorkspaceRoot } from '../../../lib/types';
import { availableFilterTypes, buildTreeFromMatches, type WorkspaceFilterType } from '../lib/workspaceFilter';
import { toFileTreeNodes, type AccessLabels } from '../lib/workspaceTree';

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
 * IDE-style file explorer for the session workspace: a lazily-loaded directory
 * tree backed by `useWorkspaceFiles`. Clicking a file opens it as a read-only
 * canvas tab (no inline preview lives here anymore). An optional "agent view"
 * overlays the active HITL policy's per-entry verdict (a colored status dot +
 * tooltip; unreachable rows are dimmed). Pure presentation — all
 * fetching/caching lives in the shared hook and the pure `workspaceTree` helpers.
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

  const accessLabels = useMemo<AccessLabels>(
    () => ({
      unreachable: t('workspace.access_unreachable'),
      read: t('workspace.access_read'),
      write: t('workspace.access_write'),
    }),
    [t],
  );

  const baseNodes = useMemo(
    () => toFileTreeNodes(cache, undefined, agentView ? accessLabels : undefined),
    [cache, agentView, accessLabels],
  );

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
  // toggles `access` in/out). References are the stable module-level objects.
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
  // An empty `globs` (the `access` type) means "walk everything" → `*`. Under
  // agent view we request filter=agent so matches carry the same verdicts the
  // lazy tree shows, and the `access` type can refine on them.
  const findGlobs = useMemo(
    () => (filterActive && query ? (query.globs.length > 0 ? query.globs : ['*']) : []),
    [filterActive, query],
  );
  const find = useWorkspaceFind({
    globs: findGlobs,
    root: root ?? undefined,
    filter: agentView ? 'agent' : undefined,
  });

  // The type's optional client-side refinement (e.g. the access verdict), then
  // the flat matches assembled into a FileTree.
  const filteredNodes = useMemo(() => {
    if (!query) return [];
    const refined = query.refine ? find.entries.filter(query.refine) : find.entries;
    return buildTreeFromMatches(refined, agentView ? accessLabels : undefined);
  }, [query, find.entries, agentView, accessLabels]);

  const nodes = filterActive ? filteredNodes : baseNodes;

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
            onClick={() => files.refresh()}
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
              options={optionValues.map(o => ({ value: o, label: o }))}
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
        <div className="border-surface-200 dark:border-dark-surface-600 flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1 border-b px-3 py-1.5 text-[11px] text-text-muted dark:text-dark-text-muted">
          <LegendItem dotClass="ring-1 ring-inset ring-success-500/60" label={t('workspace.legend_allowed')} />
          <LegendItem dotClass="bg-warning-500 dark:bg-dark-warning-500" label={t('workspace.legend_approval')} />
          <LegendItem dotClass="bg-error-500 dark:bg-dark-error-500" label={t('workspace.legend_blocked')} />
          <LegendItem dotClass="bg-text-muted dark:bg-dark-text-muted opacity-50" label={t('workspace.legend_unreachable')} />
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto p-2">
        {files.error ? (
          <WorkspaceBoundaryNotice
            message={files.error}
            roots={roots}
            onRetry={() => files.refresh()}
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

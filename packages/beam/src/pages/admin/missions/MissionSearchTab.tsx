import { Badge, Button, InlineNotice, P, Spinner } from '@contenox/ui';
import { ChevronDown, ChevronRight, Search } from 'lucide-react';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { WorkspaceBoundaryNotice } from '../../../components/workspace/WorkspaceBoundaryNotice';
import { useWorkspaceRoots } from '../../../hooks/useWorkspaceRoots';
import { useWorkspaceSearch, type WorkspaceSearchState } from '../../../hooks/useWorkspaceSearch';
import { matchChangedFile } from '../../../lib/missionChanges';
import type { MissionChangedFile, WorkspaceRoot, WorkspaceSearchMatch } from '../../../lib/types';
import { byteSlice, type SearchFileGroup } from '../../../lib/workspaceSearch';
import { shortenRootPath } from '../../../lib/workspaceRoots';

export interface MissionSearchTabProps {
  /** The mission's changed files, for click-through routing (a hit in one opens the Changes tab). */
  changedFiles: MissionChangedFile[];
  /** Route a hit's ABSOLUTE path into the Changes tab, focused. */
  onOpenInChanges: (absolutePath: string) => void;
}

/**
 * The mission inspector's Search tab (Arc 2). A debounced, streamed
 * workspace-wide literal search: the input stays instant while matches arrive
 * incrementally, grouped per file with count badges and byte-offset-accurate
 * highlights. The two pre-stream refusals are first-class states — a 501 becomes
 * the ripgrep teaching notice, a 422 out-of-bounds root reuses the designed
 * WorkspaceBoundaryNotice — and `done.truncated` offers "refine your search".
 *
 * Click-through honours the meaningful-filter: a hit whose file is one of the
 * mission's changed files routes into the Changes tab focused on it; any other
 * hit shows its matched line inline only — never a general file viewer (that is
 * editor territory, which Beam is not).
 */
export default function MissionSearchTab({ changedFiles, onOpenInChanges }: MissionSearchTabProps) {
  const [query, setQuery] = useState('');
  const { roots, defaultRoot } = useWorkspaceRoots();
  const [retryNonce, setRetryNonce] = useState(0);
  const state = useWorkspaceSearch({ query, root: defaultRoot?.path, nonce: retryNonce });
  // Re-issue the current query (the effect keys on nonce too); a no-op for a
  // still-refusing root, honest for a transient error.
  const retrySearch = () => setRetryNonce(n => n + 1);

  return (
    <SearchTabView
      state={state}
      query={query}
      onQueryChange={setQuery}
      root={defaultRoot}
      roots={roots}
      changedFiles={changedFiles}
      onOpenInChanges={onOpenInChanges}
      onRetry={retrySearch}
    />
  );
}

export interface SearchTabViewProps {
  state: WorkspaceSearchState;
  query: string;
  onQueryChange: (q: string) => void;
  root?: WorkspaceRoot;
  roots: WorkspaceRoot[];
  changedFiles: MissionChangedFile[];
  onOpenInChanges: (absolutePath: string) => void;
  onRetry: () => void;
}

/** The Search tab as a pure function of its state — extracted so every terminal
 *  state (refusal / dependency / done / truncated / empty) is render-testable. */
export function SearchTabView({
  state,
  query,
  onQueryChange,
  root,
  roots,
  changedFiles,
  onOpenInChanges,
  onRetry,
}: SearchTabViewProps) {
  const { t } = useTranslation();

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Search
            aria-hidden="true"
            className="text-text-muted dark:text-dark-text-muted pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2"
          />
          <input
            type="search"
            value={query}
            onChange={e => onQueryChange(e.target.value)}
            placeholder={t('workspaceSearch.placeholder')}
            aria-label={t('workspaceSearch.placeholder')}
            className="bg-surface-50 text-text placeholder:text-text-muted dark:bg-dark-surface-50 dark:text-dark-text dark:placeholder:text-dark-text-muted border-surface-300 dark:border-dark-surface-500 h-9 w-full rounded-lg border py-1 pr-3 pl-8 text-sm"
          />
        </div>
        {state.status === 'searching' && <Spinner size="sm" />}
      </div>

      {root && (
        <span className="text-text-muted dark:text-dark-text-muted text-xs" title={root.path}>
          {t('workspaceSearch.root_label')}: <span className="font-mono">{shortenRootPath(root.path, 4)}</span>
        </span>
      )}

      <SearchBody
        state={state}
        query={query}
        roots={roots}
        rootPath={root?.path}
        changedFiles={changedFiles}
        onOpenInChanges={onOpenInChanges}
        onRetry={onRetry}
      />
    </div>
  );
}

function SearchBody({
  state,
  query,
  roots,
  rootPath,
  changedFiles,
  onOpenInChanges,
  onRetry,
}: {
  state: WorkspaceSearchState;
  query: string;
  roots: WorkspaceRoot[];
  rootPath?: string;
  changedFiles: MissionChangedFile[];
  onOpenInChanges: (absolutePath: string) => void;
  onRetry: () => void;
}) {
  const { t } = useTranslation();

  if (state.status === 'dependency') {
    return (
      <InlineNotice variant="warning">
        <p className="font-medium">{t('workspaceSearch.dependency_title')}</p>
        <p className="mt-1 text-xs">{t('workspaceSearch.dependency_body')}</p>
      </InlineNotice>
    );
  }

  if (state.status === 'refusal') {
    return (
      <WorkspaceBoundaryNotice message={state.refusalMessage ?? ''} roots={roots} onRetry={onRetry} />
    );
  }

  if (state.status === 'error') {
    return (
      <InlineNotice variant="error">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span className="min-w-0">
            {t('workspaceSearch.error')} {state.errorMessage}
          </span>
          <Button type="button" variant="outline" size="xs" onClick={onRetry}>
            {t('common.retry')}
          </Button>
        </div>
      </InlineNotice>
    );
  }

  if (state.status === 'idle' || query.trim() === '') {
    return <P variant="muted">{t('workspaceSearch.empty_query')}</P>;
  }

  if (state.status === 'done' && state.matchCount === 0) {
    return <P variant="muted">{t('workspaceSearch.no_matches')}</P>;
  }

  return (
    <div className="flex flex-col gap-3">
      {state.matchCount > 0 && (
        <span className="text-text-muted dark:text-dark-text-muted text-xs">
          {t('workspaceSearch.match_count', { n: state.matchCount })}
        </span>
      )}

      <ul className="flex flex-col gap-2">
        {state.groups.map(group => (
          <SearchFileGroupRow
            key={group.path}
            group={group}
            changed={matchChangedFile(changedFiles, group.path, rootPath)}
            onOpenInChanges={onOpenInChanges}
          />
        ))}
      </ul>

      {state.status === 'done' && state.truncated && (
        <InlineNotice variant="info">{t('workspaceSearch.truncated')}</InlineNotice>
      )}
    </div>
  );
}

function SearchFileGroupRow({
  group,
  changed,
  onOpenInChanges,
}: {
  group: SearchFileGroup;
  changed?: MissionChangedFile;
  onOpenInChanges: (absolutePath: string) => void;
}) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(true);
  const Chevron = open ? ChevronDown : ChevronRight;

  return (
    <li className="border-surface-200 dark:border-dark-surface-600 rounded-lg border">
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          type="button"
          onClick={() => setOpen(o => !o)}
          aria-expanded={open}
          className="flex min-w-0 flex-1 items-center gap-2 text-left">
          <Chevron aria-hidden="true" className="text-text-muted dark:text-dark-text-muted h-4 w-4 shrink-0" />
          <span className="min-w-0 truncate font-mono text-sm" title={group.path}>
            {group.path}
          </span>
          <Badge variant="secondary" size="sm" className="shrink-0 tabular-nums">
            {group.matches.length}
          </Badge>
        </button>
        {changed && (
          <Button
            type="button"
            variant="outline"
            size="xs"
            className="shrink-0"
            onClick={() => onOpenInChanges(changed.path)}>
            {t('workspaceSearch.open_in_changes')}
          </Button>
        )}
      </div>

      {open && (
        <ul className="border-surface-200 dark:border-dark-surface-600 border-t">
          {group.matches.map((m, i) => (
            <SearchMatchRow key={`${m.line}:${m.column}:${i}`} match={m} />
          ))}
        </ul>
      )}
    </li>
  );
}

/** One match line: the 1-based line number and the preview with the byte-offset
 *  substring highlighted. Inline context only — never a link into a file viewer. */
function SearchMatchRow({ match }: { match: WorkspaceSearchMatch }) {
  const { before, match: hit, after } = byteSlice(match.preview, match.column, match.length);
  return (
    <li className="flex gap-3 px-3 py-1.5 font-mono text-xs">
      <span className="text-text-muted dark:text-dark-text-muted shrink-0 tabular-nums select-none">
        {match.line}
      </span>
      <span className="min-w-0 break-all whitespace-pre-wrap">
        {before}
        <mark className="bg-warning-200 text-warning-900 dark:bg-dark-surface-500 dark:text-dark-text rounded-sm px-0.5">
          {hit}
        </mark>
        {after}
      </span>
    </li>
  );
}

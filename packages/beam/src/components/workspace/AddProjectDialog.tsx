import {
  Button,
  Dialog,
  FileTree,
  FormField,
  InlineNotice,
  Input,
  Select,
  Spinner,
  type FileTreeNode,
} from '@contenox/ui';
import { FolderSearch, Keyboard } from 'lucide-react';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useWorkspaceFiles } from '../../hooks/useWorkspaceFiles';
import { useAddWorkspaceRoot } from '../../hooks/useWorkspaceRootMutations';
import { ApiError } from '../../lib/fetch';
import type { WorkspaceRoot } from '../../lib/types';
import { activeWorkspaceRoot, projectName, shortenRootPath } from '../../lib/workspaceRoots';
import { folderBasename, joinRootRelative, toFolderNodes } from './folderTree';

export interface AddProjectDialogProps {
  open: boolean;
  /** Close without adding (Cancel / Esc / backdrop). */
  onClose: () => void;
  /** The allowlisted roots — the browse starting points. */
  roots: readonly WorkspaceRoot[];
  /** Called after a folder is successfully registered, before the dialog closes. */
  onAdded?: () => void;
}

/**
 * The "add a project" journey: browse the runtime's allowlisted roots as a
 * folder tree, click the project directory, name it (defaulted to the folder
 * name), and register it — replacing the old blind "paste an absolute path"
 * form. Browsing runs through the same `/files` endpoint the session file tree
 * uses, so the picker never reaches outside the workspace boundary; a manual
 * path field stays as the escape hatch for a folder that lives outside every
 * browsable root. The server still does the real bounds/too-broad check on
 * submit and returns a teaching 422 — this UI never pretends to validate the
 * path itself (matching RootSelector).
 *
 * `Dialog` unmounts its children when closed, so the body remounts fresh on each
 * open: no reset effects, and the browse fetch only runs while the dialog is up.
 */
export function AddProjectDialog({ open, onClose, roots, onAdded }: AddProjectDialogProps) {
  const { t } = useTranslation();
  return (
    <Dialog
      open={open}
      onClose={onClose}
      title={t('projects.dialog_title')}
      closeLabel={t('common.close')}
      className="w-[34rem] max-w-[92vw]">
      <AddProjectBody roots={roots} onClose={onClose} onAdded={onAdded} />
    </Dialog>
  );
}

function AddProjectBody({
  roots,
  onClose,
  onAdded,
}: {
  roots: readonly WorkspaceRoot[];
  onClose: () => void;
  onAdded?: () => void;
}) {
  const { t } = useTranslation();
  const hasRoots = roots.length > 0;

  const [mode, setMode] = useState<'browse' | 'manual'>(hasRoots ? 'browse' : 'manual');
  const [browseRoot, setBrowseRoot] = useState(() => activeWorkspaceRoot(roots)?.path ?? '');
  // The picked folder's path RELATIVE to `browseRoot` (the FileTree node id), or
  // null when nothing is picked yet.
  const [selectedRel, setSelectedRel] = useState<string | null>(null);
  const [manualPath, setManualPath] = useState('');
  const [name, setName] = useState('');
  // Once the operator edits the name we stop overwriting it with the folder
  // basename, so a deliberate rename survives further navigation.
  const [nameEdited, setNameEdited] = useState(false);

  const addMutation = useAddWorkspaceRoot();

  // Only browse (and only while in browse mode) — manual entry needs no listing.
  const files = useWorkspaceFiles(mode === 'browse' ? browseRoot : null);
  const nodes = useMemo(() => toFolderNodes(files.cache), [files.cache]);

  // The absolute path the grant will register: browse mode joins the picked
  // relative path onto its root; manual mode takes the field verbatim.
  const selectedAbsolute =
    mode === 'manual'
      ? manualPath.trim()
      : selectedRel
        ? joinRootRelative(browseRoot, selectedRel)
        : '';

  // Default the name to the folder's basename until the operator types their own.
  useEffect(() => {
    if (nameEdited) return;
    setName(selectedAbsolute ? folderBasename(selectedAbsolute) : '');
  }, [selectedAbsolute, nameEdited]);

  // A directory click both loads its children (lazy tree) AND picks it as the
  // target — one gesture drills in and selects, the same row highlighted.
  const handleNodeSelect = useCallback(
    (node: FileTreeNode) => {
      const path = node.path ?? node.id;
      files.ensureLoaded(path);
      setSelectedRel(path);
    },
    [files],
  );

  const handleRootChange = (next: string) => {
    setBrowseRoot(next);
    setSelectedRel(null); // the old relative pick is meaningless under a new root
  };

  const switchMode = (next: 'browse' | 'manual') => {
    setMode(next);
    setSelectedRel(null);
    setManualPath('');
    setNameEdited(false);
    addMutation.reset();
  };

  // Whether the picked folder is ALREADY a registered root (exact, trailing-
  // slash-insensitive match). The grant is idempotent server-side — re-adding a
  // known path never errors: it just re-stamps the marker name (a rename), or is
  // a no-op when the name is unchanged. We surface that up front so a re-add is
  // an informed choice, not a silent surprise, and relabel the action to match.
  const normalizedSelected = selectedAbsolute.replace(/\/+$/, '');
  const existingRoot = normalizedSelected
    ? roots.find(r => r.path.replace(/\/+$/, '') === normalizedSelected)
    : undefined;

  const canSubmit = selectedAbsolute !== '' && !addMutation.isPending;

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!canSubmit) return;
    addMutation.mutate(
      { path: selectedAbsolute, name: name.trim() },
      {
        onSuccess: () => {
          onAdded?.();
          onClose();
        },
      },
    );
  };

  // apiFetch always throws an ApiError; its 422 carries the server's teaching
  // message (too broad, not permitted, already-a-root). Fall back to a generic
  // line for the unexpected non-ApiError case so a failed add is never blank.
  const addErrorMessage = addMutation.error
    ? addMutation.error instanceof ApiError
      ? addMutation.error.message
      : t('projects.add_error_fallback')
    : null;

  const rootOptions = roots.map(r => ({
    value: r.path,
    label: `${projectName(r)} (${shortenRootPath(r.path)})`,
  }));

  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4">
      {mode === 'browse' ? (
        <>
          <p className="text-text-muted dark:text-dark-text-muted text-sm">
            {t('projects.browse_hint')}
          </p>

          {roots.length > 1 && (
            <FormField label={t('projects.browse_start_label')}>
              <Select
                className="w-full"
                value={browseRoot}
                onChange={e => handleRootChange(e.target.value)}
                options={rootOptions}
              />
            </FormField>
          )}

          <div className="border-surface-200 dark:border-dark-surface-600 max-h-72 min-h-[11rem] overflow-y-auto rounded-lg border p-2">
            {files.error ? (
              <div className="flex flex-col items-start gap-2 px-1 py-2">
                <span className="text-error-600 dark:text-dark-error-500 text-xs">
                  {files.error}
                </span>
                <Button type="button" variant="outline" size="sm" onClick={() => files.refresh()}>
                  {t('common.retry')}
                </Button>
              </div>
            ) : files.rootLoading && nodes.length === 0 ? (
              <div className="text-text-muted dark:text-dark-text-muted flex items-center gap-2 px-1 py-3 text-xs">
                <Spinner size="sm" />
                {t('projects.browse_loading')}
              </div>
            ) : nodes.length === 0 ? (
              <span className="text-text-muted dark:text-dark-text-muted block px-1 py-3 text-xs">
                {t('projects.browse_empty')}
              </span>
            ) : (
              <FileTree
                nodes={nodes}
                directoryClickMode="expand"
                defaultExpanded={new Set<string>()}
                selectedId={selectedRel ?? undefined}
                onNodeSelect={handleNodeSelect}
              />
            )}
          </div>

          {/* Always-visible confirmation of exactly which folder will be registered. */}
          <div className="border-surface-200 dark:border-dark-surface-600 bg-surface-50 dark:bg-dark-surface-100 rounded-lg border px-3 py-2">
            <span className="text-text-muted dark:text-dark-text-muted block text-[11px] font-medium tracking-wide uppercase">
              {t('projects.selected_label')}
            </span>
            {selectedAbsolute ? (
              <span
                className="text-text dark:text-dark-text mt-0.5 block truncate font-mono text-xs"
                title={selectedAbsolute}>
                {selectedAbsolute}
              </span>
            ) : (
              <span className="text-text-muted dark:text-dark-text-muted mt-0.5 block text-xs">
                {t('projects.selected_none')}
              </span>
            )}
          </div>

          <button
            type="button"
            onClick={() => switchMode('manual')}
            className="text-primary-600 hover:text-primary-700 dark:text-dark-primary-500 dark:hover:text-dark-primary-400 inline-flex items-center gap-1.5 self-start text-xs">
            <Keyboard className="h-3.5 w-3.5" aria-hidden />
            {t('projects.manual_toggle')}
          </button>
        </>
      ) : (
        <>
          <FormField label={t('projects.path_label')} required>
            <Input
              autoFocus
              value={manualPath}
              onChange={e => setManualPath(e.target.value)}
              placeholder={t('projects.path_placeholder')}
              required
            />
          </FormField>
          {hasRoots && (
            <button
              type="button"
              onClick={() => switchMode('browse')}
              className="text-primary-600 hover:text-primary-700 dark:text-dark-primary-500 dark:hover:text-dark-primary-400 inline-flex items-center gap-1.5 self-start text-xs">
              <FolderSearch className="h-3.5 w-3.5" aria-hidden />
              {t('projects.browse_toggle')}
            </button>
          )}
        </>
      )}

      <FormField label={t('projects.name_label')}>
        <Input
          value={name}
          onChange={e => {
            setName(e.target.value);
            setNameEdited(true);
          }}
          placeholder={t('projects.name_placeholder')}
        />
      </FormField>

      {existingRoot && !addErrorMessage && (
        <InlineNotice variant="info">
          {t('projects.already_project', { name: projectName(existingRoot) })}
        </InlineNotice>
      )}

      {addErrorMessage && (
        <InlineNotice variant="error" role="alert">
          {addErrorMessage}
        </InlineNotice>
      )}

      <div className="flex justify-end gap-2 pt-1">
        <Button type="button" variant="outline" palette="neutral" onClick={onClose}>
          {t('common.cancel')}
        </Button>
        <Button type="submit" disabled={!canSubmit} isLoading={addMutation.isPending}>
          {addMutation.isPending
            ? t('projects.add_submitting')
            : existingRoot
              ? t('projects.add_update')
              : t('projects.add_submit')}
        </Button>
      </div>
    </form>
  );
}

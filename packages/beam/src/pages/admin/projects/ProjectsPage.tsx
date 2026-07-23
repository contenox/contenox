import {
  Badge,
  Button,
  EmptyState,
  ErrorState,
  FormField,
  H1,
  InlineNotice,
  Input,
  LoadingState,
  P,
  Page,
  Section,
  Span,
} from '@contenox/ui';
import { MessageSquarePlus, Trash2 } from 'lucide-react';
import { useCallback, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useNavigate } from 'react-router-dom';
import { useQueryClient } from '@tanstack/react-query';
import { startNewChat } from '../../../components/sidebar/newChatIntent';
import { useAcpWorkspace } from '../../../hooks/useAcpWorkspace';
import {
  useAddWorkspaceRoot,
  useForgetWorkspaceRoot,
} from '../../../hooks/useWorkspaceRootMutations';
import { useWorkspaceRoots } from '../../../hooks/useWorkspaceRoots';
import { ApiError } from '../../../lib/fetch';
import { workspaceRootKeys } from '../../../lib/queryKeys';
import { useStagedAgent } from '../../../lib/stagedAgent';
import { useStagedRoot } from '../../../lib/stagedRoot';
import type { WorkspaceRoot } from '../../../lib/types';
import { projectForCwd, projectName } from '../../../lib/workspaceRoots';

/**
 * The project-registry management surface (Slice 4): add, name, and forget the
 * folders this runtime is allowed to open sessions in. Reads the allowlist
 * through the same cached `useWorkspaceRoots` the pickers use, and mutates it
 * through the add/forget mutations, which invalidate that key so every surface
 * re-renders from one source of truth.
 *
 * A project is a `managed` workspace root; the `default` root and the launch
 * roots are structural (not operator-forgettable), so the Forget affordance is
 * offered ONLY on `managed` rows.
 */
export default function ProjectsPage() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { roots, isLoading, error } = useWorkspaceRoots();
  // The live session roster, only to name how many open sessions live under a
  // root in the forget confirmation. A missing/empty roster degrades to a
  // generic confirm — it never blocks the page. focusEmptyTab drives the "New
  // session here" launcher to the empty chat surface (see the launch handler).
  const { workspace, focusEmptyTab } = useAcpWorkspace();
  const { setStagedAgent } = useStagedAgent();
  const { setStagedRoot } = useStagedRoot();

  const addMutation = useAddWorkspaceRoot();
  const forgetMutation = useForgetWorkspaceRoot();

  const [name, setName] = useState('');
  const [path, setPath] = useState('');

  // Sessions whose cwd resolves (deepest segment-aware match) to THIS root —
  // the same containment rule the sidebar uses to label a session's project.
  const sessionCountForRoot = (root: WorkspaceRoot): number =>
    workspace.sessions.filter(s => projectForCwd(s.cwd, roots)?.path === root.path).length;

  const handleAdd = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmedPath = path.trim();
    if (!trimmedPath) return;
    addMutation.mutate(
      { path: trimmedPath, name: name.trim() },
      {
        onSuccess: () => {
          setName('');
          setPath('');
        },
      },
    );
  };

  // Each project row is a LAUNCHER (the JetBrains welcome-screen journey):
  // "New session here" opens the empty chat already scoped to this project,
  // through the SAME single funnel the sidebar's New-session button uses — native
  // agent (cleared), this project staged as the cwd. It closes the register →
  // work gap: today you register a project here, then separately pick its root in
  // the chat's Workspace control; the launcher does both in one click.
  const handleOpen = useCallback(
    (root: WorkspaceRoot) =>
      startNewChat(
        null,
        { setStagedAgent, setStagedRoot, focusEmptyTab, navigate, closeSidebar: () => {} },
        { cwd: root.path },
      ),
    [setStagedAgent, setStagedRoot, focusEmptyTab, navigate],
  );

  const handleForget = (root: WorkspaceRoot) => {
    const projectLabel = projectName(root);
    const count = sessionCountForRoot(root);
    const message =
      count > 0
        ? t('projects.forget_confirm_sessions', { name: projectLabel, count })
        : t('projects.forget_confirm', { name: projectLabel });
    if (!window.confirm(message)) return;
    forgetMutation.mutate(root.path);
  };

  if (isLoading) {
    return (
      <Page bodyScroll="auto">
        <LoadingState message={t('projects.loading')} />
      </Page>
    );
  }

  if (error) {
    return (
      <Page bodyScroll="auto">
        <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 p-4 md:p-6">
          <ErrorState
            error={error}
            onRetry={() => queryClient.invalidateQueries({ queryKey: workspaceRootKeys.list() })}
            title={t('projects.load_error')}
          />
        </div>
      </Page>
    );
  }

  // apiFetch always throws an ApiError; the 422 for a bad path carries the
  // server's teaching message. Fall back to a generic line for the (unexpected)
  // non-ApiError case so a failed add is never a blank crash.
  const addErrorMessage = addMutation.error
    ? addMutation.error instanceof ApiError
      ? addMutation.error.message
      : t('projects.add_error_fallback')
    : null;

  const forgetErrorMessage = forgetMutation.error
    ? forgetMutation.error instanceof ApiError
      ? forgetMutation.error.message
      : t('projects.forget_error_fallback')
    : null;

  return (
    <Page bodyScroll="auto">
      <div className="mx-auto flex w-full max-w-3xl flex-col gap-8 p-4 md:p-6">
        <div>
          <H1 variant="page">{t('projects.title')}</H1>
          <P variant="muted" className="mt-2">
            {t('projects.description')}
          </P>
        </div>

        {roots.length === 0 ? (
          <EmptyState
            title={t('projects.empty_title')}
            description={t('projects.empty_description')}
          />
        ) : (
          <Section>
            <ul className="space-y-2">
              {roots.map(root => {
                const forgetPending =
                  forgetMutation.isPending && forgetMutation.variables === root.path;
                return (
                  <li
                    key={root.path}
                    className="border-surface-200 dark:border-dark-surface-600 flex flex-col gap-3 rounded-lg border p-4 sm:flex-row sm:items-center sm:justify-between">
                    <div className="min-w-0 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="font-medium">{projectName(root)}</span>
                        {root.default && (
                          <Badge variant="secondary" size="sm">
                            {t('projects.default_badge')}
                          </Badge>
                        )}
                      </div>
                      <Span
                        variant="muted"
                        className="block truncate font-mono text-xs"
                        title={root.path}>
                        {root.path}
                      </Span>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {/* Every project is a launcher: open a session scoped to it. */}
                      <Button
                        variant="outline"
                        palette="primary"
                        size="sm"
                        onClick={() => handleOpen(root)}>
                        <MessageSquarePlus className="h-4 w-4" aria-hidden />
                        {t('projects.open')}
                      </Button>
                      {/* Forget is offered ONLY on managed roots — the default and
                          launch roots are structural and not forgettable. */}
                      {root.managed && (
                        <Button
                          variant="outline"
                          palette="neutral"
                          size="sm"
                          isLoading={forgetPending}
                          disabled={forgetPending}
                          onClick={() => handleForget(root)}>
                          <Trash2 className="h-4 w-4" aria-hidden />
                          {t('projects.forget')}
                        </Button>
                      )}
                    </div>
                  </li>
                );
              })}
            </ul>

            {forgetErrorMessage && (
              <InlineNotice variant="error" role="alert" className="mt-3">
                {forgetErrorMessage}
              </InlineNotice>
            )}
          </Section>
        )}

        <Section title={t('projects.add_title')} description={t('projects.add_description')}>
          <form onSubmit={handleAdd} className="space-y-4">
            <FormField label={t('projects.name_label')}>
              <Input
                value={name}
                onChange={e => setName(e.target.value)}
                placeholder={t('projects.name_placeholder')}
              />
            </FormField>
            <FormField label={t('projects.path_label')} required>
              <Input
                value={path}
                onChange={e => setPath(e.target.value)}
                placeholder={t('projects.path_placeholder')}
                required
              />
            </FormField>
            {addErrorMessage && (
              <InlineNotice variant="error" role="alert">
                {addErrorMessage}
              </InlineNotice>
            )}
            <Button type="submit" isLoading={addMutation.isPending} disabled={addMutation.isPending}>
              {addMutation.isPending ? t('projects.add_submitting') : t('projects.add_submit')}
            </Button>
          </form>
        </Section>
      </div>
    </Page>
  );
}

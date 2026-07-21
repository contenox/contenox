import { Button, InlineNotice } from '@contenox/ui';
import { useTranslation } from 'react-i18next';
import type { WorkspaceRoot } from '../../lib/types';
import {
  extractRefusedRoot,
  isWorkspaceRootRefusal,
  shortenRootPath,
} from '../../lib/workspaceRoots';

export interface WorkspaceBoundaryNoticeProps {
  /** The listing error message from the `/files` fetch. */
  message: string;
  /** The allowlisted roots, to name what IS permitted; empty is fine. */
  roots: readonly WorkspaceRoot[];
  onRetry: () => void;
}

/**
 * The file explorer's error surface, redesigned per the component roadmap: when
 * the failure is the workspace-root refusal (a path outside the allowlist), it
 * replaces the raw 422 wire string with a legible statement of the boundary —
 * "this folder is outside the permitted roots" plus the roots that ARE allowed
 * — so an operator learns the boundary instead of decoding an error. Any other
 * error keeps the plain retry notice. Detection is a pure string match
 * (isWorkspaceRootRefusal) so it needs no status code from the fetch layer.
 */
export function WorkspaceBoundaryNotice({ message, roots, onRetry }: WorkspaceBoundaryNoticeProps) {
  const { t } = useTranslation();
  const refused = isWorkspaceRootRefusal(message);

  if (!refused) {
    return (
      <InlineNotice variant="error" className="mb-2">
        <div className="flex items-center justify-between gap-2">
          <span className="min-w-0 truncate">{message}</span>
          <Button type="button" variant="outline" size="xs" onClick={onRetry}>
            {t('workspace.refresh')}
          </Button>
        </div>
      </InlineNotice>
    );
  }

  const offending = extractRefusedRoot(message);

  return (
    <InlineNotice variant="error" className="mb-2">
      <div className="flex flex-col gap-2">
        <div>
          <p className="font-medium">{t('roots.out_of_bounds_title')}</p>
          <p className="text-xs">
            {offending
              ? t('roots.out_of_bounds_body_path', { path: offending })
              : t('roots.out_of_bounds_body')}
          </p>
        </div>
        {roots.length > 0 && (
          <div>
            <p className="text-xs font-medium">{t('roots.out_of_bounds_allowed_label')}</p>
            <ul className="mt-1 space-y-0.5">
              {roots.map(r => (
                <li key={r.path} className="font-mono text-xs break-all" title={r.path}>
                  {shortenRootPath(r.path, 5)}
                  {r.default ? ` · ${t('roots.default_marker')}` : ''}
                </li>
              ))}
            </ul>
          </div>
        )}
        <div>
          <Button type="button" variant="outline" size="xs" onClick={onRetry}>
            {t('workspace.refresh')}
          </Button>
        </div>
      </div>
    </InlineNotice>
  );
}

import { Badge, cn } from '@contenox/ui';
import { FolderTree } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { WorkspaceRoot } from '../../lib/types';
import { projectName } from '../../lib/workspaceRoots';

export interface RootChipProps {
  /** The root to show; `undefined` renders nothing (nil-gated affordance). */
  root?: WorkspaceRoot;
  className?: string;
}

/**
 * A compact chip naming the active workspace root, for the file-explorer header
 * and the dispatch form. Purely informational: it shows WHERE work is rooted so
 * the boundary is legible up front rather than discovered by probing paths.
 * Nil-gated — with no root (the allowlist is absent or empty) it renders
 * nothing, so a serve without a workspace allowlist simply shows no chip. The
 * visible label is the root's friendly project name (its `name`, path-basename
 * fallback); the full path is always available as the title.
 */
export function RootChip({ root, className }: RootChipProps) {
  const { t } = useTranslation();
  if (!root) return null;

  return (
    <Badge
      variant="outline"
      size="sm"
      title={root.path}
      className={cn('inline-flex max-w-full items-center gap-1', className)}>
      <FolderTree aria-hidden="true" className="h-3 w-3 shrink-0" />
      <span className="truncate">{projectName(root)}</span>
      {root.default && (
        <span className="text-text-muted dark:text-dark-text-muted shrink-0">
          · {t('roots.default_marker')}
        </span>
      )}
    </Badge>
  );
}

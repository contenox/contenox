import { Span } from '@contenox/ui';
import type { ToolDiff } from '../../../../lib/diffs';

/**
 * Compact before/after view of a file change from the work log. Deliberately
 * simple (whole-text blocks, not a line-matched diff): the payloads are
 * size-capped by the journal, and the genre calls for glanceable evidence.
 */
export function DiffBlock({ diff }: { diff: ToolDiff }) {
  const created = diff.oldText === '';
  return (
    <div className="border-surface-300 dark:border-dark-surface-400 overflow-hidden rounded border font-mono text-xs">
      <div className="border-surface-300 dark:border-dark-surface-400 bg-surface-100 dark:bg-dark-surface-200 flex items-center gap-2 border-b px-2 py-1">
        <Span className="font-medium">{diff.path}</Span>
        <Span variant="muted" className="text-[10px]">
          {created ? 'created' : 'modified'}
          {diff.toolName ? ` · ${diff.toolName}` : ''}
        </Span>
      </div>
      {!created && (
        <pre className="max-h-48 overflow-auto whitespace-pre-wrap bg-error/5 px-2 py-1 text-error dark:text-dark-error">
          {diff.oldText
            .split('\n')
            .map(l => `- ${l}`)
            .join('\n')}
        </pre>
      )}
      <pre className="max-h-48 overflow-auto whitespace-pre-wrap bg-success/5 px-2 py-1 text-success">
        {diff.newText
          .split('\n')
          .map(l => `+ ${l}`)
          .join('\n')}
      </pre>
    </div>
  );
}

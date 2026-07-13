import { Badge, Button, EmptyState, Span } from '@contenox/ui';
import { X } from 'lucide-react';
import type { CanvasArtifact } from '../../../../lib/artifacts/canvas';

interface CanvasPanelProps {
  className?: string;
  artifact?: CanvasArtifact | null;
  currentRun?: {
    status: string;
    eventsCount: number;
    contextUsed?: number;
    contextSize?: number;
  };
  onClose?: () => void;
}

function renderArtifact(artifact: CanvasArtifact) {
  switch (artifact.kind) {
    case 'run':
      return (
        <div className="space-y-2 text-sm">
          <div className="font-medium">{artifact.title || 'Run'}</div>
          {artifact.body && <div className="text-text-muted text-xs whitespace-pre-wrap">{artifact.body}</div>}
        </div>
      );
    case 'event':
    case 'message':
      return (
        <div className="space-y-2">
          {artifact.title && <div className="font-medium text-sm">{artifact.title}</div>}
          <div className="rounded bg-surface-100 p-3 font-mono text-xs dark:bg-dark-surface-200 whitespace-pre-wrap overflow-auto max-h-[420px]">
            {artifact.body || '(no content)'}
          </div>
        </div>
      );
    case 'markdown':
      return (
        <div className="space-y-2">
          {artifact.title && <div className="font-medium text-sm">{artifact.title}</div>}
          <div className="rounded bg-surface-100 p-3 text-xs dark:bg-dark-surface-200 whitespace-pre-wrap overflow-auto max-h-[420px]">
            {artifact.content || '(no content)'}
          </div>
        </div>
      );
    case 'diff':
      return (
        <div>
          <div className="mb-1 font-medium text-sm">{artifact.title || 'Diff'}</div>
          <pre className="overflow-auto rounded bg-black/5 p-2 text-[10px] dark:bg-white/5">
            {artifact.content || ''}
          </pre>
        </div>
      );
    default:
      return null;
  }
}

export function CanvasPanel({ className, artifact, currentRun, onClose }: CanvasPanelProps) {
  const hasArtifact = artifact && artifact.kind !== 'empty';

  return (
    <div className={`flex min-h-0 flex-col ${className || ''}`}>
      <div className="flex items-center justify-between border-b border-surface-300 px-3 py-2 dark:border-dark-surface-400">
        <div className="flex items-center gap-2">
          <Span className="text-sm font-medium">Canvas</Span>
          {currentRun && (
            <Badge variant={currentRun.status.includes('Failed') ? 'error' : 'secondary'} size="sm">
              {currentRun.status}
            </Badge>
          )}
        </div>
        {onClose && (
          <Button variant="ghost" size="sm" onClick={onClose} aria-label="Close canvas">
            <X className="h-4 w-4" />
          </Button>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-3">
        {hasArtifact ? (
          <div>{renderArtifact(artifact!)}</div>
        ) : currentRun ? (
          <div className="space-y-3 text-sm">
            <div>
              <Span variant="muted" className="text-xs">Current Run</Span>
              <div className="mt-1 font-mono text-xs">{currentRun.status}</div>
            </div>
            <div className="grid grid-cols-2 gap-x-4 gap-y-1 text-xs">
              <div className="text-text-muted">Events</div>
              <div>{currentRun.eventsCount}</div>
              {typeof currentRun.contextUsed === 'number' && (
                <>
                  <div className="text-text-muted">Context</div>
                  <div>
                    {currentRun.contextUsed} / {currentRun.contextSize ?? '—'} tokens
                  </div>
                </>
              )}
            </div>
            <div className="pt-2 text-[11px] text-text-muted">
              Select an item from the timeline to inspect a specific artifact or step.
            </div>
          </div>
        ) : (
          <EmptyState
            title="Artifact Canvas"
            description="Execution artifacts, diffs, plans and previews will appear here as the agent runs."
            icon="🖼️"
            orientation="vertical"
            className="h-full"
          />
        )}
      </div>

      <div className="border-t border-surface-200 p-2 text-[10px] text-text-muted dark:border-dark-surface-300">
        Sovereign workspace preview — Timeline selection drives this view.
      </div>
    </div>
  );
}

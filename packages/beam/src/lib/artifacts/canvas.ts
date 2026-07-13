/**
 * CanvasArtifact — human-visible, session-scoped objects for the Sovereign Workspace.
 *
 * This is deliberately separate from ChatContextArtifact (which is per-turn model input
 * collected via the ArtifactRegistry for prompt injection).
 *
 * See sovereign-workspace.md and sovereign-workspace-architecture.md.
 */

import type { TaskEvent } from '../types';

export type CanvasArtifactId = string;

export type CanvasArtifact =
  | { kind: 'empty' }
  | {
      kind: 'run';
      id: CanvasArtifactId;
      title?: string;
      body?: string;
      requestId?: string;
    }
  | {
      kind: 'event';
      id: CanvasArtifactId;
      title?: string;
      body?: string;
      sourceEventKind?: string;
    }
  | {
      kind: 'message';
      id: CanvasArtifactId;
      title?: string;
      body: string;
    }
  | {
      kind: 'markdown';
      id: CanvasArtifactId;
      title?: string;
      content: string;
    }
  | {
      kind: 'diff';
      id: CanvasArtifactId;
      title?: string;
      content?: string;
    };

export interface CanvasState {
  current?: CanvasArtifact;
  history: CanvasArtifact[];
  selection: { eventTimestamp?: string; artifactId?: CanvasArtifactId };
}

/**
 * Minimal event projection for the first slice.
 * This will grow into the full table from the blueprint.
 * Prefers `content` for structured kinds to match CanvasArtifact spec.
 */
export function projectEventToCanvas(event: TaskEvent | null | undefined): CanvasArtifact | null {
  if (!event) return null;

  const id = `${event.timestamp || Date.now()}-${event.kind}`;

  if (event.kind === 'chain_started') {
    return {
      kind: 'run',
      id,
      title: event.chain_id ? `Run: ${event.chain_id}` : 'Run started',
      body: event.content,
      requestId: event.request_id,
    };
  }

  if (event.kind === 'step_completed' || event.kind === 'step_chunk') {
    const content = event.content || event.thinking || '';
    return {
      kind: 'event',
      id,
      title: `${event.kind}${event.task_handler ? ` · ${event.task_handler}` : ''}`,
      body: content || '(no output)',
      sourceEventKind: event.kind,
    };
  }

  if (event.approval_diff || event.tool_name) {
    return {
      kind: 'diff',
      id,
      title: event.tool_name || 'Change',
      content: event.approval_diff || event.content || '',
    };
  }

  if (event.content || event.thinking) {
    return {
      kind: 'message',
      id,
      title: event.kind,
      body: event.content || event.thinking || '',
    };
  }

  return {
    kind: 'event',
    id,
    title: event.kind,
    body: JSON.stringify(event).slice(0, 200),
  };
}

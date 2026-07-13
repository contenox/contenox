import { useMemo, useState, useCallback } from 'react';
import type { TaskEvent } from '../lib/types';
import { projectEventToCanvas, type CanvasArtifact } from '../lib/artifacts/canvas';

type ProjectedArtifact = Exclude<CanvasArtifact, { kind: 'empty' }>;

export type UseCanvasResult = {
  artifacts: ProjectedArtifact[];
  current: ProjectedArtifact | null;
  select: (event: TaskEvent | null) => void;
  clear: () => void;
};

/**
 * useCanvas manages the human-visible canvas state for a run.
 * It consumes live TaskEvents and projects them to CanvasArtifacts.
 * Selection drives the "current" artifact shown in the canvas panel.
 *
 * This keeps derivation out of page components and follows the architecture plan.
 */
export function useCanvas(events: TaskEvent[]): UseCanvasResult {
  const [selectedId, setSelectedId] = useState<string | null>(null);

  const artifacts = useMemo(() => {
    const projected: ProjectedArtifact[] = [];
    for (const ev of events) {
      const art = projectEventToCanvas(ev);
      if (art && art.kind !== 'empty') {
        projected.push(art);
      }
    }
    return projected;
  }, [events]);

  const current = useMemo(() => {
    if (!selectedId) {
      // Default to the latest meaningful artifact (run or last event)
      return artifacts[artifacts.length - 1] ?? null;
    }
    return artifacts.find(a => a.id === selectedId) ?? null;
  }, [artifacts, selectedId]);

  const select = useCallback((event: TaskEvent | null) => {
    if (!event) {
      setSelectedId(null);
      return;
    }
    // Use a stable id based on the event
    const id = `${event.timestamp}-${event.kind}`;
    setSelectedId(id);
  }, []);

  const clear = useCallback(() => {
    setSelectedId(null);
  }, []);

  return { artifacts, current, select, clear };
}

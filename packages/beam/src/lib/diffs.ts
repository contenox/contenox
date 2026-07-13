import type { TaskEvent } from './types';

export type ToolDiff = {
  path: string;
  oldText: string;
  newText: string;
  toolName: string;
};

/**
 * Extracts the file diffs a run produced, from its (live or journaled)
 * tool_call events. A diff is present when the tool reported a changed file
 * (tool_diff_path set and old/new differ).
 */
export function extractDiffs(events: TaskEvent[]): ToolDiff[] {
  const diffs: ToolDiff[] = [];
  for (const ev of events) {
    if (ev.kind !== 'tool_call' || !ev.tool_diff_path) continue;
    const oldText = ev.tool_diff_old_text ?? '';
    const newText = ev.tool_diff_new_text ?? '';
    if (oldText === newText) continue;
    diffs.push({
      path: ev.tool_diff_path,
      oldText,
      newText,
      toolName: ev.tool_name ?? '',
    });
  }
  return diffs;
}

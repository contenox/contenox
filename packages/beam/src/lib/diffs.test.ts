import { describe, expect, it } from 'vitest';
import { extractDiffs } from './diffs';
import type { TaskEvent } from './types';

const ev = (kind: TaskEvent['kind'], extra: Partial<TaskEvent> = {}): TaskEvent => ({
  kind,
  timestamp: '2026-07-13T12:00:00Z',
  ...extra,
});

describe('extractDiffs', () => {
  it('extracts a file change from a tool_call event', () => {
    const diffs = extractDiffs([
      ev('chain_started'),
      ev('tool_call', {
        tool_name: 'local_fs.write_file',
        tool_diff_path: 'a.txt',
        tool_diff_old_text: 'old',
        tool_diff_new_text: 'new',
      }),
    ]);
    expect(diffs).toEqual([
      { path: 'a.txt', oldText: 'old', newText: 'new', toolName: 'local_fs.write_file' },
    ]);
  });

  it('treats a missing old text as a file creation', () => {
    const diffs = extractDiffs([
      ev('tool_call', { tool_diff_path: 'new.txt', tool_diff_new_text: 'content' }),
    ]);
    expect(diffs[0].oldText).toBe('');
    expect(diffs[0].newText).toBe('content');
  });

  it('ignores tool calls without a diff and unchanged files', () => {
    const diffs = extractDiffs([
      ev('tool_call', { tool_name: 'local_shell.local_shell', content: 'stdout' }),
      ev('tool_call', { tool_diff_path: 'same.txt', tool_diff_old_text: 'x', tool_diff_new_text: 'x' }),
    ]);
    expect(diffs).toHaveLength(0);
  });
});

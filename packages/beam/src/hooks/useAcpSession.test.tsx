import { describe, expect, it } from 'vitest';
import type { AcpTimelineItem } from './acpSessionState';
import { deriveMessages, deriveToolCallOrder, mapStatus } from './useAcpSession';

/**
 * `useAcpSession` is now a thin adapter over `useAcpWorkspace` (which itself
 * reads React context from `AcpWorkspaceProvider`) — see that file's header
 * comment for the full rationale. `@testing-library/react` is still not a
 * dependency of `packages/beam` (no jsdom either — `npx vitest run` runs in
 * plain Node; verified: mounting a component tree isn't possible here
 * without adding a new dependency, which this stage's brief disallows).
 *
 * The adapter's OWN logic — collapsing workspace status and un-interleaving
 * the unified timeline back into the page's original flat-lists shape — is
 * pure and exported specifically so it can be unit-tested directly, which is
 * what this file does. The underlying multi-session behavior (connect,
 * switch, reconnect, permission handling, etc.) is covered by
 * `acpWorkspaceController.test.ts`, `acpSessionState.test.ts`, and
 * `acpWorkspaceState.test.ts`; this file does not re-test any of that.
 */

describe('useAcpSession adapter: mapStatus', () => {
  it('collapses the 6-value workspace status onto the page\'s original 3-value one', () => {
    expect(mapStatus('connecting')).toBe('connecting');
    expect(mapStatus('reconnecting')).toBe('connecting');
    expect(mapStatus('ready')).toBe('ready');
    expect(mapStatus('disconnected')).toBe('error');
    expect(mapStatus('setup_required')).toBe('error');
    expect(mapStatus('error')).toBe('error');
  });
});

describe('useAcpSession adapter: deriveMessages', () => {
  it('un-interleaves the unified timeline back into a flat, arrival-ordered message list', () => {
    const items: AcpTimelineItem[] = [
      { kind: 'message', id: 'u1' },
      { kind: 'tool_call', id: 'tc-1' },
      { kind: 'message', id: 'a1' },
    ];
    const messages = {
      u1: { id: 'u1', role: 'user' as const, text: 'hi' },
      a1: { id: 'a1', role: 'assistant' as const, text: 'hello!' },
    };
    expect(deriveMessages(items, messages)).toEqual([
      { id: 'u1', role: 'user', text: 'hi' },
      { id: 'a1', role: 'assistant', text: 'hello!' },
    ]);
  });

  it('skips a timeline item whose message record is missing rather than throwing', () => {
    const items: AcpTimelineItem[] = [{ kind: 'message', id: 'missing' }];
    expect(deriveMessages(items, {})).toEqual([]);
  });
});

describe('useAcpSession adapter: deriveToolCallOrder', () => {
  it('extracts tool-call ids from the unified timeline in arrival order, ignoring messages', () => {
    const items: AcpTimelineItem[] = [
      { kind: 'message', id: 'u1' },
      { kind: 'tool_call', id: 'tc-1' },
      { kind: 'message', id: 'a1' },
      { kind: 'tool_call', id: 'tc-2' },
    ];
    expect(deriveToolCallOrder(items)).toEqual(['tc-1', 'tc-2']);
  });
});
